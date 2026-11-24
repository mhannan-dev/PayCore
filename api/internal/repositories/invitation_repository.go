package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
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
func (r *InvitationRepository) CreateInvitedUser(ctx context.Context, firstName string, lastName *string, email string, roles []string, tokenHash []byte, expires time.Time, createdBy string, draft ...models.EmailDraft) (models.ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.ManagedUser{}, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, "INSERT INTO users (first_name, last_name, email, password_hash, roles) VALUES ($1, $2, $3, NULL, $4) RETURNING id",
		firstName, lastName, email, roles).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return models.ManagedUser{}, ErrEmailTaken
	}
	if err != nil {
		return models.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = ANY($2) ON CONFLICT DO NOTHING", id, roles); err != nil {
		return models.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_invitations (token_hash, user_id, created_by, expires_at) VALUES ($1, $2, $3, $4)", tokenHash, id, nullableUUID(createdBy), expires); err != nil {
		return models.ManagedUser{}, err
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+" WHERE users.id = $1", id))
	if err != nil {
		return models.ManagedUser{}, err
	}
	if err := insertEmail(ctx, tx, models.EmailInvitation, models.EmailRecipient{ID: u.ID, Email: u.Email, FirstName: u.FirstName}, tokenHash, expires, draft...); err != nil {
		return models.ManagedUser{}, err
	}
	if err := insertAudit(ctx, tx, audit.Event{Action: audit.UserInvited, TargetID: audit.Ptr(u.ID), TargetEmail: audit.Ptr(u.Email), Details: audit.Details{RolesAfter: u.Roles}}); err != nil {
		return models.ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}

// ReissueInvitation replaces a not-yet-activated user's unused invitations with a new one.
// It returns pgx.ErrNoRows for an unknown user and ErrAlreadyActivated once a password is set.
func (r *InvitationRepository) ReissueInvitation(ctx context.Context, userID string, tokenHash []byte, expires time.Time, createdBy string, draft ...models.EmailDraft) (models.ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.ManagedUser{}, err
	}
	defer tx.Rollback(ctx)

	var passwordSet bool
	if err := tx.QueryRow(ctx, "SELECT password_hash IS NOT NULL FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&passwordSet); err != nil {
		return models.ManagedUser{}, notFound(err, ErrUserNotFound)
	}
	if passwordSet {
		return models.ManagedUser{}, ErrAlreadyActivated
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_invitations WHERE user_id = $1 AND used_at IS NULL", userID); err != nil {
		return models.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_invitations (token_hash, user_id, created_by, expires_at) VALUES ($1, $2, $3, $4)", tokenHash, userID, nullableUUID(createdBy), expires); err != nil {
		return models.ManagedUser{}, err
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+" WHERE users.id = $1", userID))
	if err != nil {
		return models.ManagedUser{}, err
	}
	if err := insertEmail(ctx, tx, models.EmailInvitation, models.EmailRecipient{ID: u.ID, Email: u.Email, FirstName: u.FirstName}, tokenHash, expires, draft...); err != nil {
		return models.ManagedUser{}, err
	}
	if err := insertAudit(ctx, tx, audit.Event{Action: audit.InvitationResent, TargetID: audit.Ptr(u.ID), TargetEmail: audit.Ptr(u.Email)}); err != nil {
		return models.ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}

// FindInvitation returns the invited user for a usable token (unused, unexpired, user
// active and still without a password), or pgx.ErrNoRows.
func (r *InvitationRepository) FindInvitation(ctx context.Context, tokenHash []byte) (models.InvitationInfo, error) {
	var info models.InvitationInfo
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
	if err := insertAudit(ctx, tx, audit.Event{Action: audit.InvitationAccepted, ActorID: audit.Ptr(userID), ActorEmail: audit.Ptr(email), TargetID: audit.Ptr(userID), TargetEmail: audit.Ptr(email)}); err != nil {
		return "", err
	}
	return email, tx.Commit(ctx)
}
