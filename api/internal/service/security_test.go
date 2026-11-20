package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"paycore/api/internal/model"
)

type securityStoreFake struct {
	record   model.SecurityRecord
	command  model.SecurityCommand
	approval model.MFAApproval
	applied  bool
	hash     []byte
}

func (s *securityStoreFake) Overview(context.Context, string, []byte) (model.SecurityOverview, error) {
	return model.SecurityOverview{}, nil
}
func (s *securityStoreFake) Change(_ context.Context, _ string, hash []byte, command model.SecurityCommand, decide model.SecurityDecide) (model.SecurityActionResult, error) {
	s.command = command
	s.hash = hash
	approval, err := decide(s.record)
	if err != nil {
		return model.SecurityActionResult{}, err
	}
	s.approval = approval
	s.applied = true
	return model.SecurityActionResult{RevokedCount: 2}, nil
}

type securityAuthenticatorFake struct{ matched []byte }

func (a *securityAuthenticatorFake) Enrollment(string, string) (model.MFAEnrollment, []byte, error) {
	return model.MFAEnrollment{Secret: "setup"}, []byte("new encrypted secret"), nil
}
func (a *securityAuthenticatorFake) Match(_ string, secret []byte, code string, last int64, _ time.Time) (int64, error) {
	a.matched = secret
	if code != "123456" || last >= 7 {
		return -1, model.ErrMFACode
	}
	return 7, nil
}
func securityUnitFixture(t *testing.T) (*SecurityService, *securityStoreFake, *securityAuthenticatorFake) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Admin#1234"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	store := &securityStoreFake{record: model.SecurityRecord{User: model.User{ID: "owner", PasswordHash: string(hash)}, MFA: model.MFARecord{UserID: "owner", Secret: []byte("old encrypted secret"), PendingSecret: []byte("new encrypted secret"), LastCounter: 6}}}
	auth := &securityAuthenticatorFake{}
	return NewSecurityService(store, auth), store, auth
}
func TestSecurityServiceProofAndHashBoundaries(t *testing.T) {
	svc, store, _ := securityUnitFixture(t)
	ctx := context.Background()
	token := strings.Repeat("a", 64)
	if _, err := svc.Regenerate(ctx, "owner", token, model.SecurityProof{Password: "wrong", Code: "123456"}); !errors.Is(err, model.ErrMFACode) || store.applied {
		t.Fatal("wrong password authorized regeneration")
	}
	result, err := svc.Regenerate(ctx, "owner", token, model.SecurityProof{Password: "Admin#1234", Code: "123456"})
	if err != nil || len(result.RecoveryCodes) != 8 || !store.applied {
		t.Fatalf("regeneration: %v", err)
	}
	if store.command.Action != model.SecurityRecoveryCodes || string(store.hash) == token {
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
	if _, err = svc.Regenerate(ctx, "owner", token, model.SecurityProof{Password: "Admin#1234", Code: "123456"}); !errors.Is(err, model.ErrMFACode) || store.applied {
		t.Fatal("replayed step authorized regeneration")
	}
}
func TestSecurityServiceReplacementUsesPendingFactor(t *testing.T) {
	svc, store, auth := securityUnitFixture(t)
	result, err := svc.ConfirmReplacement(context.Background(), "owner", strings.Repeat("b", 64), model.SecurityProof{Password: "Admin#1234", Code: "123456"})
	if err != nil || len(result.RecoveryCodes) != 8 || result.RevokedCount != 2 || string(auth.matched) != "new encrypted secret" || store.command.Action != model.SecurityReplaceConfirm {
		t.Fatalf("wrong replacement verification: %+v %v", result, err)
	}
}
func TestSecurityServicePasswordOnlySessions(t *testing.T) {
	svc, store, auth := securityUnitFixture(t)
	store.record.MFA.Secret = nil
	if _, err := svc.Revoke(context.Background(), "owner", strings.Repeat("c", 64), "", true, model.SecurityProof{Password: "Admin#1234"}); err != nil || !store.applied || auth.matched != nil || store.command.Action != model.SecurityRevokeOthers {
		t.Fatalf("password-only revoke: %v", err)
	}
	if _, err := svc.Regenerate(context.Background(), "owner", strings.Repeat("c", 64), model.SecurityProof{Password: "Admin#1234"}); !errors.Is(err, model.ErrMFARequired) {
		t.Fatal("codes generated for an unenrolled account")
	}
}
