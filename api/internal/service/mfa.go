package service

import (
	"context"
	"errors"
	"time"

	"paycore/api/internal/audit"
	"paycore/api/internal/model"
)

const MFAChallengeTTL = 5 * time.Minute

type MFAStore interface {
	State(context.Context, string) (model.MFARecord, error)
	Start(context.Context, model.User, model.MFAChallenge) error
	Complete(context.Context, []byte, []byte, time.Time, model.MFADecide) (model.User, error)
}
type Authenticator interface {
	Enrollment(string, string) (model.MFAEnrollment, []byte, error)
	Match(string, []byte, string, int64, time.Time) (int64, error)
}
type MFAService struct {
	disabled      bool
	store         MFAStore
	authenticator Authenticator
	Audit         audit.Recorder
}

func NewMFAService(store MFAStore, authenticator Authenticator) *MFAService {
	return &MFAService{store: store, authenticator: authenticator}
}
func NewMFAServiceWithMFAPolicy(store MFAStore, authenticator Authenticator, enabled bool) *MFAService {
	s := NewMFAService(store, authenticator)
	s.disabled = !enabled
	return s
}
func (s *MFAService) Begin(ctx context.Context, u model.User) (model.MFAChallengeResponse, bool, error) {
	if s.disabled {
		return model.MFAChallengeResponse{}, false, nil
	}
	state, err := s.store.State(ctx, u.ID)
	if err != nil {
		return model.MFAChallengeResponse{}, false, err
	}
	if !u.HasRole(model.RoleAdmin) && len(state.Secret) == 0 {
		return model.MFAChallengeResponse{}, false, nil
	}
	token, hash, err := newInvitationToken()
	if err != nil {
		return model.MFAChallengeResponse{}, false, err
	}
	result := model.MFAChallengeResponse{State: model.MFAVerificationRequired, ChallengeToken: token, ExpiresIn: int64(MFAChallengeTTL.Seconds())}
	challenge := model.MFAChallenge{Hash: hash, ExpiresAt: time.Now().Add(MFAChallengeTTL)}
	if len(state.Secret) == 0 {
		enrollment, encrypted, err := s.authenticator.Enrollment(u.ID, u.Email)
		if err != nil {
			return model.MFAChallengeResponse{}, false, err
		}
		result.State = model.MFAEnrollmentRequired
		result.Enrollment = &enrollment
		challenge.PendingSecret = encrypted
	}
	if err = s.store.Start(ctx, u, challenge); err != nil {
		return model.MFAChallengeResponse{}, false, err
	}
	return result, true, nil
}
func (s *MFAService) Verify(ctx context.Context, req model.MFAVerifyRequest) (model.MFALoginResult, error) {
	if s.disabled {
		return model.MFALoginResult{}, model.ErrMFADisabled
	}
	token, sessionHash, err := newInvitationToken()
	if err != nil {
		return model.MFALoginResult{}, err
	}
	codes := []string{}
	var enrolling, recovered bool
	u, err := s.store.Complete(ctx, digestToken(req.ChallengeToken), sessionHash, time.Now().Add(SessionTTL), func(state model.MFARecord) (model.MFAApproval, error) {

		enrolling = len(state.Secret) == 0
		approval, usedRecovery, err := verifyMFAProof(s.authenticator, state, req.Code, time.Now())
		if err != nil {
			return model.MFAApproval{}, err
		}
		recovered = usedRecovery
		if enrolling {
			codes, approval.NewRecoveryHashes, err = newRecoveryCodes()
			if err != nil {
				return model.MFAApproval{}, err
			}
		}
		return approval, nil
	})
	if err != nil {
		if errors.Is(err, model.ErrMFACode) || errors.Is(err, model.ErrMFAChallenge) || errors.Is(err, model.ErrMFALocked) {
			audit.Record(ctx, s.Audit, audit.Event{Action: audit.MFAFailed})
		}
		return model.MFALoginResult{}, err
	}
	action := audit.MFAVerified
	if enrolling {
		action = audit.MFAEnrolled
	} else if recovered {
		action = audit.MFARecoveryUsed
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: action, ActorID: audit.Ptr(u.ID), TargetID: audit.Ptr(u.ID)})
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.LoginSucceeded, ActorID: audit.Ptr(u.ID), TargetID: audit.Ptr(u.ID)})
	return model.MFALoginResult{Session: model.LoginResponse{Token: token, TokenType: "Bearer", ExpiresIn: int64(SessionTTL.Seconds()), User: u.Public()}, RecoveryCodes: codes}, nil
}
