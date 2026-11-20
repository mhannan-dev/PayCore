package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

// ErrResetThrottled means a reset link was issued for the user very recently.
var ErrResetThrottled = errors.New("a reset link was issued recently")

// PasswordResetRepository stores single-use "forgot password" tokens.
type PasswordResetRepository struct{ pool *pgxpool.Pool }

func NewPasswordResetRepository(pool *pgxpool.Pool) *PasswordResetRepository {
	return &PasswordResetRepository{pool: pool}
}

// IssueResetToken replaces the active user's unused reset tokens with a new one. It
// returns pgx.ErrNoRows when no active user has the email, and ErrResetThrottled when
// a token was issued within cooldown.
func (r *PasswordResetRepository) IssueResetToken(ctx context.Context, email string, tokenHash []byte, expires time.Time, cooldown time.Duration, draft ...model.EmailDraft) (model.ResetUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.ResetUser{}, err
	}
	defer tx.Rollback(ctx)

	var u model.ResetUser
	// Lock the user row so concurrent requests serialize on the cooldown check.
	if err := tx.QueryRow(ctx, "SELECT id, email, first_name FROM users WHERE email = $1 AND is_active FOR UPDATE", email).Scan(&u.ID, &u.Email, &u.FirstName); err != nil {
		return model.ResetUser{}, err
	}
	var recent bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM password_reset_tokens WHERE user_id = $1 AND created_at > NOW() - make_interval(secs => $2))",
		u.ID, cooldown.Seconds()).Scan(&recent); err != nil {
		return model.ResetUser{}, err
	}
	if recent {
		return model.ResetUser{}, ErrResetThrottled
	}
	if _, err := tx.Exec(ctx, "DELETE FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL", u.ID); err != nil {
		return model.ResetUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)", tokenHash, u.ID, expires); err != nil {
		return model.ResetUser{}, err
	}
	if err := insertEmail(ctx, tx, model.EmailPasswordReset, model.EmailRecipient{ID: u.ID, Email: u.Email, FirstName: u.FirstName}, tokenHash, expires, draft...); err != nil {
		return model.ResetUser{}, err
	}
	return u, tx.Commit(ctx)
}

// FindResetToken returns who a usable token (unused, unexpired, active user) belongs to, or pgx.ErrNoRows.
func (r *PasswordResetRepository) FindResetToken(ctx context.Context, tokenHash []byte) (model.PasswordResetInfo, error) {
	var info model.PasswordResetInfo
	err := r.pool.QueryRow(ctx, `SELECT u.email, u.first_name, t.expires_at
		FROM password_reset_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1 AND t.used_at IS NULL AND t.expires_at > NOW() AND u.is_active`, tokenHash).
		Scan(&info.Email, &info.FirstName, &info.ExpiresAt)
	return info, err
}

// ResetPassword consumes a usable token, sets the password, signs the user out of
// every session and closes any pending invitation. It returns the user's email, or
// pgx.ErrNoRows when the token is unknown, used or expired.
func (r *PasswordResetRepository) ResetPassword(ctx context.Context, tokenHash []byte, passwordHash string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `UPDATE password_reset_tokens SET used_at = NOW()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		RETURNING user_id`, tokenHash).Scan(&userID)
	if err != nil {
		return "", err
	}
	var email string
	err = tx.QueryRow(ctx, "UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1 AND is_active RETURNING email",
		userID, passwordHash).Scan(&email)
	if err != nil {
		return "", err
	}
	for _, stmt := range []string{
		"DELETE FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL", // other outstanding links
		"UPDATE user_invitations SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL",
		"DELETE FROM auth_sessions WHERE user_id = $1", // sign out everywhere
	} {
		if _, err := tx.Exec(ctx, stmt, userID); err != nil {
			return "", err
		}
	}
	return email, tx.Commit(ctx)
}
