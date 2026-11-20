package service

import (
	"context"
	"crypto/sha256"
	"time"

	"golang.org/x/crypto/bcrypt"
	"paycore/api/internal/model"
)

type SecurityStore interface {
	Overview(context.Context, string, []byte) (model.SecurityOverview, error)
	Change(context.Context, string, []byte, model.SecurityCommand, model.SecurityDecide) (model.SecurityActionResult, error)
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
		return nil, model.ErrSecuritySession
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], nil
}
func (s *SecurityService) Overview(ctx context.Context, owner, token string) (model.SecurityOverview, error) {
	hash, err := sessionDigest(token)
	if err != nil {
		return model.SecurityOverview{}, err
	}
	return s.store.Overview(ctx, owner, hash)
}
func (s *SecurityService) proof(state model.SecurityRecord, proof model.SecurityProof) (model.MFAApproval, error) {
	if len(proof.Password) == 0 || len(proof.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(state.User.PasswordHash), []byte(proof.Password)) != nil {
		return model.MFAApproval{}, model.ErrMFACode
	}
	if s.mfaDisabled || len(state.MFA.Secret) == 0 {
		return model.MFAApproval{Counter: -1}, nil
	}
	approval, _, err := verifyMFAProof(s.authenticator, state.MFA, proof.Code, time.Now())
	return approval, err
}
func (s *SecurityService) Regenerate(ctx context.Context, owner, token string, proof model.SecurityProof) (model.RecoveryCodesResult, error) {
	if s.mfaDisabled {
		return model.RecoveryCodesResult{}, model.ErrMFADisabled
	}
	hash, err := sessionDigest(token)
	if err != nil {
		return model.RecoveryCodesResult{}, err
	}
	var codes []string
	_, err = s.store.Change(ctx, owner, hash, model.SecurityCommand{Action: model.SecurityRecoveryCodes}, func(state model.SecurityRecord) (model.MFAApproval, error) {
		if len(state.MFA.Secret) == 0 {
			return model.MFAApproval{}, model.ErrMFARequired
		}
		approval, err := s.proof(state, proof)
		if err != nil {
			return model.MFAApproval{}, err
		}
		codes, approval.NewRecoveryHashes, err = newRecoveryCodes()
		return approval, err
	})
	if err != nil {
		return model.RecoveryCodesResult{}, err
	}
	return model.RecoveryCodesResult{RecoveryCodes: codes}, nil
}
func (s *SecurityService) BeginReplacement(ctx context.Context, actor model.UserDTO, token string, proof model.SecurityProof) (model.ReplacementSetup, error) {
	if s.mfaDisabled {
		return model.ReplacementSetup{}, model.ErrMFADisabled
	}
	hash, err := sessionDigest(token)
	if err != nil {
		return model.ReplacementSetup{}, err
	}
	enrollment, encrypted, err := s.authenticator.Enrollment(actor.ID, actor.Email)
	if err != nil {
		return model.ReplacementSetup{}, err
	}
	_, err = s.store.Change(ctx, actor.ID, hash, model.SecurityCommand{Action: model.SecurityReplaceBegin, PendingSecret: encrypted, ExpiresAt: time.Now().Add(MFAChallengeTTL)}, func(state model.SecurityRecord) (model.MFAApproval, error) {
		if len(state.MFA.Secret) == 0 {
			return model.MFAApproval{}, model.ErrMFARequired
		}
		return s.proof(state, proof)
	})
	if err != nil {
		return model.ReplacementSetup{}, err
	}
	return model.ReplacementSetup{Enrollment: enrollment, ExpiresIn: int64(MFAChallengeTTL.Seconds())}, nil
}
func (s *SecurityService) ConfirmReplacement(ctx context.Context, owner, token string, proof model.SecurityProof) (model.ReplacementResult, error) {
	if s.mfaDisabled {
		return model.ReplacementResult{}, model.ErrMFADisabled
	}
	hash, err := sessionDigest(token)
	if err != nil {
		return model.ReplacementResult{}, err
	}
	var codes []string
	result, err := s.store.Change(ctx, owner, hash, model.SecurityCommand{Action: model.SecurityReplaceConfirm}, func(state model.SecurityRecord) (model.MFAApproval, error) {
		if len(proof.Password) == 0 || len(proof.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(state.User.PasswordHash), []byte(proof.Password)) != nil {
			return model.MFAApproval{}, model.ErrMFACode
		}
		counter, err := s.authenticator.Match(owner, state.MFA.PendingSecret, proof.Code, -1, time.Now())
		if err != nil {
			return model.MFAApproval{}, err
		}
		var hashes [][]byte
		codes, hashes, err = newRecoveryCodes()
		return model.MFAApproval{Counter: counter, NewRecoveryHashes: hashes}, err
	})
	if err != nil {
		return model.ReplacementResult{}, err
	}
	return model.ReplacementResult{RecoveryCodes: codes, RevokedCount: result.RevokedCount}, nil
}
func (s *SecurityService) Revoke(ctx context.Context, owner, token, target string, others bool, proof model.SecurityProof) (model.SecurityActionResult, error) {
	hash, err := sessionDigest(token)
	if err != nil {
		return model.SecurityActionResult{}, err
	}
	action := model.SecurityRevokeSession
	if others {
		action = model.SecurityRevokeOthers
	}
	return s.store.Change(ctx, owner, hash, model.SecurityCommand{Action: action, TargetID: target}, func(state model.SecurityRecord) (model.MFAApproval, error) { return s.proof(state, proof) })
}
