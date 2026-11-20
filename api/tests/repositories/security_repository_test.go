package repositories_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/mfa"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

type securityFixture struct {
	pool     *pgxpool.Pool
	user     models.User
	token    string
	codes    []string
	security *services.SecurityService
	second   *services.MFAService
}

func newSecurityFixture(t *testing.T) securityFixture {
	t.Helper()
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	hash, err := services.HashPassword("Admin#1234")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(ctx, "Security", nil, "security@example.test", hash, []string{models.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	codec, err := mfa.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	second := services.NewMFAService(repositories.NewMFARepository(pool), codec)
	challenge, required, err := second.Begin(ctx, u)
	if err != nil || !required || challenge.Enrollment == nil {
		t.Fatalf("enrollment: %v", err)
	}
	result, err := second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: appCode(t, challenge.Enrollment.Secret)})
	if err != nil {
		t.Fatal(err)
	}
	return securityFixture{pool: pool, user: u, token: result.Session.Token, codes: result.RecoveryCodes, security: services.NewSecurityService(repositories.NewSecurityRepository(pool), codec), second: second}
}
func (f securityFixture) extraSession(t *testing.T, owner, label string) (string, string) {
	t.Helper()
	digest := sha256.Sum256([]byte(label))
	token := hex.EncodeToString(digest[:])
	hash := sha256.Sum256([]byte(token))
	var id string
	if err := f.pool.QueryRow(context.Background(), "INSERT INTO auth_sessions(user_id,token_hash,expires_at,mfa_verified) VALUES($1,$2,NOW()+INTERVAL '1 hour',TRUE) RETURNING id", owner, hash[:]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id, token
}
func (f securityFixture) proof(code string) models.SecurityProof {
	return models.SecurityProof{Password: "Admin#1234", Code: code}
}

func TestSecurityRecoveryAndSessionOwnership(t *testing.T) {
	f := newSecurityFixture(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(f.pool)
	other, err := users.Create(ctx, "Other", nil, "other-security@example.test", f.user.PasswordHash, []string{models.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	foreignID, foreignToken := f.extraSession(t, other.ID, "foreign")
	oneID, oneToken := f.extraSession(t, f.user.ID, "one")
	_, twoToken := f.extraSession(t, f.user.ID, "two")
	view, err := f.security.Overview(ctx, f.user.ID, f.token)
	if err != nil {
		t.Fatal(err)
	}
	if !view.MFAEnabled || !view.MFARequired || view.EnabledAt == nil || view.RecoveryCodesRemaining != 8 || len(view.Sessions) != 3 || !view.Sessions[0].Current {
		t.Fatalf("wrong overview: %+v", view)
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "token_hash") || strings.Contains(string(body), f.token) {
		t.Fatal("session credentials exposed")
	}
	if _, err = f.security.Overview(ctx, f.user.ID, foreignToken); !errors.Is(err, models.ErrSecuritySession) {
		t.Fatal("cross-user token accepted")
	}
	if _, err = f.security.Revoke(ctx, f.user.ID, f.token, foreignID, false, f.proof(f.codes[0])); !errors.Is(err, models.ErrSecurityTarget) {
		t.Fatal("another user's session could be revoked")
	}
	if _, err = f.security.Revoke(ctx, f.user.ID, f.token, view.Sessions[0].ID, false, f.proof(f.codes[0])); !errors.Is(err, models.ErrSecurityCurrentSession) {
		t.Fatal("current session accepted as another session")
	}
	result, err := f.security.Revoke(ctx, f.user.ID, f.token, oneID, false, f.proof(f.codes[0]))
	if err != nil || result.RevokedCount != 1 {
		t.Fatalf("single revoke: %+v %v", result, err)
	}
	oneHash := sha256.Sum256([]byte(oneToken))
	if _, err = users.FindSession(ctx, oneHash[:]); err == nil {
		t.Fatal("revoked session still authenticates")
	}
	fresh, err := f.security.Regenerate(ctx, f.user.ID, f.token, f.proof(f.codes[1]))
	if err != nil || len(fresh.RecoveryCodes) != 8 {
		t.Fatalf("regeneration: %v", err)
	}
	oldHash := sha256.Sum256([]byte(f.codes[2]))
	var count int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM mfa_recovery_codes WHERE user_id=$1 AND code_hash=$2", f.user.ID, oldHash[:]).Scan(&count); err != nil || count != 0 {
		t.Fatal("old recovery code survived regeneration")
	}
	result, err = f.security.Revoke(ctx, f.user.ID, f.token, "", true, f.proof(fresh.RecoveryCodes[0]))
	if err != nil || result.RevokedCount != 1 {
		t.Fatalf("revoke others: %+v %v", result, err)
	}
	twoHash := sha256.Sum256([]byte(twoToken))
	if _, err = users.FindSession(ctx, twoHash[:]); err == nil {
		t.Fatal("other session survived")
	}
	foreignHash := sha256.Sum256([]byte(foreignToken))
	if _, err = users.FindSession(ctx, foreignHash[:]); err != nil {
		t.Fatal("another owner's session was removed")
	}
	view, err = f.security.Overview(ctx, f.user.ID, f.token)
	if err != nil || len(view.Sessions) != 1 || !view.Sessions[0].Current || view.RecoveryCodesRemaining != 7 {
		t.Fatalf("current session was not preserved: %+v %v", view, err)
	}
	if _, err = f.security.Regenerate(ctx, f.user.ID, f.token, f.proof(f.codes[2])); !errors.Is(err, models.ErrMFACode) {
		t.Fatal("invalidated recovery code authorized a change")
	}
}

func TestSecurityAuthenticatorReplacementIsBoundAndAtomic(t *testing.T) {
	f := newSecurityFixture(t)
	ctx := context.Background()
	_, otherToken := f.extraSession(t, f.user.ID, "replacement-other")
	var before []byte
	if err := f.pool.QueryRow(ctx, "SELECT encrypted_secret FROM user_mfa WHERE user_id=$1", f.user.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	setup, err := f.security.BeginReplacement(ctx, f.user.Public(), f.token, f.proof(f.codes[0]))
	if err != nil {
		t.Fatal(err)
	}
	var pending []byte
	if err = f.pool.QueryRow(ctx, "SELECT encrypted_secret FROM user_mfa WHERE user_id=$1", f.user.ID).Scan(&pending); err != nil || !bytes.Equal(before, pending) {
		t.Fatal("begin replaced the active authenticator")
	}
	proof := f.proof(appCode(t, setup.Enrollment.Secret))
	if _, err = f.security.ConfirmReplacement(ctx, f.user.ID, otherToken, proof); !errors.Is(err, models.ErrSecurityReplacement) {
		t.Fatal("replacement completed from a different session")
	}
	if _, err = f.pool.Exec(ctx, "UPDATE mfa_replacements SET expires_at=NOW()-INTERVAL '1 second'"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.security.ConfirmReplacement(ctx, f.user.ID, f.token, proof); !errors.Is(err, models.ErrSecurityReplacement) {
		t.Fatal("expired replacement accepted")
	}
	setup, err = f.security.BeginReplacement(ctx, f.user.Public(), f.token, f.proof(f.codes[1]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.security.ConfirmReplacement(ctx, f.user.ID, f.token, f.proof("abcdef")); !errors.Is(err, models.ErrMFACode) {
		t.Fatal("replacement accepted without new-factor proof")
	}
	if err = f.pool.QueryRow(ctx, "SELECT encrypted_secret FROM user_mfa WHERE user_id=$1", f.user.ID).Scan(&pending); err != nil || !bytes.Equal(before, pending) {
		t.Fatal("failed confirmation changed the active authenticator")
	}
	challenge, _, err := f.second.Begin(ctx, f.user)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.security.ConfirmReplacement(ctx, f.user.ID, f.token, f.proof(appCode(t, setup.Enrollment.Secret)))
	if err != nil || len(result.RecoveryCodes) != 8 || result.RevokedCount != 1 {
		t.Fatalf("confirm: %+v %v", result, err)
	}
	var after []byte
	if err = f.pool.QueryRow(ctx, "SELECT encrypted_secret FROM user_mfa WHERE user_id=$1", f.user.ID).Scan(&after); err != nil || bytes.Equal(before, after) {
		t.Fatal("authenticator was not replaced")
	}
	if _, err = f.security.Overview(ctx, f.user.ID, otherToken); !errors.Is(err, models.ErrSecuritySession) {
		t.Fatal("replacement preserved another session")
	}
	if _, err = f.second.Verify(ctx, models.MFAVerifyRequest{ChallengeToken: challenge.ChallengeToken, Code: f.codes[2]}); !errors.Is(err, models.ErrMFAChallenge) {
		t.Fatal("old-factor login challenge survived replacement")
	}
	view, err := f.security.Overview(ctx, f.user.ID, f.token)
	if err != nil || view.RecoveryCodesRemaining != 8 {
		t.Fatal("replacement did not preserve current session and fresh codes")
	}
	if _, err = f.security.ConfirmReplacement(ctx, f.user.ID, f.token, f.proof(appCode(t, setup.Enrollment.Secret))); !errors.Is(err, models.ErrSecurityReplacement) {
		t.Fatal("replacement confirmation replay accepted")
	}
}

func TestSecurityAuditFailureRollsBackRecoveryRegeneration(t *testing.T) {
	f := newSecurityFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_security_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture audit unavailable'; END $$;
	 CREATE TRIGGER reject_security_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_security_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.security.Regenerate(ctx, f.user.ID, f.token, f.proof(f.codes[0])); err == nil {
		t.Fatal("security change committed without audit")
	}
	view, err := f.security.Overview(ctx, f.user.ID, f.token)
	if err != nil || view.RecoveryCodesRemaining != 8 {
		t.Fatal("audit failure consumed or replaced codes")
	}
	if _, err = f.pool.Exec(ctx, "DROP TRIGGER reject_security_audit ON audit_events"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.security.Regenerate(ctx, f.user.ID, f.token, f.proof(f.codes[0])); err != nil {
		t.Fatalf("failed transaction consumed proof: %v", err)
	}
}

func TestSecurityConcurrentRecoveryProofWorksOnce(t *testing.T) {
	f := newSecurityFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.security.Regenerate(ctx, f.user.ID, f.token, f.proof(f.codes[0]))
			outcomes <- err
		}()
	}
	wg.Wait()
	close(outcomes)
	succeeded := 0
	for err := range outcomes {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, models.ErrMFACode) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("proof committed %d times", succeeded)
	}
	var count int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='mfa_recovery_regenerated'").Scan(&count); err != nil || count != 1 {
		t.Fatal("concurrent mutation audit count wrong")
	}
}

func TestSecurityPasswordOnlyAccountAndRevokedCurrentSession(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	hash, err := services.HashPassword("Simple#123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(ctx, "Ordinary", nil, "ordinary-security@example.test", hash, []string{models.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	current := sha256.Sum256([]byte(token))
	other := sha256.Sum256([]byte(strings.Repeat("b", 64)))
	if err = users.CreateSession(ctx, u.ID, hash, current[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = users.CreateSession(ctx, u.ID, hash, other[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	svc := services.NewSecurityService(repositories.NewSecurityRepository(pool), nil)
	view, err := svc.Overview(ctx, u.ID, token)
	if err != nil || view.MFAEnabled || view.MFARequired || view.EnabledAt != nil || len(view.Sessions) != 2 {
		t.Fatalf("ordinary overview: %+v %v", view, err)
	}
	result, err := svc.Revoke(ctx, u.ID, token, "", true, models.SecurityProof{Password: "Simple#123"})
	if err != nil || result.RevokedCount != 1 {
		t.Fatalf("ordinary revocation: %+v %v", result, err)
	}
	if err = users.DeleteSession(ctx, current[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Overview(ctx, u.ID, token); !errors.Is(err, models.ErrSecuritySession) {
		t.Fatal("revoked current session read security data")
	}
	if _, err = svc.Revoke(ctx, u.ID, token, "", true, models.SecurityProof{Password: "Simple#123"}); !errors.Is(err, models.ErrSecuritySession) {
		t.Fatal("revoked current session mutated security data")
	}
}

func TestSecurityWrongPasswordLockout(t *testing.T) {
	f := newSecurityFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := f.security.Regenerate(ctx, f.user.ID, f.token, models.SecurityProof{Password: "wrong password", Code: f.codes[0]})
		want := models.ErrMFACode
		if i == 4 {
			want = models.ErrMFALocked
		}
		if !errors.Is(err, want) {
			t.Fatalf("failed attempt %d: %v", i, err)
		}
	}
	if _, err := f.security.Regenerate(ctx, f.user.ID, f.token, f.proof(f.codes[0])); !errors.Is(err, models.ErrMFALocked) {
		t.Fatal("lockout bypassed with fresh proof")
	}
	view, err := f.security.Overview(ctx, f.user.ID, f.token)
	if err != nil || view.RecoveryCodesRemaining != 8 {
		t.Fatal("failed password proof consumed a code")
	}
}
