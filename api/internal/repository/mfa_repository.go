package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

type MFARepository struct{ pool *pgxpool.Pool }

func NewMFARepository(pool *pgxpool.Pool) *MFARepository { return &MFARepository{pool: pool} }
func (r *MFARepository) State(ctx context.Context, id string) (model.MFARecord, error) {
	state := model.MFARecord{UserID: id, LastCounter: -1}
	err := r.pool.QueryRow(ctx, "SELECT encrypted_secret,last_counter FROM user_mfa WHERE user_id=$1", id).Scan(&state.Secret, &state.LastCounter)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	return state, err
}
func (r *MFARepository) Start(ctx context.Context, u model.User, ch model.MFAChallenge) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var hash string
	var active bool
	if err = tx.QueryRow(ctx, "SELECT COALESCE(password_hash,''),is_active FROM users WHERE id=$1 FOR UPDATE", u.ID).Scan(&hash, &active); err != nil {
		return err
	}
	if !active || hash != u.PasswordHash {
		return model.ErrCredentialsChanged
	}
	if _, err = tx.Exec(ctx, "INSERT INTO user_mfa(user_id) VALUES ($1) ON CONFLICT DO NOTHING", u.ID); err != nil {
		return err
	}
	var secret []byte
	var locked *time.Time
	if err = tx.QueryRow(ctx, "SELECT encrypted_secret,locked_until FROM user_mfa WHERE user_id=$1 FOR UPDATE", u.ID).Scan(&secret, &locked); err != nil {
		return err
	}
	if locked != nil && locked.After(time.Now()) {
		return model.ErrMFALocked
	}
	if (len(secret) > 0) == (len(ch.PendingSecret) > 0) {
		return model.ErrMFAChallenge
	}
	if _, err = tx.Exec(ctx, "DELETE FROM mfa_challenges WHERE user_id=$1", u.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mfa_challenges(token_hash,user_id,password_hash_snapshot,pending_secret,expires_at) VALUES($1,$2,$3,$4,$5)`, ch.Hash, u.ID, hash, ch.PendingSecret, ch.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Complete serializes credential changes, enrollment, one-time-code consumption
// and session issuance. No HTTP or network I/O runs inside the decision callback.
func (r *MFARepository) Complete(ctx context.Context, challenge, session []byte, expiry time.Time, decide model.MFADecide) (model.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, "SELECT user_id FROM mfa_challenges WHERE token_hash=$1", challenge).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.ErrMFAChallenge
	} else if err != nil {
		return model.User{}, err
	}
	var password string
	var active bool
	if err = tx.QueryRow(ctx, "SELECT COALESCE(password_hash,''),is_active FROM users WHERE id=$1 FOR UPDATE", id).Scan(&password, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, model.ErrMFAChallenge
		}
		return model.User{}, err
	}
	var snapshot string
	var pending []byte
	err = tx.QueryRow(ctx, `SELECT password_hash_snapshot,pending_secret FROM mfa_challenges WHERE token_hash=$1 AND user_id=$2 AND expires_at>clock_timestamp() FOR UPDATE`, challenge, id).Scan(&snapshot, &pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.ErrMFAChallenge
	}
	if err != nil {
		return model.User{}, err
	}
	if !active || password != snapshot {
		return model.User{}, model.ErrMFAChallenge
	}
	state := model.MFARecord{UserID: id, PendingSecret: pending}
	var locked *time.Time
	var failed int
	if err = tx.QueryRow(ctx, "SELECT encrypted_secret,last_counter,locked_until,failed_attempts FROM user_mfa WHERE user_id=$1 FOR UPDATE", id).Scan(&state.Secret, &state.LastCounter, &locked, &failed); err != nil {
		return model.User{}, err
	}
	if locked != nil {
		if locked.After(time.Now()) {
			return model.User{}, model.ErrMFALocked
		}
		failed = 0
	}
	rows, err := tx.Query(ctx, "SELECT code_hash FROM mfa_recovery_codes WHERE user_id=$1", id)
	if err != nil {
		return model.User{}, err
	}
	for rows.Next() {
		var hash []byte
		if err = rows.Scan(&hash); err != nil {
			rows.Close()
			return model.User{}, err
		}
		state.RecoveryHashes = append(state.RecoveryHashes, hash)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return model.User{}, err
	}
	approval, err := decide(state)
	if errors.Is(err, model.ErrMFACode) {
		failed++
		if _, err = tx.Exec(ctx, `UPDATE user_mfa SET failed_attempts=$2,locked_until=CASE WHEN $2>=5 THEN clock_timestamp()+INTERVAL '5 minutes' ELSE NULL END WHERE user_id=$1`, id, failed); err != nil {
			return model.User{}, err
		}
		if failed >= 5 {
			if _, err = tx.Exec(ctx, "DELETE FROM mfa_challenges WHERE user_id=$1", id); err != nil {
				return model.User{}, err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return model.User{}, err
		}
		if failed >= 5 {
			return model.User{}, model.ErrMFALocked
		}
		return model.User{}, model.ErrMFACode
	}
	if err != nil {
		return model.User{}, err
	}
	enrolling := len(state.Secret) == 0
	if enrolling {
		if len(pending) == 0 || len(approval.NewRecoveryHashes) != 8 {
			return model.User{}, model.ErrMFAChallenge
		}
		if _, err = tx.Exec(ctx, "UPDATE user_mfa SET encrypted_secret=$2,enabled_at=NOW() WHERE user_id=$1", id, pending); err != nil {
			return model.User{}, err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id=$1", id); err != nil {
			return model.User{}, err
		}
		for _, hash := range approval.NewRecoveryHashes {
			if _, err = tx.Exec(ctx, "INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)", id, hash); err != nil {
				return model.User{}, err
			}
		}
	}
	if len(approval.RecoveryHash) > 0 {
		if enrolling {
			return model.User{}, model.ErrMFACode
		}
		tag, err := tx.Exec(ctx, "DELETE FROM mfa_recovery_codes WHERE user_id=$1 AND code_hash=$2", id, approval.RecoveryHash)
		if err != nil {
			return model.User{}, err
		}
		if tag.RowsAffected() != 1 {
			return model.User{}, model.ErrMFACode
		}
	} else if approval.Counter <= state.LastCounter {
		return model.User{}, model.ErrMFACode
	}
	if _, err = tx.Exec(ctx, "UPDATE user_mfa SET last_counter=$2,failed_attempts=0,locked_until=NULL WHERE user_id=$1", id, approval.Counter); err != nil {
		return model.User{}, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM mfa_challenges WHERE token_hash=$1", challenge); err != nil {
		return model.User{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO auth_sessions(token_hash,user_id,expires_at,mfa_verified) VALUES($1,$2,$3,TRUE)", session, id, expiry); err != nil {
		return model.User{}, err
	}
	u, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users u "+userPhotoJoin+" WHERE u.id=$1", id))
	if err != nil {
		return model.User{}, err
	}
	return u, tx.Commit(ctx)
}
