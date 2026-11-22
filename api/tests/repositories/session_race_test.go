package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

type acceptingChallenge struct{}

func (acceptingChallenge) Verify(string, string) bool { return true }

type resetBeforeIssuance struct {
	*repositories.UserRepository
	reset func(context.Context) error
}

func (s *resetBeforeIssuance) CreateSession(ctx context.Context, id, expected string, hash []byte, expiry time.Time) error {
	if err := s.reset(ctx); err != nil {
		return err
	}
	return s.UserRepository.CreateSession(ctx, id, expected, hash, expiry)
}

func TestPasswordResetRejectsAlreadyVerifiedLogin(t *testing.T) {
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	users := repositories.NewUserRepository(pool)
	oldHash, err := services.HashPassword("OldPwd123!")
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := services.HashPassword("NewPwd123!")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(ctx, "Regression", nil, "race@example.test", oldHash, []string{"ACCOUNTANT"})
	if err != nil {
		t.Fatal(err)
	}
	resets := repositories.NewPasswordResetRepository(pool)
	digest := sha256.Sum256([]byte("reset fixture"))
	if _, err = resets.IssueResetToken(ctx, u.Email, digest[:], time.Now().Add(time.Hour), 0); err != nil {
		t.Fatal(err)
	}
	store := &resetBeforeIssuance{UserRepository: users, reset: func(ctx context.Context) error {
		_, err := resets.ResetPassword(ctx, digest[:], newHash)
		return err
	}}
	auth, err := services.NewAuthService(store, acceptingChallenge{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Login(ctx, models.LoginRequest{Email: u.Email, Password: "OldPwd123!"}); !errors.Is(err, services.ErrCredentials) {
		t.Fatalf("stale verified login accepted: %v", err)
	}
	var sessions int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM auth_sessions WHERE user_id=$1", u.ID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("password reset left a newly issued old-password session")
	}
}

func TestSessionIssuanceLocksCredentialSnapshot(t *testing.T) {
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	users := repositories.NewUserRepository(pool)
	u, err := users.Create(ctx, "Lock test", nil, "lock@example.test", "initial", []string{"ACCOUNTANT"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE users SET password_hash='changed' WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("session fixture"))
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		done <- users.CreateSession(ctx, u.ID, "initial", digest[:], time.Now().Add(time.Hour))
	}()
	<-started
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, models.ErrCredentialsChanged) {
		t.Fatalf("stale snapshot survived reset lock: %v", err)
	}
	if err = users.CreateSession(ctx, u.ID, "changed", digest[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	resets := repositories.NewPasswordResetRepository(pool)
	reset := sha256.Sum256([]byte("next reset fixture"))
	if _, err = resets.IssueResetToken(ctx, u.Email, reset[:], time.Now().Add(time.Hour), 0); err != nil {
		t.Fatal(err)
	}
	if _, err = resets.ResetPassword(ctx, reset[:], "newer"); err != nil {
		t.Fatal(err)
	}
	if _, err = users.FindSession(ctx, digest[:]); err == nil {
		t.Fatal("reset failed to revoke the session that committed first")
	}
}
