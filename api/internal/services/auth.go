package services

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

const PasswordCost = 12
const SessionTTL = 24 * time.Hour

var ErrCredentials = errors.New("invalid email or password")
var ErrCaptcha = errors.New("invalid or expired captcha")
var ErrUnavailable = errors.New("authentication unavailable")
var ErrSession = errors.New("invalid session")

// AuthStore is the user and session storage AuthService needs (implemented by
// repositories.UserRepository). Declared here, by its consumer.
type AuthStore interface {
	FindByEmail(context.Context, string) (models.User, error)
	CreateSession(context.Context, string, string, []byte, time.Time) error
	FindSession(context.Context, []byte) (models.User, error)
	DeleteSession(context.Context, []byte) error
	UpdateName(ctx context.Context, userID, firstName string, lastName *string) (models.User, error)
}

type CaptchaVerifier interface{ Verify(string, string) bool }
type LoginMFA interface {
	Begin(context.Context, models.User) (models.MFAChallengeResponse, bool, error)
}
type AuthService struct {
	mfaDisabled bool
	mfa         LoginMFA
	store       AuthStore
	captcha     CaptchaVerifier
	dummyHash   []byte
	// Audit, when set, records sign-ins (successful and failed) and sign-outs.
	Audit audit.Recorder
}

func NewAuthService(store AuthStore, captcha CaptchaVerifier, secondFactor ...LoginMFA) (*AuthService, error) {
	// Perform the same password KDF for unknown and known emails.
	dummy, err := bcrypt.GenerateFromPassword([]byte("unused-dummy-password-for-unknown-users"), PasswordCost)
	if err != nil {
		return nil, err
	}
	var mfa LoginMFA
	if len(secondFactor) > 0 {
		mfa = secondFactor[0]
	}
	return &AuthService{store: store, captcha: captcha, dummyHash: dummy, mfa: mfa}, nil
}

// NewAuthServiceWithMFAPolicy allows an explicit deployment-wide pause. Normal
// constructors enforce MFA; disabling it never marks password sessions verified.
func NewAuthServiceWithMFAPolicy(store AuthStore, captcha CaptchaVerifier, factor LoginMFA, enabled bool) (*AuthService, error) {
	s, err := NewAuthService(store, captcha, factor)
	if err != nil {
		return nil, err
	}
	s.mfaDisabled = !enabled
	return s, nil
}

// PasswordLength is the exact number of characters (Unicode code points) a new
// password must have. Existing passwords are not re-checked at login.
const PasswordLength = 10

// ErrPasswordLength means a new password is not exactly PasswordLength characters.
var ErrPasswordLength = errors.New("password must be exactly 10 characters")

func HashPassword(password string) (string, error) {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) != PasswordLength {
		return "", ErrPasswordLength
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	return string(hash), err
}

func (s *AuthService) Login(ctx context.Context, req models.LoginRequest) (models.LoginResponse, error) {
	if s.store == nil {
		return models.LoginResponse{}, ErrUnavailable
	}
	if len(req.Password) > 72 {
		return models.LoginResponse{}, ErrCredentials
	}
	if s.captcha == nil || !s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer) {
		return models.LoginResponse{}, ErrCaptcha
	}
	u, err := s.store.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return models.LoginResponse{}, fmt.Errorf("find login identity: %w", err)
	}
	// Unknown emails and invited users without a password still pay for one bcrypt
	// comparison, so response time does not reveal which accounts exist.
	hash := s.dummyHash
	if err == nil && u.PasswordHash != "" {
		hash = []byte(u.PasswordHash)
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(req.Password))
	if err != nil || u.PasswordHash == "" || match != nil || !u.IsActive {
		s.recordLoginFailure(ctx, req.Email, u, err == nil, match == nil)
		return models.LoginResponse{}, ErrCredentials
	}
	if !s.mfaDisabled && s.mfa != nil {
		challenge, required, err := s.mfa.Begin(ctx, u)
		if err != nil {
			if errors.Is(err, models.ErrCredentialsChanged) || errors.Is(err, pgx.ErrNoRows) {
				return models.LoginResponse{}, ErrCredentials
			}
			return models.LoginResponse{}, err
		}
		if required {
			return models.LoginResponse{MFA: &challenge}, nil
		}
	} else if !s.mfaDisabled && u.HasRole(models.RoleAdmin) {
		return models.LoginResponse{}, models.ErrMFARequired
	}
	token, err := randomHex()
	if err != nil {
		return models.LoginResponse{}, fmt.Errorf("generate session token: %w", err)
	}
	digest := sha256.Sum256([]byte(token))
	if err = s.store.CreateSession(ctx, u.ID, u.PasswordHash, digest[:], time.Now().UTC().Add(SessionTTL)); err != nil {
		if errors.Is(err, repositories.ErrInactiveUser) || errors.Is(err, models.ErrCredentialsChanged) {
			return models.LoginResponse{}, ErrCredentials
		}
		return models.LoginResponse{}, fmt.Errorf("create session: %w", err)
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.LoginSucceeded,
		ActorID: audit.Ptr(u.ID), ActorEmail: audit.Ptr(u.Email), TargetID: audit.Ptr(u.ID), TargetEmail: audit.Ptr(u.Email)})
	return models.LoginResponse{Token: token, TokenType: "Bearer", ExpiresIn: int64(SessionTTL.Seconds()), User: u.Public()}, nil
}

// recordLoginFailure audits a rejected sign-in with a fixed reason code. The API
// response stays the same for every reason, so accounts cannot be discovered.
func (s *AuthService) recordLoginFailure(ctx context.Context, email string, u models.User, found, passwordMatched bool) {
	reason := "wrong_password"
	switch {
	case !found:
		reason = "unknown_email"
	case u.PasswordHash == "":
		reason = "password_not_set"
	case !u.IsActive && passwordMatched:
		reason = "account_deactivated"
	}
	e := audit.Event{Action: audit.LoginFailed, TargetEmail: audit.Ptr(strings.ToLower(strings.TrimSpace(email))), Details: audit.Details{Reason: reason}}
	if found {
		e.TargetID = audit.Ptr(u.ID)
	}
	audit.Record(ctx, s.Audit, e)
}

func (s *AuthService) CurrentUser(ctx context.Context, token string) (models.UserDTO, error) {
	if s.store == nil {
		return models.UserDTO{}, ErrUnavailable
	}
	if len(token) != 64 {
		return models.UserDTO{}, ErrSession
	}
	digest := sha256.Sum256([]byte(token))
	u, err := s.store.FindSession(ctx, digest[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return models.UserDTO{}, ErrSession
	}
	if err != nil {
		return models.UserDTO{}, fmt.Errorf("load session: %w", err)
	}
	if !u.IsActive {
		return models.UserDTO{}, ErrSession
	}
	return u.Public(), nil
}

// UpdateProfile changes the session owner's first and last name.
func (s *AuthService) UpdateProfile(ctx context.Context, token string, req models.UpdateProfileRequest) (models.UserDTO, error) {
	current, err := s.CurrentUser(ctx, token)
	if err != nil {
		return models.UserDTO{}, err
	}
	first, last, err := req.Normalize()
	if err != nil {
		return models.UserDTO{}, err
	}
	u, err := s.store.UpdateName(ctx, current.ID, first, last)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.UserDTO{}, ErrSession
	}
	if err != nil {
		return models.UserDTO{}, fmt.Errorf("update profile: %w", err)
	}
	return u.Public(), nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if s.store == nil {
		return ErrUnavailable
	}
	if len(token) != 64 {
		return ErrSession
	}
	digest := sha256.Sum256([]byte(token))
	// Look the session up first so the audit trail knows who signed out.
	var who *models.User
	if s.Audit != nil {
		if u, err := s.store.FindSession(ctx, digest[:]); err == nil {
			who = &u
		}
	}
	if err := s.store.DeleteSession(ctx, digest[:]); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if who != nil {
		audit.Record(ctx, s.Audit, audit.Event{Action: audit.Logout,
			ActorID: audit.Ptr(who.ID), ActorEmail: audit.Ptr(who.Email), TargetID: audit.Ptr(who.ID), TargetEmail: audit.Ptr(who.Email)})
	}
	return nil
}
