package services

import (
	"context"
	"errors"
	"time"

	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

const MFAChallengeTTL = 5 * time.Minute

type MFAStore interface {
	State(context.Context, string) (models.MFARecord, error)
	Start(context.Context, models.User, models.MFAChallenge) error
	Complete(context.Context, []byte, []byte, time.Time, models.MFADecide) (models.User, error)
}
type Authenticator interface {
	Enrollment(string, string) (models.MFAEnrollment, []byte, error)
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
func (s *MFAService) Begin(ctx context.Context, u models.User) (models.MFAChallengeResponse, bool, error) {
	if s.disabled {
		return models.MFAChallengeResponse{}, false, nil
	}
	state, err := s.store.State(ctx, u.ID)
	if err != nil {
		return models.MFAChallengeResponse{}, false, err
	}
	if !u.HasRole(models.RoleAdmin) && len(state.Secret) == 0 {
		return models.MFAChallengeResponse{}, false, nil
	}
	token, hash, err := newInvitationToken()
	if err != nil {
		return models.MFAChallengeResponse{}, false, err
	}
	result := models.MFAChallengeResponse{State: models.MFAVerificationRequired, ChallengeToken: token, ExpiresIn: int64(MFAChallengeTTL.Seconds())}
	challenge := models.MFAChallenge{Hash: hash, ExpiresAt: time.Now().Add(MFAChallengeTTL)}
	if len(state.Secret) == 0 {
		enrollment, encrypted, err := s.authenticator.Enrollment(u.ID, u.Email)
		if err != nil {
			return models.MFAChallengeResponse{}, false, err
		}
		result.State = models.MFAEnrollmentRequired
		result.Enrollment = &enrollment
		challenge.PendingSecret = encrypted
	}
	if err = s.store.Start(ctx, u, challenge); err != nil {
		return models.MFAChallengeResponse{}, false, err
	}
	return result, true, nil
}
func (s *MFAService) Verify(ctx context.Context, req models.MFAVerifyRequest) (models.MFALoginResult, error) {
	if s.disabled {
		return models.MFALoginResult{}, models.ErrMFADisabled
	}
	token, sessionHash, err := newInvitationToken()
	if err != nil {
		return models.MFALoginResult{}, err
	}
	codes := []string{}
	var enrolling, recovered bool
	u, err := s.store.Complete(ctx, digestToken(req.ChallengeToken), sessionHash, time.Now().Add(SessionTTL), func(state models.MFARecord) (models.MFAApproval, error) {

		enrolling = len(state.Secret) == 0
		approval, usedRecovery, err := verifyMFAProof(s.authenticator, state, req.Code, time.Now())
		if err != nil {
			return models.MFAApproval{}, err
		}
		recovered = usedRecovery
		if enrolling {
			codes, approval.NewRecoveryHashes, err = newRecoveryCodes()
			if err != nil {
				return models.MFAApproval{}, err
			}
		}
		return approval, nil
	})
	if err != nil {
		if errors.Is(err, models.ErrMFACode) || errors.Is(err, models.ErrMFAChallenge) || errors.Is(err, models.ErrMFALocked) {
			audit.Record(ctx, s.Audit, audit.Event{Action: audit.MFAFailed})
		}
		return models.MFALoginResult{}, err
	}
	action := audit.MFAVerified
	if enrolling {
		action = audit.MFAEnrolled
	} else if recovered {
		action = audit.MFARecoveryUsed
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: action, ActorID: audit.Ptr(u.ID), TargetID: audit.Ptr(u.ID)})
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.LoginSucceeded, ActorID: audit.Ptr(u.ID), TargetID: audit.Ptr(u.ID)})
	return models.MFALoginResult{Session: models.LoginResponse{Token: token, TokenType: "Bearer", ExpiresIn: int64(SessionTTL.Seconds()), User: u.Public()}, RecoveryCodes: codes}, nil
}
