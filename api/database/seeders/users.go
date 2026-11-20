package seeders

import (
	"context"
	"errors"
	"paycore/api/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type UserSeed struct {
	FirstName string   `json:"first_name"`
	LastName  *string  `json:"last_name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
}

type UserResult struct {
	FirstName string   `json:"first_name"`
	LastName  *string  `json:"last_name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
	Inserted  bool     `json:"inserted"`
	Refreshed bool     `json:"refreshed"`
}

// SampleUsers returns development fixtures; it does not create live identities.
func SampleUsers() []UserSeed {
	last := func(value string) *string { return &value }
	return []UserSeed{
		{FirstName: "Admin", LastName: last("Admin"), Email: "admin@yopmail.com", Roles: []string{"ADMIN", "OPERATOR"}},
		{FirstName: "Operator", LastName: nil, Email: "operator@yopmail.com", Roles: []string{"OPERATOR"}},
		{FirstName: "Auditor", LastName: last("Auditor"), Email: "auditor@yopmail.com", Roles: []string{"COMPLIANCE_AUDITOR"}},
		{FirstName: "Engineer", LastName: last("Engineer"), Email: "engineer@yopmail.com", Roles: []string{"STAFF_ENGINEER"}},
	}
}

// Users inserts all missing sample users atomically. Existing users, passwords,
// roles, profile fields and active flags are never changed by a rerun.
func Users(ctx context.Context, pool *pgxpool.Pool, password string) ([]UserResult, error) {
	return seedUsers(ctx, pool, password, false)
}

// RefreshUsers explicitly resets only fixture emails and revokes their sessions.
func RefreshUsers(ctx context.Context, pool *pgxpool.Pool, password string) ([]UserResult, error) {
	return seedUsers(ctx, pool, password, true)
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool, password string, refresh bool) ([]UserResult, error) {
	// This exception is confined to explicit development seeding. Normal account
	// provisioning continues to require a password of at least 12 bytes.
	if len(password) < 8 || len(password) > 72 {
		return nil, errors.New("seed password must be 8 to 72 bytes")
	}
	fixtures := SampleUsers()
	hashes := make([]string, len(fixtures))
	for i := range fixtures {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), services.PasswordCost)
		if err != nil {
			return nil, err
		}
		hashes[i] = string(hash)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	results := make([]UserResult, 0, len(fixtures))
	for i, user := range fixtures {
		query := `INSERT INTO users (first_name, last_name, email, password_hash, roles)
            VALUES ($1, $2, $3, $4, $5) ON CONFLICT (email) DO NOTHING`
		if refresh {
			query = `INSERT INTO users (first_name, last_name, email, password_hash, roles)
                VALUES ($1, $2, $3, $4, $5) ON CONFLICT (email) DO UPDATE SET
                first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
                photo_attachment_id = NULL, password_hash = EXCLUDED.password_hash, roles = EXCLUDED.roles,
                is_active = TRUE, updated_at = NOW()`
		}
		tag, err := tx.Exec(ctx, query, user.FirstName, user.LastName, user.Email, hashes[i], user.Roles)
		if err != nil {
			return nil, err
		}
		if refresh {
			if _, err = tx.Exec(ctx, "DELETE FROM auth_sessions WHERE user_id = (SELECT id FROM users WHERE email = $1)", user.Email); err != nil {
				return nil, err
			}
		}
		result := UserResult{Email: user.Email, Inserted: !refresh && tag.RowsAffected() == 1, Refreshed: refresh}
		// Sync user_roles junction table for this user
		syncJunctionQuery := `
			INSERT INTO user_roles (user_id, role_id)
			SELECT u.id, r.id
			FROM users u
			CROSS JOIN LATERAL unnest(u.roles) AS role_code
			JOIN roles r ON r.code = role_code
			WHERE u.email = $1
			ON CONFLICT DO NOTHING
		`
		if _, err = tx.Exec(ctx, syncJunctionQuery, user.Email); err != nil {
			return nil, err
		}
		// Report persisted values even when an existing user's fixture was skipped.
		if err = tx.QueryRow(ctx, "SELECT first_name, last_name, roles FROM users WHERE email = $1", user.Email).Scan(&result.FirstName, &result.LastName, &result.Roles); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return results, nil
}
