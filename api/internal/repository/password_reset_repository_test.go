package repository_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/database"
	"paycore/api/internal/repository"
	"paycore/api/migrations"
)

func TestPasswordResetLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		t.Skip("set DATABASE_TEST_URL for password reset integration")
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
	resets := repository.NewPasswordResetRepository(pool)
	invites := repository.NewInvitationRepository(pool)
	email := fmt.Sprintf("reset-%d@example.test", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", email) })
	digest := func(s string) []byte { d := sha256.Sum256([]byte(s)); return d[:] }

	// An invited user with an open invitation and a live session.
	u, err := invites.CreateInvitedUser(ctx, "Reset", nil, email, []string{"OPERATOR"}, digest(email+"invite"), time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO auth_sessions (token_hash, user_id, expires_at) VALUES ($1, $2, NOW() + interval '1 hour')", digest(email+"session"), u.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := resets.IssueResetToken(ctx, "nobody-"+email, digest("x"), time.Now().Add(time.Hour), time.Minute); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown email: %v", err)
	}
	if got, err := resets.IssueResetToken(ctx, email, digest(email+"1"), time.Now().Add(time.Hour), time.Minute); err != nil || got.ID != u.ID || got.FirstName != "Reset" {
		t.Fatalf("issue: %+v %v", got, err)
	}
	if _, err := resets.IssueResetToken(ctx, email, digest(email+"2"), time.Now().Add(time.Hour), time.Minute); !errors.Is(err, repository.ErrResetThrottled) {
		t.Fatalf("cooldown: %v", err)
	}
	// Without a cooldown, a new link replaces the old one.
	if _, err := resets.IssueResetToken(ctx, email, digest(email+"3"), time.Now().Add(time.Hour), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := resets.FindResetToken(ctx, digest(email+"1")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("old link should be replaced: %v", err)
	}
	if info, err := resets.FindResetToken(ctx, digest(email+"3")); err != nil || info.Email != email {
		t.Fatalf("inspect: %+v %v", info, err)
	}

	if got, err := resets.ResetPassword(ctx, digest(email+"3"), "$2a$12$hash"); err != nil || got != email {
		t.Fatalf("reset: %q %v", got, err)
	}
	var sessions, openInvites int
	var hasPassword bool
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM auth_sessions WHERE user_id = $1),
		(SELECT count(*) FROM user_invitations WHERE user_id = $1 AND used_at IS NULL),
		(SELECT password_hash IS NOT NULL FROM users WHERE id = $1)`, u.ID).Scan(&sessions, &openInvites, &hasPassword); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || openInvites != 0 || !hasPassword {
		t.Fatalf("after reset: sessions=%d openInvites=%d password=%t", sessions, openInvites, hasPassword)
	}
	if _, err := resets.ResetPassword(ctx, digest(email+"3"), "$2a$12$other"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("reuse must fail: %v", err)
	}

	// Expired links do not work.
	if _, err := resets.IssueResetToken(ctx, email, digest(email+"4"), time.Now().Add(time.Hour), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE password_reset_tokens SET created_at = NOW() - interval '2 hours', expires_at = NOW() - interval '1 second' WHERE token_hash = $1", digest(email+"4")); err != nil {
		t.Fatal(err)
	}
	if _, err := resets.ResetPassword(ctx, digest(email+"4"), "$2a$12$hash"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired: %v", err)
	}
}
