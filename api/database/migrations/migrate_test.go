package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFreshAndLegacyUserSchema(t *testing.T) {
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("set DATABASE_TEST_URL to test fresh/legacy migrations")
	}
	// Generous: this test applies every migration three times (fresh + two legacy
	// schemas) and shares the database with other packages' tests when run with ./...
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	for _, legacyVersion := range []int{0, 1, 2} {
		legacy := legacyVersion > 0
		label := "fresh"
		if legacy {
			label = fmt.Sprintf("legacy_version_%d", legacyVersion)
		}
		t.Run(label, func(t *testing.T) {
			var nonce [8]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				t.Fatal(err)
			}
			schema := "migration_test_" + hex.EncodeToString(nonce[:])
			identifier := pgx.Identifier{schema}.Sanitize()
			if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
					t.Error("test schema cleanup failed")
				}
			}()
			cfg, err := pgxpool.ParseConfig(dsn)
			if err != nil {
				t.Fatal(err)
			}
			cfg.ConnConfig.RuntimeParams["search_path"] = schema
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			if legacy {
				_, err = pool.Exec(ctx, "CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT NOW()); INSERT INTO schema_migrations(version) VALUES (1); CREATE TABLE users (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80), role TEXT NOT NULL DEFAULT 'LEAD_OPERATOR' CHECK (role IN ('OPERATOR', 'LEAD_OPERATOR', 'COMPLIANCE_AUDITOR', 'STAFF_ENGINEER'))); INSERT INTO users(name) VALUES ('Existing Display Name')")
				if err != nil {
					t.Fatal(err)
				}
				// Authentication predates these profile-field upgrades. Include its
				// existing session table in the legacy fixture, not only users.
				if _, err = pool.Exec(ctx, `CREATE TABLE auth_sessions (
				 token_hash BYTEA PRIMARY KEY CHECK(octet_length(token_hash)=32),
				 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),expires_at TIMESTAMPTZ NOT NULL,
				 CHECK(expires_at>created_at))`); err != nil {
					t.Fatal(err)
				}
				if legacyVersion == 2 {
					if _, err = pool.Exec(ctx, "UPDATE users SET role = 'STAFF_ENGINEER'"); err != nil {
						t.Fatal(err)
					}
					profileSQL, err := sqlFiles.ReadFile("002_user_profile_fields.sql")
					if err != nil {
						t.Fatal(err)
					}
					if _, err = pool.Exec(ctx, string(profileSQL)); err != nil {
						t.Fatal(err)
					}
					if _, err = pool.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES (2)"); err != nil {
						t.Fatal(err)
					}
					if _, err = pool.Exec(ctx, "UPDATE users SET photo = 'https://example.test/legacy-photo.png'"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if legacy {
				if err := Apply(ctx, pool); !errors.Is(err, ErrLegacyRoleReview) {
					t.Fatalf("unreviewed legacy promotion must be blocked: %v", err)
				}
				var version int
				if err := pool.QueryRow(ctx, "SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != legacyVersion {
					t.Fatalf("blocked upgrade must roll back migrations: version=%d error=%v", version, err)
				}
				// Explicitly choose the conservative operational role in the old
				// schema; migration 026 will rename it to ACCOUNTANT.
				if _, err := pool.Exec(ctx, "UPDATE users SET role = 'OPERATOR'"); err != nil {
					t.Fatal(err)
				}
			}
			if err := Apply(ctx, pool); err != nil {
				t.Fatal(err)
			}
			if err := Apply(ctx, pool); err != nil {
				t.Fatal("reapplying migration failed:", err)
			}
			if !legacy {
				_, err = pool.Exec(ctx, "INSERT INTO users(first_name, email, password_hash) VALUES ('New User', 'new@example.test', 'test-fixture-hash')")
				if err != nil {
					t.Fatal(err)
				}
			}
			var firstName string
			var lastName *string
			if err := pool.QueryRow(ctx, "SELECT first_name, last_name FROM users LIMIT 1").Scan(&firstName, &lastName); err != nil {
				t.Fatal(err)
			}
			expectedFirstName := "New User"
			if legacy {
				expectedFirstName = "Existing Display Name"
			}
			if firstName != expectedFirstName || lastName != nil {
				t.Fatal("name preservation or nullable last_name failed")
			}
			if legacyVersion == 2 {
				var legacyPhoto string
				if err := pool.QueryRow(ctx, "SELECT a.external_url FROM users u JOIN attachments a ON a.id = u.photo_attachment_id AND a.owner_user_id = u.id").Scan(&legacyPhoto); err != nil || legacyPhoto != "https://example.test/legacy-photo.png" {
					t.Fatal("legacy photo URL not preserved in attachments")
				}
			}
			var roles []string
			if err := pool.QueryRow(ctx, "SELECT roles FROM users LIMIT 1").Scan(&roles); err != nil {
				t.Fatal(err)
			}
			expectedRole := "ACCOUNTANT"
			if len(roles) != 1 || roles[0] != expectedRole {
				t.Fatal("default role or legacy role migration failed")
			}
			for _, invalid := range []string{
				"ARRAY[]::TEXT[]", "ARRAY['UNKNOWN']", "ARRAY['ADMIN','ADMIN']", "ARRAY['ADMIN',NULL]",
				"ARRAY['OPERATOR']", "ARRAY['LEAD_OPERATOR']", "ARRAY['STAFF_ENGINEER']",
			} {
				if _, err := pool.Exec(ctx, "UPDATE users SET roles = "+invalid); err == nil {
					t.Fatal("invalid role array accepted:", invalid)
				}
			}
			if _, err := pool.Exec(ctx, "UPDATE users SET roles = ARRAY['ADMIN','ACCOUNTANT']"); err != nil {
				t.Fatal("multiple valid roles rejected:", err)
			}
			var nameExists bool
			if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'name')").Scan(&nameExists); err != nil || nameExists {
				t.Fatal("redundant name column still exists")
			}
			if _, err := pool.Exec(ctx, "UPDATE users SET last_name = 'Family'"); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, "UPDATE users SET last_name = NULL"); err != nil {
				t.Fatal("nullable last_name rejected")
			}
			// Multiple addresses and contact numbers per user, one primary of each.
			const userID = "(SELECT id FROM users LIMIT 1)"
			for _, valid := range []string{
				"INSERT INTO user_addresses(user_id, line1, city, country_code, is_primary) VALUES (" + userID + ", '1 Main St', 'Tokyo', 'JP', TRUE)",
				"INSERT INTO user_addresses(user_id, label, line1, city, country_code) VALUES (" + userID + ", 'WORK', '2 Side St', 'Osaka', 'JP')",
				"INSERT INTO user_contact_numbers(user_id, phone_number, is_primary) VALUES (" + userID + ", '+81312345678', TRUE)",
				"INSERT INTO user_contact_numbers(user_id, label, phone_number) VALUES (" + userID + ", 'WORK', '+81612345678')",
			} {
				if _, err := pool.Exec(ctx, valid); err != nil {
					t.Fatal("valid address/contact rejected:", err)
				}
			}
			for _, invalid := range []string{
				"INSERT INTO user_addresses(user_id, line1, city, country_code, is_primary) VALUES (" + userID + ", '3 St', 'Kyoto', 'JP', TRUE)",
				"INSERT INTO user_addresses(user_id, line1, city, country_code) VALUES (" + userID + ", '3 St', 'Kyoto', 'jp')",
				"INSERT INTO user_addresses(user_id, label, line1, city, country_code) VALUES (" + userID + ", 'VACATION', '3 St', 'Kyoto', 'JP')",
				"INSERT INTO user_contact_numbers(user_id, phone_number, is_primary) VALUES (" + userID + ", '+81900000000', TRUE)",
				"INSERT INTO user_contact_numbers(user_id, phone_number) VALUES (" + userID + ", '+81312345678')",
				"INSERT INTO user_contact_numbers(user_id, phone_number) VALUES (" + userID + ", '03-1234-5678')",
			} {
				if _, err := pool.Exec(ctx, invalid); err == nil {
					t.Fatal("invalid address/contact accepted:", invalid)
				}
			}
			var addresses, numbers int
			if _, err := pool.Exec(ctx, "DELETE FROM users"); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM user_addresses), (SELECT count(*) FROM user_contact_numbers)").Scan(&addresses, &numbers); err != nil || addresses != 0 || numbers != 0 {
				t.Fatal("addresses/contacts were not removed with their user")
			}
		})
	}
}
