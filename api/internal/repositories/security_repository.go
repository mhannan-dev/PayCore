package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

type SecurityRepository struct {
	pool        *pgxpool.Pool
	mfaDisabled bool
}

func NewSecurityRepository(pool *pgxpool.Pool) *SecurityRepository {
	return &SecurityRepository{pool: pool}
}

func NewSecurityRepositoryWithMFAPolicy(pool *pgxpool.Pool, enabled bool) *SecurityRepository {
	return &SecurityRepository{pool: pool, mfaDisabled: !enabled}
}

type securityLock struct {
	record            models.SecurityRecord
	failed            int
	locked, enabledAt *time.Time
}

// All security mutations and login-MFA completion lock the user first. Revalidate
// the current session under that lock, even if the HTTP Guard already checked it.
func (r *SecurityRepository) lockSecurity(ctx context.Context, tx pgx.Tx, owner string, hash []byte) (securityLock, error) {
	var locked securityLock
	if owner == "" {
		return locked, models.ErrSecuritySession
	}
	u, err := scanUser(tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users u "+userPhotoJoin+" WHERE u.id=$1 FOR UPDATE OF u", owner))
	if errors.Is(err, pgx.ErrNoRows) {
		return locked, models.ErrSecuritySession
	}
	if err != nil {
		return locked, err
	}
	if !u.IsActive {
		return locked, models.ErrSecuritySession
	}
	locked.record.User = u
	locked.record.MFA = models.MFARecord{UserID: owner, LastCounter: -1}
	var verified bool
	err = tx.QueryRow(ctx, `SELECT id,mfa_verified FROM auth_sessions WHERE user_id=$1 AND token_hash=$2 AND expires_at>clock_timestamp() FOR UPDATE`, owner, hash).Scan(&locked.record.SessionID, &verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return locked, models.ErrSecuritySession
	}
	if err != nil {
		return locked, err
	}
	err = tx.QueryRow(ctx, `SELECT encrypted_secret,last_counter,failed_attempts,locked_until,enabled_at FROM user_mfa WHERE user_id=$1 FOR UPDATE`, owner).Scan(&locked.record.MFA.Secret, &locked.record.MFA.LastCounter, &locked.failed, &locked.locked, &locked.enabledAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return locked, err
	}
	if !r.mfaDisabled && (u.HasRole(models.RoleAdmin) || len(locked.record.MFA.Secret) > 0) && !verified {
		return locked, models.ErrSecuritySession
	}
	rows, err := tx.Query(ctx, "SELECT code_hash FROM mfa_recovery_codes WHERE user_id=$1", owner)
	if err != nil {
		return locked, err
	}
	for rows.Next() {
		var code []byte
		if err = rows.Scan(&code); err != nil {
			rows.Close()
			return locked, err
		}
		locked.record.MFA.RecoveryHashes = append(locked.record.MFA.RecoveryHashes, code)
	}
	err = rows.Err()
	rows.Close()
	return locked, err
}
func (r *SecurityRepository) Overview(ctx context.Context, owner string, hash []byte) (models.SecurityOverview, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.SecurityOverview{}, err
	}
	defer tx.Rollback(ctx)
	locked, err := r.lockSecurity(ctx, tx, owner, hash)
	if err != nil {
		return models.SecurityOverview{}, err
	}
	enabled := len(locked.record.MFA.Secret) > 0
	result := models.SecurityOverview{MFAEnforced: !r.mfaDisabled, MFAEnabled: enabled, MFARequired: !r.mfaDisabled && (enabled || locked.record.User.HasRole(models.RoleAdmin)), EnabledAt: locked.enabledAt, RecoveryCodesRemaining: len(locked.record.MFA.RecoveryHashes), Sessions: []models.SessionInfo{}}
	rows, err := tx.Query(ctx, `SELECT id,created_at,expires_at,mfa_verified,token_hash=$2 FROM auth_sessions WHERE user_id=$1 AND expires_at>clock_timestamp() AND ($3=FALSE OR mfa_verified) ORDER BY (token_hash=$2) DESC,created_at DESC,id LIMIT 101`, owner, hash, result.MFARequired)
	if err != nil {
		return models.SecurityOverview{}, err
	}
	for rows.Next() {
		var session models.SessionInfo
		if err = rows.Scan(&session.ID, &session.CreatedAt, &session.ExpiresAt, &session.MFAVerified, &session.Current); err != nil {
			rows.Close()
			return models.SecurityOverview{}, err
		}
		result.Sessions = append(result.Sessions, session)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return models.SecurityOverview{}, err
	}
	if len(result.Sessions) > 100 {
		result.Sessions = result.Sessions[:100]
		result.MoreSessions = true
	}
	return result, tx.Commit(ctx)
}
func (r *SecurityRepository) Change(ctx context.Context, owner string, hash []byte, command models.SecurityCommand, decide models.SecurityDecide) (models.SecurityActionResult, error) {
	if r.mfaDisabled && command.Action != models.SecurityRevokeSession && command.Action != models.SecurityRevokeOthers {
		return models.SecurityActionResult{}, models.ErrMFADisabled
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.SecurityActionResult{}, err
	}
	defer tx.Rollback(ctx)
	locked, err := r.lockSecurity(ctx, tx, owner, hash)
	if err != nil {
		return models.SecurityActionResult{}, err
	}
	if locked.locked != nil {
		if locked.locked.After(time.Now()) {
			return models.SecurityActionResult{}, models.ErrMFALocked
		}
		locked.failed = 0
	}
	if _, err = tx.Exec(ctx, "INSERT INTO user_mfa(user_id) VALUES($1) ON CONFLICT DO NOTHING", owner); err != nil {
		return models.SecurityActionResult{}, err
	}
	if command.Action == models.SecurityRevokeSession {
		if command.TargetID == locked.record.SessionID {
			return models.SecurityActionResult{}, models.ErrSecurityCurrentSession
		}
		var found string
		err = tx.QueryRow(ctx, "SELECT id FROM auth_sessions WHERE user_id=$1 AND id=$2 AND expires_at>clock_timestamp() FOR UPDATE", owner, command.TargetID).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return models.SecurityActionResult{}, models.ErrSecurityTarget
		}
		if err != nil {
			return models.SecurityActionResult{}, err
		}
	}
	if command.Action == models.SecurityReplaceConfirm {
		var snapshot string
		err = tx.QueryRow(ctx, `SELECT encrypted_secret,password_hash_snapshot FROM mfa_replacements WHERE user_id=$1 AND session_id=$2 AND expires_at>clock_timestamp() FOR UPDATE`, owner, locked.record.SessionID).Scan(&locked.record.MFA.PendingSecret, &snapshot)
		if errors.Is(err, pgx.ErrNoRows) {
			return models.SecurityActionResult{}, models.ErrSecurityReplacement
		}
		if err != nil {
			return models.SecurityActionResult{}, err
		}
		if snapshot != locked.record.User.PasswordHash {
			return models.SecurityActionResult{}, models.ErrSecurityReplacement
		}
	}
	approval, err := decide(locked.record)
	if errors.Is(err, models.ErrMFACode) {
		failed := locked.failed + 1
		if _, err = tx.Exec(ctx, `UPDATE user_mfa SET failed_attempts=$2,locked_until=CASE WHEN $2>=5 THEN clock_timestamp()+INTERVAL '5 minutes' ELSE NULL END WHERE user_id=$1`, owner, failed); err != nil {
			return models.SecurityActionResult{}, err
		}
		if failed >= 5 {
			for _, query := range []string{"DELETE FROM mfa_challenges WHERE user_id=$1", "DELETE FROM mfa_replacements WHERE user_id=$1"} {
				if _, err = tx.Exec(ctx, query, owner); err != nil {
					return models.SecurityActionResult{}, err
				}
			}
		}
		if err = insertAudit(ctx, tx, audit.Event{Action: audit.MFAFailed, ActorID: audit.Ptr(owner), ActorEmail: audit.Ptr(locked.record.User.Email), TargetID: audit.Ptr(owner), TargetEmail: audit.Ptr(locked.record.User.Email), Details: audit.Details{Reason: "security_verification_failed"}}); err != nil {
			return models.SecurityActionResult{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return models.SecurityActionResult{}, err
		}
		if failed >= 5 {
			return models.SecurityActionResult{}, models.ErrMFALocked
		}
		return models.SecurityActionResult{}, models.ErrMFACode
	}
	if err != nil {
		return models.SecurityActionResult{}, err
	}
	if command.Action != models.SecurityReplaceConfirm && len(approval.RecoveryHash) > 0 {
		tag, err := tx.Exec(ctx, "DELETE FROM mfa_recovery_codes WHERE user_id=$1 AND code_hash=$2", owner, approval.RecoveryHash)
		if err != nil {
			return models.SecurityActionResult{}, err
		}
		if tag.RowsAffected() != 1 {
			return models.SecurityActionResult{}, models.ErrMFACode
		}
	} else if !r.mfaDisabled && len(locked.record.MFA.Secret) > 0 && command.Action != models.SecurityReplaceConfirm && approval.Counter <= locked.record.MFA.LastCounter {
		return models.SecurityActionResult{}, models.ErrMFACode
	}
	result := models.SecurityActionResult{}
	var action audit.Action
	switch command.Action {
	case models.SecurityRecoveryCodes:
		if err = replaceRecoveryCodes(ctx, tx, owner, approval.NewRecoveryHashes); err != nil {
			return result, err
		}
		for _, query := range []string{"DELETE FROM mfa_challenges WHERE user_id=$1", "DELETE FROM mfa_replacements WHERE user_id=$1"} {
			if _, err = tx.Exec(ctx, query, owner); err != nil {
				return result, err
			}
		}
		action = audit.MFARecoveryRegenerated
	case models.SecurityReplaceBegin:
		if len(command.PendingSecret) == 0 {
			return result, models.ErrSecurityReplacement
		}
		if _, err = tx.Exec(ctx, `INSERT INTO mfa_replacements(user_id,session_id,password_hash_snapshot,encrypted_secret,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id) DO UPDATE SET session_id=EXCLUDED.session_id,password_hash_snapshot=EXCLUDED.password_hash_snapshot,encrypted_secret=EXCLUDED.encrypted_secret,expires_at=EXCLUDED.expires_at`, owner, locked.record.SessionID, locked.record.User.PasswordHash, command.PendingSecret, command.ExpiresAt); err != nil {
			return result, err
		}
		action = audit.MFAReplacementStarted
	case models.SecurityReplaceConfirm:
		if len(locked.record.MFA.PendingSecret) == 0 {
			return result, models.ErrSecurityReplacement
		}
		if _, err = tx.Exec(ctx, "UPDATE user_mfa SET encrypted_secret=$2,enabled_at=NOW() WHERE user_id=$1", owner, locked.record.MFA.PendingSecret); err != nil {
			return result, err
		}
		if err = replaceRecoveryCodes(ctx, tx, owner, approval.NewRecoveryHashes); err != nil {
			return result, err
		}
		for _, query := range []string{"DELETE FROM mfa_challenges WHERE user_id=$1", "DELETE FROM mfa_replacements WHERE user_id=$1"} {
			if _, err = tx.Exec(ctx, query, owner); err != nil {
				return result, err
			}
		}
		tag, err := tx.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id=$1 AND id<>$2", owner, locked.record.SessionID)
		if err != nil {
			return result, err
		}
		result.RevokedCount = tag.RowsAffected()
		action = audit.MFAReplaced
	case models.SecurityRevokeSession:
		tag, err := tx.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id=$1 AND id=$2", owner, command.TargetID)
		if err != nil {
			return result, err
		}
		result.RevokedCount = tag.RowsAffected()
		action = audit.SessionsRevoked
	case models.SecurityRevokeOthers:
		tag, err := tx.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id=$1 AND id<>$2", owner, locked.record.SessionID)
		if err != nil {
			return result, err
		}
		result.RevokedCount = tag.RowsAffected()
		action = audit.SessionsRevoked
	default:
		return result, errors.New("unknown security action")
	}
	if r.mfaDisabled {
		approval.Counter = locked.record.MFA.LastCounter
	}
	if _, err = tx.Exec(ctx, "UPDATE user_mfa SET last_counter=$2,failed_attempts=0,locked_until=NULL WHERE user_id=$1", owner, approval.Counter); err != nil {
		return result, err
	}
	if err = insertAudit(ctx, tx, audit.Event{Action: action, ActorID: audit.Ptr(owner), ActorEmail: audit.Ptr(locked.record.User.Email), TargetID: audit.Ptr(owner), TargetEmail: audit.Ptr(locked.record.User.Email), Details: audit.Details{Reason: string(command.Action), Count: &result.RevokedCount}}); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func replaceRecoveryCodes(ctx context.Context, tx pgx.Tx, owner string, hashes [][]byte) error {
	if len(hashes) != 8 {
		return errors.New("exactly eight recovery hashes required")
	}
	if _, err := tx.Exec(ctx, "DELETE FROM mfa_recovery_codes WHERE user_id=$1", owner); err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err := tx.Exec(ctx, "INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)", owner, hash); err != nil {
			return err
		}
	}
	return nil
}
