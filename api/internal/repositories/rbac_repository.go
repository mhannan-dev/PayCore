package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

type RBACRepository struct {
	pool *pgxpool.Pool
}

func NewRBACRepository(pool *pgxpool.Pool) *RBACRepository {
	return &RBACRepository{pool: pool}
}

// ListRoles returns all defined system roles sorted by code.
func (r *RBACRepository) ListRoles(ctx context.Context) ([]models.Role, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, code, name, COALESCE(description, ''), created_at, updated_at FROM roles ORDER BY code ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// ListPermissions returns all defined system permissions sorted by code.
func (r *RBACRepository) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, code, name, COALESCE(description, ''), created_at, updated_at FROM permissions ORDER BY code ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []models.Permission
	for rows.Next() {
		var p models.Permission
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// FindPermissionsByUserID fetches all distinct permission codes granted to a user across all assigned roles.
func (r *RBACRepository) FindPermissionsByUserID(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT DISTINCT p.code
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id = p.id
		JOIN roles r ON r.id = rp.role_id
		JOIN users u ON r.code = ANY(u.roles)
		WHERE u.id = $1 AND u.is_active
		ORDER BY p.code ASC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		permissions = append(permissions, code)
	}
	return permissions, rows.Err()
}

// FindRolesByUserID reads the same authoritative role array as authentication.
func (r *RBACRepository) FindRolesByUserID(ctx context.Context, userID string) ([]models.Role, error) {
	query := `
		SELECT r.id, r.code, r.name, COALESCE(r.description, ''), r.created_at, r.updated_at
		FROM roles r
		JOIN users u ON r.code = ANY(u.roles)
		WHERE u.id = $1 AND u.is_active
		ORDER BY r.code ASC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// ErrLastAdmin is returned when a change would leave no active ADMIN.
var ErrLastAdmin = errors.New("at least one active administrator is required")

// ErrUserHasMerchant preserves merchant/payment history when removing an owner.
var ErrUserHasMerchant = errors.New("user owns a Quantum merchant profile")

// ListRoleDetails returns every role with its permission codes and active-user count.
func (r *RBACRepository) ListRoleDetails(ctx context.Context) ([]models.RoleDetail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.code, r.name, COALESCE(r.description, ''), r.created_at, r.updated_at,
			ARRAY(SELECT p.code FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
				WHERE rp.role_id = r.id ORDER BY p.code),
			(SELECT count(*) FROM users u WHERE u.is_active AND r.code = ANY(u.roles))
		FROM roles r
		ORDER BY r.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := []models.RoleDetail{}
	for rows.Next() {
		var d models.RoleDetail
		if err := rows.Scan(&d.ID, &d.Code, &d.Name, &d.Description, &d.CreatedAt, &d.UpdatedAt, &d.Permissions, &d.UserCount); err != nil {
			return nil, err
		}
		roles = append(roles, d)
	}
	return roles, rows.Err()
}

// The invite_* columns describe the user's newest unused invitation (at most one exists).
const managedUserColumns = `id, first_name, last_name, email, roles, is_active, created_at, password_hash IS NOT NULL,
	i.expires_at, i.email_status, i.email_sent_at, i.email_error`

// managedUserFrom joins the newest unused invitation; use it as "FROM " + managedUserFrom.
const managedUserFrom = `users LEFT JOIN LATERAL (
	SELECT expires_at, email_status, email_sent_at, email_error FROM user_invitations
	WHERE user_id = users.id AND used_at IS NULL ORDER BY created_at DESC LIMIT 1) i ON TRUE`

func scanManagedUser(row interface{ Scan(...any) error }) (u models.ManagedUser, err error) {
	err = row.Scan(&u.ID, &u.FirstName, &u.LastName, &u.Email, &u.Roles, &u.IsActive, &u.CreatedAt, &u.PasswordSet, &u.InviteExpiresAt, &u.InviteEmailStatus, &u.InviteEmailSentAt, &u.InviteEmailError)
	return
}

// ListUsers returns one page of users ordered by email, plus how many match in total.
func (r *RBACRepository) ListUsers(ctx context.Context, q models.UserQuery) ([]models.ManagedUser, int, error) {
	// Escape LIKE wildcards so a search for "a_b" or "50%" matches literally.
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.Search) + "%"
	const where = ` WHERE ($1 = '%%' OR (first_name || ' ' || COALESCE(last_name, '') || ' ' || email) ILIKE $1)
		AND ($2 = '' OR $2 = ANY(roles))`
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM users"+where, pattern, q.Role).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+where+" ORDER BY email LIMIT $3 OFFSET $4", pattern, q.Role, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := []models.ManagedUser{}
	for rows.Next() {
		u, err := scanManagedUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// SetUserRoles replaces a user's roles (users.roles and user_roles together) and returns the
// updated account. It returns pgx.ErrNoRows for an unknown user and ErrLastAdmin when the
// change would leave no active administrator.
func (r *RBACRepository) SetUserRoles(ctx context.Context, userID string, roleCodes []string) (models.ManagedUser, error) {
	normalized, err := models.NormalizeRoles(roleCodes)
	if err != nil {
		return models.ManagedUser{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.ManagedUser{}, err
	}
	defer tx.Rollback(ctx)

	// Serialize role changes so two concurrent edits cannot both remove the last admin.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('paycore.user_roles'))"); err != nil {
		return models.ManagedUser{}, err
	}
	var email string
	var before []string
	if err := tx.QueryRow(ctx, "SELECT email, roles FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&email, &before); err != nil {
		return models.ManagedUser{}, notFound(err, ErrUserNotFound)
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET roles = $2, updated_at = NOW() WHERE id = $1", userID, normalized); err != nil {
		return models.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID); err != nil {
		return models.ManagedUser{}, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = ANY($2) ON CONFLICT DO NOTHING", userID, normalized); err != nil {
		return models.ManagedUser{}, err
	}
	var admins int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE is_active AND 'ADMIN' = ANY(roles)").Scan(&admins); err != nil {
		return models.ManagedUser{}, err
	}
	if admins == 0 {
		return models.ManagedUser{}, ErrLastAdmin
	}
	if err := insertAudit(ctx, tx, audit.Event{Action: audit.RolesChanged, TargetID: audit.Ptr(userID), TargetEmail: audit.Ptr(email),
		Details: audit.Details{RolesBefore: before, RolesAfter: normalized}}); err != nil {
		return models.ManagedUser{}, err
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+" WHERE users.id = $1", userID))
	if err != nil {
		return models.ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}

// DeleteUser permanently removes a user; sessions, roles, invitations, contacts and
// attachment records cascade. It returns pgx.ErrNoRows for an unknown user and
// ErrLastAdmin when it would leave no active administrator.
func (r *RBACRepository) DeleteUser(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Same lock as SetUserRoles, so concurrent edits cannot remove the last admin.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('paycore.user_roles'))"); err != nil {
		return err
	}
	var email string
	var roles []string
	if err := tx.QueryRow(ctx, "DELETE FROM users WHERE id = $1 RETURNING email, roles", userID).Scan(&email, &roles); err != nil {
		var constraint *pgconn.PgError
		if errors.As(err, &constraint) && constraint.Code == "23503" && constraint.ConstraintName == "quantum_merchants_owner_user_id_fkey" {
			return ErrUserHasMerchant
		}
		return notFound(err, ErrUserNotFound)
	}
	var admins int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE is_active AND 'ADMIN' = ANY(roles)").Scan(&admins); err != nil {
		return err
	}
	if admins == 0 {
		return ErrLastAdmin
	}
	if err := insertAudit(ctx, tx, audit.Event{Action: audit.UserDeleted, TargetID: audit.Ptr(userID), TargetEmail: audit.Ptr(email),
		Details: audit.Details{RolesBefore: roles}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetUserActive deactivates or reactivates a user. Deactivating signs them out of
// every session and voids their reset links; the account and its history remain.
// Returns pgx.ErrNoRows for an unknown user and ErrLastAdmin when it would leave no
// active administrator. Setting the current state again is a no-op (no audit event).
func (r *RBACRepository) SetUserActive(ctx context.Context, userID string, active bool) (models.ManagedUser, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.ManagedUser{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('paycore.user_roles'))"); err != nil {
		return models.ManagedUser{}, err
	}
	var email string
	var wasActive bool
	if err := tx.QueryRow(ctx, "SELECT email, is_active FROM users WHERE id = $1 FOR UPDATE", userID).Scan(&email, &wasActive); err != nil {
		return models.ManagedUser{}, notFound(err, ErrUserNotFound)
	}
	if wasActive != active {
		if _, err := tx.Exec(ctx, "UPDATE users SET is_active = $2, updated_at = NOW() WHERE id = $1", userID, active); err != nil {
			return models.ManagedUser{}, err
		}
		action := audit.UserActivated
		if !active {
			action = audit.UserDeactivated
			for _, stmt := range []string{
				"DELETE FROM auth_sessions WHERE user_id = $1",
				"DELETE FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL",
			} {
				if _, err := tx.Exec(ctx, stmt, userID); err != nil {
					return models.ManagedUser{}, err
				}
			}
			var admins int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE is_active AND 'ADMIN' = ANY(roles)").Scan(&admins); err != nil {
				return models.ManagedUser{}, err
			}
			if admins == 0 {
				return models.ManagedUser{}, ErrLastAdmin
			}
		}
		if err := insertAudit(ctx, tx, audit.Event{Action: action, TargetID: audit.Ptr(userID), TargetEmail: audit.Ptr(email)}); err != nil {
			return models.ManagedUser{}, err
		}
	}
	u, err := scanManagedUser(tx.QueryRow(ctx, "SELECT "+managedUserColumns+" FROM "+managedUserFrom+" WHERE users.id = $1", userID))
	if err != nil {
		return models.ManagedUser{}, err
	}
	return u, tx.Commit(ctx)
}
