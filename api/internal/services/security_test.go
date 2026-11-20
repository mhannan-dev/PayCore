package services

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"paycore/api/internal/models"
)

type securityStoreFake struct {
	record   models.SecurityRecord
	command  models.SecurityCommand
	approval models.MFAApproval
	applied  bool
	hash     []byte
}

func (s *securityStoreFake) Overview(context.Context, string, []byte) (models.SecurityOverview, error) {
	return models.SecurityOverview{}, nil
}
func (s *securityStoreFake) Change(_ context.Context, _ string, hash []byte, command models.SecurityCommand, decide models.SecurityDecide) (models.SecurityActionResult, error) {
	s.command = command
	s.hash = hash
	approval, err := decide(s.record)
	if err != nil {
		return models.SecurityActionResult{}, err
	}
	s.approval = approval
	s.applied = true
	return models.SecurityActionResult{RevokedCount: 2}, nil
}

type securityAuthenticatorFake struct{ matched []byte }

func (a *securityAuthenticatorFake) Enrollment(string, string) (models.MFAEnrollment, []byte, error) {
	return models.MFAEnrollment{Secret: "setup"}, []byte("new encrypted secret"), nil
}
func (a *securityAuthenticatorFake) Match(_ string, secret []byte, code string, last int64, _ time.Time) (int64, error) {
	a.matched = secret
	if code != "123456" || last >= 7 {
		return -1, models.ErrMFACode
	}
	return 7, nil
}
func securityUnitFixture(t *testing.T) (*SecurityService, *securityStoreFake, *securityAuthenticatorFake) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Admin#1234"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	store := &securityStoreFake{record: models.SecurityRecord{User: models.User{ID: "owner", PasswordHash: string(hash)}, MFA: models.MFARecord{UserID: "owner", Secret: []byte("old encrypted secret"), PendingSecret: []byte("new encrypted secret"), LastCounter: 6}}}
	auth := &securityAuthenticatorFake{}
	return NewSecurityService(store, auth), store, auth
}
func TestSecurityServiceProofAndHashBoundaries(t *testing.T) {
	svc, store, _ := securityUnitFixture(t)
	ctx := context.Background()
	token := strings.Repeat("a", 64)
	if _, err := svc.Regenerate(ctx, "owner", token, models.SecurityProof{Password: "wrong", Code: "123456"}); !errors.Is(err, models.ErrMFACode) || store.applied {
		t.Fatal("wrong password authorized regeneration")
	}
	result, err := svc.Regenerate(ctx, "owner", token, models.SecurityProof{Password: "Admin#1234", Code: "123456"})
	if err != nil || len(result.RecoveryCodes) != 8 || !store.applied {
		t.Fatalf("regeneration: %v", err)
	}
	if store.command.Action != models.SecurityRecoveryCodes || string(store.hash) == token {
		t.Fatal("store received raw token or wrong command")
	}
	for i, code := range result.RecoveryCodes {
		hash := sha256.Sum256([]byte(code))
		if string(hash[:]) != string(store.approval.NewRecoveryHashes[i]) || len(code) != 32 {
			t.Fatal("recovery code/hash contract broken")
		}
	}
	store.applied = false
	store.record.MFA.LastCounter = 7
	if _, err = svc.Regenerate(ctx, "owner", token, models.SecurityProof{Password: "Admin#1234", Code: "123456"}); !errors.Is(err, models.ErrMFACode) || store.applied {
		t.Fatal("replayed step authorized regeneration")
	}
}
func TestSecurityServiceReplacementUsesPendingFactor(t *testing.T) {
	svc, store, auth := securityUnitFixture(t)
	result, err := svc.ConfirmReplacement(context.Background(), "owner", strings.Repeat("b", 64), models.SecurityProof{Password: "Admin#1234", Code: "123456"})
	if err != nil || len(result.RecoveryCodes) != 8 || result.RevokedCount != 2 || string(auth.matched) != "new encrypted secret" || store.command.Action != models.SecurityReplaceConfirm {
		t.Fatalf("wrong replacement verification: %+v %v", result, err)
	}
}
func TestSecurityServicePasswordOnlySessions(t *testing.T) {
	svc, store, auth := securityUnitFixture(t)
	store.record.MFA.Secret = nil
	if _, err := svc.Revoke(context.Background(), "owner", strings.Repeat("c", 64), "", true, models.SecurityProof{Password: "Admin#1234"}); err != nil || !store.applied || auth.matched != nil || store.command.Action != models.SecurityRevokeOthers {
		t.Fatalf("password-only revoke: %v", err)
	}
	if _, err := svc.Regenerate(context.Background(), "owner", strings.Repeat("c", 64), models.SecurityProof{Password: "Admin#1234"}); !errors.Is(err, models.ErrMFARequired) {
		t.Fatal("codes generated for an unenrolled account")
	}
}
