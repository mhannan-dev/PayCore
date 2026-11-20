package services

import (
	"context"
	"crypto/sha256"
	"time"

	"golang.org/x/crypto/bcrypt"
	"paycore/api/internal/models"
)

type SecurityStore interface {
	Overview(context.Context, string, []byte) (models.SecurityOverview, error)
	Change(context.Context, string, []byte, models.SecurityCommand, models.SecurityDecide) (models.SecurityActionResult, error)
}
type SecurityService struct {
	mfaDisabled   bool
	store         SecurityStore
	authenticator Authenticator
}

func NewSecurityService(store SecurityStore, authenticator Authenticator) *SecurityService {
	return &SecurityService{store: store, authenticator: authenticator}
}
func NewSecurityServiceWithMFAPolicy(store SecurityStore, authenticator Authenticator, enabled bool) *SecurityService {
	s := NewSecurityService(store, authenticator)
	s.mfaDisabled = !enabled
	return s
}
func sessionDigest(token string) ([]byte, error) {
	if len(token) != 64 {
		return nil, models.ErrSecuritySession
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], nil
}
func (s *SecurityService) Overview(ctx context.Context, owner, token string) (models.SecurityOverview, error) {
	hash, err := sessionDigest(token)
	if err != nil {
		return models.SecurityOverview{}, err
	}
	return s.store.Overview(ctx, owner, hash)
}
func (s *SecurityService) proof(state models.SecurityRecord, proof models.SecurityProof) (models.MFAApproval, error) {
	if len(proof.Password) == 0 || len(proof.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(state.User.PasswordHash), []byte(proof.Password)) != nil {
		return models.MFAApproval{}, models.ErrMFACode
	}
	if s.mfaDisabled || len(state.MFA.Secret) == 0 {
		return models.MFAApproval{Counter: -1}, nil
	}
	approval, _, err := verifyMFAProof(s.authenticator, state.MFA, proof.Code, time.Now())
	return approval, err
}
func (s *SecurityService) Regenerate(ctx context.Context, owner, token string, proof models.SecurityProof) (models.RecoveryCodesResult, error) {
	if s.mfaDisabled {
		return models.RecoveryCodesResult{}, models.ErrMFADisabled
	}
	hash, err := sessionDigest(token)
	if err != nil {
		return models.RecoveryCodesResult{}, err
	}
	var codes []string
	_, err = s.store.Change(ctx, owner, hash, models.SecurityCommand{Action: models.SecurityRecoveryCodes}, func(state models.SecurityRecord) (models.MFAApproval, error) {
		if len(state.MFA.Secret) == 0 {
			return models.MFAApproval{}, models.ErrMFARequired
		}
		approval, err := s.proof(state, proof)
		if err != nil {
			return models.MFAApproval{}, err
		}
		codes, approval.NewRecoveryHashes, err = newRecoveryCodes()
		return approval, err
	})
	if err != nil {
		return models.RecoveryCodesResult{}, err
	}
	return models.RecoveryCodesResult{RecoveryCodes: codes}, nil
}
func (s *SecurityService) BeginReplacement(ctx context.Context, actor models.UserDTO, token string, proof models.SecurityProof) (models.ReplacementSetup, error) {
	if s.mfaDisabled {
		return models.ReplacementSetup{}, models.ErrMFADisabled
	}
	hash, err := sessionDigest(token)
	if err != nil {
		return models.ReplacementSetup{}, err
	}
	enrollment, encrypted, err := s.authenticator.Enrollment(actor.ID, actor.Email)
	if err != nil {
		return models.ReplacementSetup{}, err
	}
	_, err = s.store.Change(ctx, actor.ID, hash, models.SecurityCommand{Action: models.SecurityReplaceBegin, PendingSecret: encrypted, ExpiresAt: time.Now().Add(MFAChallengeTTL)}, func(state models.SecurityRecord) (models.MFAApproval, error) {
		if len(state.MFA.Secret) == 0 {
			return models.MFAApproval{}, models.ErrMFARequired
		}
		return s.proof(state, proof)
	})
	if err != nil {
		return models.ReplacementSetup{}, err
	}
	return models.ReplacementSetup{Enrollment: enrollment, ExpiresIn: int64(MFAChallengeTTL.Seconds())}, nil
}
func (s *SecurityService) ConfirmReplacement(ctx context.Context, owner, token string, proof models.SecurityProof) (models.ReplacementResult, error) {
	if s.mfaDisabled {
		return models.ReplacementResult{}, models.ErrMFADisabled
	}
	hash, err := sessionDigest(token)
	if err != nil {
		return models.ReplacementResult{}, err
	}
	var codes []string
	result, err := s.store.Change(ctx, owner, hash, models.SecurityCommand{Action: models.SecurityReplaceConfirm}, func(state models.SecurityRecord) (models.MFAApproval, error) {
		if len(proof.Password) == 0 || len(proof.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(state.User.PasswordHash), []byte(proof.Password)) != nil {
			return models.MFAApproval{}, models.ErrMFACode
		}
		counter, err := s.authenticator.Match(owner, state.MFA.PendingSecret, proof.Code, -1, time.Now())
		if err != nil {
			return models.MFAApproval{}, err
		}
		var hashes [][]byte
		codes, hashes, err = newRecoveryCodes()
		return models.MFAApproval{Counter: counter, NewRecoveryHashes: hashes}, err
	})
	if err != nil {
		return models.ReplacementResult{}, err
	}
	return models.ReplacementResult{RecoveryCodes: codes, RevokedCount: result.RevokedCount}, nil
}
func (s *SecurityService) Revoke(ctx context.Context, owner, token, target string, others bool, proof models.SecurityProof) (models.SecurityActionResult, error) {
	hash, err := sessionDigest(token)
	if err != nil {
		return models.SecurityActionResult{}, err
	}
	action := models.SecurityRevokeSession
	if others {
		action = models.SecurityRevokeOthers
	}
	return s.store.Change(ctx, owner, hash, models.SecurityCommand{Action: action, TargetID: target}, func(state models.SecurityRecord) (models.MFAApproval, error) { return s.proof(state, proof) })
}
