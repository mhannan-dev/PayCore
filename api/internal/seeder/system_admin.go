package seeder

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GrantSystemAdmin explicitly promotes an existing account without resetting its
// password or removing its roles. ADMIN receives every registered permission.
// Authentication, resource ownership and other business rules still apply.
func GrantSystemAdmin(ctx context.Context, pool *pgxpool.Pool, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("email is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var adminRoleID string
	if err = tx.QueryRow(ctx, "SELECT id FROM roles WHERE code = 'ADMIN'").Scan(&adminRoleID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('paycore.user_roles'))"); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET
        roles = CASE WHEN 'ADMIN' = ANY(roles) THEN roles ELSE array_append(roles, 'ADMIN') END,
        is_active = TRUE, updated_at = NOW()
        WHERE email = $1`, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("target user does not exist; no access was changed")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id)
        SELECT u.id, r.id FROM users u CROSS JOIN roles r
        WHERE u.email = $1 AND r.code = ANY(u.roles)
        ON CONFLICT DO NOTHING`, email); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
        SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
        WHERE r.code = 'ADMIN' ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
