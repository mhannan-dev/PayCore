package repository_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/database"
	"paycore/api/internal/repository"
	"paycore/api/internal/service"
	"paycore/api/migrations"
)

func TestUserSessionPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("set DATABASE_TEST_URL for user/session integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal("migration must be repeatable:", err)
	}
	repo := repository.NewUserRepository(pool)
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	email := "auth-test-" + hex.EncodeToString(nonce[:]) + "@example.test"
	hash, err := service.HashPassword("Temp#20261")
	if err != nil {
		t.Fatal(err)
	}
	u, err := repo.Create(ctx, "Test operator", nil, email, hash, []string{"OPERATOR", "STAFF_ENGINEER"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", u.ID); err != nil {
			t.Error("test user cleanup failed")
		}
	}()
	if _, err = repo.Create(ctx, "Duplicate", nil, email, hash, []string{"OPERATOR", "STAFF_ENGINEER"}); err == nil {
		t.Fatal("duplicate email accepted")
	}
	found, err := repo.FindByEmail(ctx, email)
	if err != nil || found.PasswordHash != hash || (!found.HasRole("OPERATOR") || !found.HasRole("STAFF_ENGINEER") || len(found.Roles) != 2) {
		t.Fatal("user lookup failed")
	}
	var firstName string
	var lastName, photo *string
	if err = pool.QueryRow(ctx, "SELECT first_name, last_name, photo_attachment_id FROM users WHERE id = $1", u.ID).Scan(&firstName, &lastName, &photo); err != nil {
		t.Fatal(err)
	}
	if firstName != "Test operator" || lastName != nil || photo != nil {
		t.Fatal("optional last_name/photo must default to NULL")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET last_name = 'Example' WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.FindByEmail(ctx, email)
	if err != nil || updated.FirstName != "Test operator" || updated.LastName == nil || *updated.LastName != "Example" {
		t.Fatal("first/last name lookup failed")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET last_name = NULL WHERE id = $1", u.ID); err != nil {
		t.Fatal("nullable last_name rejected:", err)
	}
	family := "Family"
	renamed, err := repo.UpdateName(ctx, u.ID, "Renamed", &family)
	if err != nil || renamed.FirstName != "Renamed" || renamed.LastName == nil || *renamed.LastName != "Family" || renamed.Email != email || renamed.PasswordHash != hash {
		t.Fatal("UpdateName did not return the renamed user:", err)
	}
	if renamed, err = repo.UpdateName(ctx, u.ID, "Test operator", nil); err != nil || renamed.LastName != nil {
		t.Fatal("UpdateName could not clear last_name:", err)
	}
	digest := sha256.Sum256(nonce[:])
	if err = repo.CreateSession(ctx, u.ID, u.PasswordHash, digest[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.FindSession(ctx, digest[:]); err != nil {
		t.Fatal("active session rejected")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET is_active = FALSE WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.FindSession(ctx, digest[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("disabled user session accepted")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET is_active = TRUE WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE auth_sessions SET expires_at = created_at + interval '1 microsecond' WHERE token_hash = $1", digest[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.FindSession(ctx, digest[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("expired session accepted")
	}
	if err = repo.DeleteSession(ctx, digest[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.FindSession(ctx, digest[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("revoked session accepted")
	}
}
