package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

func TestMFAPausePreservesEnrollmentAndReenablingRejectsPasswordSession(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	enforced := repositories.NewUserRepository(pool)
	hash, err := services.HashPassword("Admin#1234")
	if err != nil {
		t.Fatal(err)
	}
	user, err := enforced.Create(ctx, "Paused", nil, "mfa-paused@example.test", hash, []string{models.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an existing enrollment and unused recovery code; paused operations
	// must preserve both without pretending to verify an authenticator code.
	if _, err = pool.Exec(ctx, `INSERT INTO user_mfa(user_id,encrypted_secret,last_counter,enabled_at) VALUES($1,$2,123,now())`, user.ID, []byte("retained encrypted secret")); err != nil {
		t.Fatal(err)
	}
	code := sha256.Sum256([]byte("unused recovery code"))
	if _, err = pool.Exec(ctx, `INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)`, user.ID, code[:]); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	digest := sha256.Sum256([]byte(token))
	if err = enforced.CreateSession(ctx, user.ID, hash, digest[:], time.Now().Add(time.Hour)); !errors.Is(err, models.ErrMFARequired) {
		t.Fatal("default policy allowed password-only session")
	}
	paused := repositories.NewUserRepositoryWithMFAPolicy(pool, false)
	if err = paused.CreateSession(ctx, user.ID, hash, digest[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = paused.FindSession(ctx, digest[:]); err != nil {
		t.Fatal(err)
	}
	var verified bool
	if err = pool.QueryRow(ctx, `SELECT mfa_verified FROM auth_sessions WHERE token_hash=$1`, digest[:]).Scan(&verified); err != nil || verified {
		t.Fatal("password session falsely marked MFA verified")
	}
	if _, err = enforced.FindSession(ctx, digest[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("re-enabling MFA accepted password-only admin session")
	}
	security := services.NewSecurityServiceWithMFAPolicy(repositories.NewSecurityRepositoryWithMFAPolicy(pool, false), nil, false)
	overview, err := security.Overview(ctx, user.ID, token)
	if err != nil || overview.MFAEnforced || overview.MFARequired || !overview.MFAEnabled || len(overview.Sessions) != 1 {
		t.Fatalf("paused overview %+v %v", overview, err)
	}
	// Revocation still verifies the password but no longer requires a TOTP code.
	second := sha256.Sum256([]byte("another password-only session"))
	if err = paused.CreateSession(ctx, user.ID, hash, second[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	result, err := security.Revoke(ctx, user.ID, token, "", true, models.SecurityProof{Password: "Admin#1234"})
	if err != nil || result.RevokedCount != 1 {
		t.Fatalf("password-only revoke: %+v %v", result, err)
	}
	if _, err = security.Regenerate(ctx, user.ID, token, models.SecurityProof{Password: "Admin#1234"}); !errors.Is(err, models.ErrMFADisabled) {
		t.Fatal("paused recovery settings changed")
	}
	var secret []byte
	var counter, codes int
	if err = pool.QueryRow(ctx, `SELECT encrypted_secret,last_counter,(SELECT count(*) FROM mfa_recovery_codes WHERE user_id=$1) FROM user_mfa WHERE user_id=$1`, user.ID).Scan(&secret, &counter, &codes); err != nil || string(secret) != "retained encrypted secret" || counter != 123 || codes != 1 {
		t.Fatal("pause modified enrollment or recovery state")
	}
	if _, err = pool.Exec(ctx, `UPDATE users SET roles=ARRAY['ACCOUNTANT']::text[] WHERE id=$1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = enforced.FindSession(ctx, digest[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("re-enabling MFA accepted password-only enrolled non-admin session")
	}
}
