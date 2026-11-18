package seeder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"paycore/api/migrations"
)

func TestUsersIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("set DATABASE_TEST_URL for isolated seeder integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var nonce [8]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "seeder_test_" + hex.EncodeToString(nonce[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
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
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	password := "Seeder-integration-test-password"
	results, err := Users(ctx, pool, password)
	if err != nil || len(results) != 4 {
		t.Fatal("initial seeding failed")
	}
	for i, result := range results {
		if !result.Inserted {
			t.Fatal("missing new user")
		}
		fixture := SampleUsers()[i]
		if result.FirstName != fixture.FirstName || (result.LastName == nil) != (fixture.LastName == nil) {
			t.Fatal("seeder result is missing first_name/last_name")
		}
		if result.LastName != nil && *result.LastName != *fixture.LastName {
			t.Fatal("seeder result has incorrect last_name")
		}
	}
	var firstHash, operatorHash string
	var nullableLast, nullablePhoto *string
	fixtures := SampleUsers()
	adminEmail, operatorEmail := fixtures[0].Email, fixtures[1].Email
	var adminRoles []string
	if err = pool.QueryRow(ctx, "SELECT password_hash, roles FROM users WHERE email = $1", adminEmail).Scan(&firstHash, &adminRoles); err != nil {
		t.Fatal(err)
	}
	if len(adminRoles) != 2 || adminRoles[0] != "ADMIN" || adminRoles[1] != "OPERATOR" {
		t.Fatal("multi-role admin seed failed")
	}
	if err = bcrypt.CompareHashAndPassword([]byte(firstHash), []byte(password)); err != nil {
		t.Fatal("seed password is not correctly hashed")
	}
	if err = pool.QueryRow(ctx, "SELECT password_hash, last_name, photo_attachment_id FROM users WHERE email = $1", operatorEmail).Scan(&operatorHash, &nullableLast, &nullablePhoto); err != nil {
		t.Fatal(err)
	}
	if firstHash == operatorHash || nullableLast != nil || nullablePhoto != nil {
		t.Fatal("unique password salts or nullable profile fields failed")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET first_name = 'Edited', roles = ARRAY['OPERATOR'], is_active = FALSE WHERE email = $1", adminEmail); err != nil {
		t.Fatal(err)
	}
	results, err = Users(ctx, pool, "Different-password-on-rerun")
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Inserted {
			t.Fatal("rerun created duplicate user")
		}
		if result.Email == adminEmail && (result.FirstName != "Edited" || len(result.Roles) != 1 || result.Roles[0] != "OPERATOR") {
			t.Fatal("skipped result must show existing profile and roles")
		}
	}
	var firstName, unchangedHash string
	var roles []string
	var active bool
	if err = pool.QueryRow(ctx, "SELECT first_name, roles, is_active, password_hash FROM users WHERE email = $1", adminEmail).Scan(&firstName, &roles, &active, &unchangedHash); err != nil {
		t.Fatal(err)
	}
	if firstName != "Edited" || len(roles) != 1 || roles[0] != "OPERATOR" || active || unchangedHash != firstHash {
		t.Fatal("rerun changed existing user data")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO users (first_name, email, password_hash, roles, is_active)
        VALUES ('Unrelated', 'unrelated@example.test', $1, ARRAY['OPERATOR'], FALSE)`, firstHash); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth_sessions (user_id, token_hash, expires_at)
        SELECT id, decode(repeat('01',32),'hex'), NOW() + interval '1 hour' FROM users WHERE email = $1`, adminEmail); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth_sessions (user_id, token_hash, expires_at)
        SELECT id, decode(repeat('02',32),'hex'), NOW() + interval '1 hour' FROM users WHERE email = 'unrelated@example.test'`); err != nil {
		t.Fatal(err)
	}
	developmentPassword := "Dev@12345"
	if _, err = RefreshUsers(ctx, pool, developmentPassword); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT first_name, roles, is_active, password_hash FROM users WHERE email = $1", adminEmail).Scan(&firstName, &roles, &active, &unchangedHash); err != nil {
		t.Fatal(err)
	}
	if firstName != fixtures[0].FirstName || len(roles) != 2 || !active || bcrypt.CompareHashAndPassword([]byte(unchangedHash), []byte(developmentPassword)) != nil {
		t.Fatal("fixture refresh failed")
	}
	var sessions int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM auth_sessions s JOIN users u ON u.id = s.user_id WHERE u.email = $1", adminEmail).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("fixture sessions not revoked")
	}
	if err = pool.QueryRow(ctx, "SELECT first_name, is_active, password_hash FROM users WHERE email = 'unrelated@example.test'").Scan(&firstName, &active, &unchangedHash); err != nil {
		t.Fatal(err)
	}
	if firstName != "Unrelated" || active || unchangedHash != firstHash {
		t.Fatal("unrelated user changed by refresh")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM auth_sessions").Scan(&sessions); err != nil || sessions != 1 {
		t.Fatal("unrelated session was revoked")
	}
	// Explicit administrator promotion preserves credentials and existing roles.
	var beforeGrantHash string
	if err = pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE email=$1", operatorEmail).Scan(&beforeGrantHash); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "INSERT INTO permissions(code,name) VALUES ('custom:manage','Custom permission')"); err != nil {
		t.Fatal(err)
	}
	if err = GrantSystemAdmin(ctx, pool, "missing@example.test"); err == nil {
		t.Fatal("missing user promotion must fail")
	}
	for i := 0; i < 2; i++ {
		if err = GrantSystemAdmin(ctx, pool, operatorEmail); err != nil {
			t.Fatal(err)
		}
	}
	var promotedRoles []string
	if err = pool.QueryRow(ctx, "SELECT password_hash, roles FROM users WHERE email=$1", operatorEmail).Scan(&unchangedHash, &promotedRoles); err != nil {
		t.Fatal(err)
	}
	if unchangedHash != beforeGrantHash || len(promotedRoles) != 2 || promotedRoles[0] != "OPERATOR" || promotedRoles[1] != "ADMIN" {
		t.Fatal("promotion changed password, removed roles or duplicated ADMIN")
	}
	var missingPermissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM permissions p WHERE NOT EXISTS (
        SELECT 1 FROM role_permissions rp JOIN user_roles ur ON ur.role_id=rp.role_id
        JOIN users u ON u.id=ur.user_id WHERE u.email=$1 AND rp.permission_id=p.id)`, operatorEmail).Scan(&missingPermissions); err != nil || missingPermissions != 0 {
		t.Fatal("system administrator is missing permissions")
	}
	// New permissions are included when RBAC is seeded again.
	if _, err = pool.Exec(ctx, "INSERT INTO permissions(code,name) VALUES ('custom:new','New custom permission')"); err != nil {
		t.Fatal(err)
	}
	if err = RBAC(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var granted bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE r.code='ADMIN' AND p.code='custom:new')`).Scan(&granted); err != nil || !granted {
		t.Fatal("RBAC seeding did not grant custom permission to ADMIN")
	}
	// Force the fourth insert to fail and confirm the earlier three roll back.
	if _, err = pool.Exec(ctx, "DELETE FROM users; ALTER TABLE users ADD CONSTRAINT test_reject_engineer CHECK (NOT ('STAFF_ENGINEER' = ANY(roles)))"); err != nil {
		t.Fatal(err)
	}
	if _, err = Users(ctx, pool, password); err == nil {
		t.Fatal("expected seed insert failure")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed seed left partial users")
	}
	if _, err = Users(ctx, pool, "short"); err == nil {
		t.Fatal("short seed password accepted")
	}
}
