package repositories_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"paycore/api/internal/mfa"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

// Simulates the authenticator app from the enrollment key, not a server helper.
func appCode(t *testing.T, secret string) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], uint64(time.Now().Unix()/30))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(message[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
}

func TestMFAEnrollmentLoginAndRecoveryConcurrency(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	hash, err := services.HashPassword("Admin#1234")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(ctx, "Admin", nil, "mfa@example.test", hash, []string{models.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	codec, err := mfa.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	second := services.NewMFAService(repositories.NewMFARepository(pool), codec)
	second.Audit = repositories.NewAuditRepository(pool)
	auth, err := services.NewAuthService(users, acceptingChallenge{}, second)
	if err != nil {
		t.Fatal(err)
	}
	login, err := auth.Login(ctx, models.LoginRequest{Email: u.Email, Password: "Admin#1234"})
	if err != nil {
		t.Fatal(err)
	}
	if login.MFA == nil || login.MFA.Enrollment == nil || login.Token != "" {
		t.Fatal("password login bypassed mandatory enrollment")
	}
	challenge := login.MFA
	secret := challenge.Enrollment.Secret
	if _, err = auth.CurrentUser(ctx, challenge.ChallengeToken); !errors.Is(err, services.ErrSession) {
		t.Fatal("challenge token acted as bearer session")
	}
	enrollmentCode := appCode(t, secret)
	result, err := second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: enrollmentCode})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RecoveryCodes) != 8 || len(result.Session.Token) != 64 {
		t.Fatal("enrollment did not issue recovery codes/session")
	}
	if _, err = auth.CurrentUser(ctx, result.Session.Token); err != nil {
		t.Fatal("MFA session rejected")
	}
	if _, err = second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: appCode(t, secret)}); !errors.Is(err, models.ErrMFAChallenge) {
		t.Fatal("consumed challenge replayed")
	}
	var encrypted []byte
	var count int
	if err = pool.QueryRow(ctx, "SELECT encrypted_secret FROM user_mfa WHERE user_id=$1", u.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), secret) {
		t.Fatal("unencrypted secret stored")
	}
	next, err := auth.Login(ctx, models.LoginRequest{Email: u.Email, Password: "Admin#1234"})
	if err != nil {
		t.Fatal(err)
	}
	if next.MFA == nil || next.MFA.Enrollment != nil {
		t.Fatal("enabled secret was disclosed again")
	}
	if _, err = second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: next.MFA.ChallengeToken, Code: enrollmentCode}); !errors.Is(err, models.ErrMFACode) {
		t.Fatal("TOTP time step reused on a new challenge")
	}
	// Two workers race to use one recovery code and challenge; only one can commit.
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: next.MFA.ChallengeToken, Code: result.RecoveryCodes[0]})
			outcomes <- err
		}()
	}
	wg.Wait()
	close(outcomes)
	successes := 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else if !errors.Is(err, models.ErrMFAChallenge) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("recovery committed %d times", successes)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM mfa_recovery_codes WHERE user_id=$1", u.ID).Scan(&count); err != nil || count != 7 {
		t.Fatal("recovery code not consumed atomically")
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action IN ('mfa_enrolled','mfa_recovery_used')").Scan(&count); err != nil || count != 2 {
		t.Fatal("MFA audit events missing")
	}
	// Plain session issuance cannot bypass MFA even with the correct hash.
	digest := sha256.Sum256([]byte("bypass"))
	if err = users.CreateSession(ctx, u.ID, u.PasswordHash, digest[:], time.Now().Add(time.Hour)); !errors.Is(err, models.ErrMFARequired) {
		t.Fatal("password-only session issued to enrolled admin")
	}
	// A password reset does not disable MFA, and invalidates pending challenges.
	last, err := auth.Login(ctx, models.LoginRequest{Email: u.Email, Password: "Admin#1234"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: last.MFA.ChallengeToken, Code: result.RecoveryCodes[0]}); !errors.Is(err, models.ErrMFACode) {
		t.Fatal("consumed recovery code reused with a fresh challenge")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET password_hash='changed' WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: last.MFA.ChallengeToken, Code: result.RecoveryCodes[1]}); !errors.Is(err, models.ErrMFAChallenge) {
		t.Fatal("old-password challenge survived password change")
	}
}

func TestMFALockoutExpiryAndPromotion(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	u, err := users.Create(ctx, "Promotion", nil, "promotion@example.test", "fixture hash", []string{models.RoleAccountant})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("c", 64)
	digest := sha256.Sum256([]byte(token))
	if err = users.CreateSession(ctx, u.ID, u.PasswordHash, digest[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	codec, err := mfa.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	second := services.NewMFAService(repositories.NewMFARepository(pool), codec)
	if _, required, err := second.Begin(ctx, u); err != nil || required {
		t.Fatal("unenrolled non-admin unexpectedly required MFA")
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET roles=ARRAY['ADMIN']::TEXT[] WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = users.FindSession(ctx, digest[:]); err == nil {
		t.Fatal("promoted admin reused password-only session")
	}
	u.Roles = []string{models.RoleAdmin}
	challenge, _, err := second.Begin(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "UPDATE mfa_challenges SET expires_at=NOW()-INTERVAL '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err = second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: "abcdef"}); !errors.Is(err, models.ErrMFAChallenge) {
		t.Fatal("expired challenge accepted")
	}
	for i := 0; i < 5; i++ {
		// A fresh challenge must not reset the account's failed-attempt counter.
		challenge, _, err = second.Begin(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		_, err = second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: "abcdef"})
		want := models.ErrMFACode
		if i == 4 {
			want = models.ErrMFALocked
		}
		if !errors.Is(err, want) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, _, err = second.Begin(ctx, u); !errors.Is(err, models.ErrMFALocked) {
		t.Fatal("new challenge bypassed account lockout")
	}
}
