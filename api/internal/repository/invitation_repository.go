package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

// ErrEmailTaken is returned when another user already has the email.
var ErrEmailTaken = errors.New("email already registered")

// ErrAlreadyActivated is returned when re-inviting a user who has already set a password.
var ErrAlreadyActivated = errors.New("user has already set a password")

// InvitationRepository stores invited users and their single-use set-password tokens.
type InvitationRepository struct{ pool *pgxpool.Pool }

func NewInvitationRepository(pool *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{pool: pool}
}

// nullableUUID turns "" into NULL for created_by.
func nullableUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// CreateInvitedUser inserts an active user without a password, their roles, and an invitation.
func (r *InvitationRepository) CreateInvitedUser(ctx context.Context, firstName string, lastName *string, email string, roles []string, tokenHash []byte, expires time.Time, createdBy string) (model.ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.ManagedUser{}, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, "INSERT INTO users (first_name, last_name, email, password_hash, roles) VALUES ($1, $2, $3, NULL, $4) RETURNING id",
		firstName, lastName, email, roles).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return model.ManagedUser{}, ErrEmailTaken
	}
	if err != nil {
		return model.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = ANY($2) ON CONFLICT DO NOTHING", id, roles); err != nil {
		return model.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_invitations (token_hash, user_id, created_by, expires_at) VALUES ($1, $2, $3, $4)", tokenHash, id, nullableUUID(createdBy), expires); err != nil {
		return model.ManagedUser{}, err
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+" WHERE users.id = $1", id))
	if err != nil {
		return model.ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}

// ReissueInvitation replaces a not-yet-activated user's unused invitations with a new one.
// It returns pgx.ErrNoRows for an unknown user and ErrAlreadyActivated once a password is set.
func (r *InvitationRepository) ReissueInvitation(ctx context.Context, userID string, tokenHash []byte, expires time.Time, createdBy string) (model.ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.ManagedUser{}, err
	}
	defer tx.Rollback(ctx)

	var passwordSet bool
	if err := tx.QueryRow(ctx, "SELECT password_hash IS NOT NULL FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&passwordSet); err != nil {
		return model.ManagedUser{}, notFound(err, ErrUserNotFound)
	}
	if passwordSet {
		return model.ManagedUser{}, ErrAlreadyActivated
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_invitations WHERE user_id = $1 AND used_at IS NULL", userID); err != nil {
		return model.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_invitations (token_hash, user_id, created_by, expires_at) VALUES ($1, $2, $3, $4)", tokenHash, userID, nullableUUID(createdBy), expires); err != nil {
		return model.ManagedUser{}, err
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+" WHERE users.id = $1", userID))
	if err != nil {
		return model.ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}

// FindInvitation returns the invited user for a usable token (unused, unexpired, user
// active and still without a password), or pgx.ErrNoRows.
func (r *InvitationRepository) FindInvitation(ctx context.Context, tokenHash []byte) (model.InvitationInfo, error) {
	var info model.InvitationInfo
	err := r.pool.QueryRow(ctx, `SELECT u.email, u.first_name, i.expires_at
		FROM user_invitations i JOIN users u ON u.id = i.user_id
		WHERE i.token_hash = $1 AND i.used_at IS NULL AND i.expires_at > NOW()
			AND u.is_active AND u.password_hash IS NULL`, tokenHash).Scan(&info.Email, &info.FirstName, &info.ExpiresAt)
	return info, err
}

// AcceptInvitation consumes a usable token and sets the user's password. It returns the
// user's email, or pgx.ErrNoRows when the token is unknown, used, expired or the user
// already has a password.
func (r *InvitationRepository) AcceptInvitation(ctx context.Context, tokenHash []byte, passwordHash string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `UPDATE user_invitations SET used_at = NOW()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		RETURNING user_id`, tokenHash).Scan(&userID)
	if err != nil {
		return "", err
	}
	var email string
	err = tx.QueryRow(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW()
		WHERE id = $1 AND is_active AND password_hash IS NULL
		RETURNING email`, userID, passwordHash).Scan(&email)
	if err != nil {
		return "", err
	}
	// Any other outstanding links for this user stop working.
	if _, err := tx.Exec(ctx, "DELETE FROM user_invitations WHERE user_id = $1 AND used_at IS NULL", userID); err != nil {
		return "", err
	}
	return email, tx.Commit(ctx)
}

// RecordEmail stores the outcome of one delivery attempt for an invitation. A
// replaced (re-sent) invitation no longer exists, so its late result is ignored.
func (r *InvitationRepository) RecordEmail(ctx context.Context, tokenHash []byte, status string, emailError string) error {
	var errText any
	if emailError != "" {
		if len(emailError) > 480 {
			emailError = emailError[:480] + "…"
		}
		errText = emailError
	}
	_, err := r.pool.Exec(ctx, `UPDATE user_invitations SET email_status = $2, email_attempts = email_attempts + 1,
		email_sent_at = CASE WHEN $2 = 'sent' THEN NOW() ELSE email_sent_at END,
		email_error = $3
		WHERE token_hash = $1`, tokenHash, status, errText)
	return err
}
