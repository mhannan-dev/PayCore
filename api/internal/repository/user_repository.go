package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

var ErrInactiveUser = errors.New("inactive user")

type AuthStore interface {
	FindByEmail(context.Context, string) (model.User, error)
	CreateSession(context.Context, string, []byte, time.Time) error
	FindSession(context.Context, []byte) (model.User, error)
	DeleteSession(context.Context, []byte) error
	UpdateName(ctx context.Context, userID, firstName string, lastName *string) (model.User, error)
}

type UserRepository struct{ pool *pgxpool.Pool }

func NewUserRepository(pool *pgxpool.Pool) *UserRepository { return &UserRepository{pool: pool} }

type scanner interface{ Scan(...any) error }

// password_hash is NULL for invited users who have not set a password yet.
const userColumns = `u.id, u.first_name, u.last_name, u.email, COALESCE(u.password_hash, ''), u.roles, u.is_active,
    CASE WHEN a.storage_kind = 'EXTERNAL' THEN a.external_url
         WHEN a.storage_kind = 'LOCAL' THEN '/api/v1/auth/photo' ELSE NULL END,
    a.id, u.created_at, u.updated_at, ` + userPermissions

// userPermissions lists the distinct permission codes granted by the user's current roles.
const userPermissions = `ARRAY(SELECT DISTINCT p.code FROM roles r
    JOIN role_permissions rp ON rp.role_id = r.id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE r.code = ANY(u.roles) ORDER BY p.code)`
const userPhotoJoin = " LEFT JOIN attachments a ON a.id = u.photo_attachment_id AND a.owner_user_id = u.id AND a.deleted_at IS NULL "

func scanUser(row scanner) (u model.User, err error) {
	err = row.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Email, &u.PasswordHash, &u.Roles, &u.IsActive, &u.Photo, &u.PhotoAttachmentID, &u.CreatedAt, &u.UpdatedAt, &u.Permissions)
	return
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (model.User, error) {
	return scanUser(r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users u "+userPhotoJoin+" WHERE u.email = $1", email))
}
func (r *UserRepository) Create(ctx context.Context, firstName string, lastName *string, email, hash string, roles []string) (model.User, error) {
	roles, err := model.NormalizeRoles(roles)
	if err != nil {
		return model.User{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback(ctx)

	u, err := scanUser(tx.QueryRow(ctx, "INSERT INTO users (first_name, last_name, email, password_hash, roles) VALUES ($1, $2, $3, $4, $5) RETURNING id, first_name, last_name, email, password_hash, roles, is_active, NULL::TEXT, photo_attachment_id, created_at, updated_at, ARRAY[]::TEXT[]", firstName, lastName, email, hash, roles))
	if err != nil {
		return model.User{}, err
	}

	for _, roleCode := range roles {
		if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = $2 ON CONFLICT DO NOTHING", u.ID, roleCode); err != nil {
			return model.User{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.User{}, err
	}
	return u, nil
}
func (r *UserRepository) CreateSession(ctx context.Context, userID string, hash []byte, expiry time.Time) error {
	// A concurrent account disable cannot issue a usable new session.
	tag, err := r.pool.Exec(ctx, "INSERT INTO auth_sessions (token_hash, user_id, expires_at) SELECT $1, id, $3 FROM users WHERE id = $2 AND is_active", hash, userID, expiry)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrInactiveUser
	}
	return err
}
func (r *UserRepository) FindSession(ctx context.Context, hash []byte) (model.User, error) {
	return scanUser(r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users u JOIN auth_sessions s ON s.user_id = u.id "+userPhotoJoin+" WHERE s.token_hash = $1 AND s.expires_at > NOW() AND u.is_active", hash))
}

// UpdateName changes only the name fields of an active user and returns the updated row.
func (r *UserRepository) UpdateName(ctx context.Context, userID, firstName string, lastName *string) (model.User, error) {
	return scanUser(r.pool.QueryRow(ctx, `WITH updated AS (
            UPDATE users SET first_name = $2, last_name = $3, updated_at = NOW()
            WHERE id = $1 AND is_active
            RETURNING *)
        SELECT `+userColumns+` FROM updated u `+userPhotoJoin, userID, firstName, lastName))
}
func (r *UserRepository) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM auth_sessions WHERE token_hash = $1", hash)
	return err
}
