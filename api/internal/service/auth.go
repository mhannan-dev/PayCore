package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
)

const PasswordCost = 12
const SessionTTL = 24 * time.Hour

var ErrCredentials = errors.New("invalid email or password")
var ErrCaptcha = errors.New("invalid or expired captcha")
var ErrUnavailable = errors.New("authentication unavailable")
var ErrSession = errors.New("invalid session")

type CaptchaVerifier interface{ Verify(string, string) bool }
type AuthService struct {
	store     repository.AuthStore
	captcha   CaptchaVerifier
	dummyHash []byte
}

func NewAuthService(store repository.AuthStore, captcha CaptchaVerifier) (*AuthService, error) {
	// Perform the same password KDF for unknown and known emails.
	dummy, err := bcrypt.GenerateFromPassword([]byte("unused-dummy-password-for-unknown-users"), PasswordCost)
	if err != nil {
		return nil, err
	}
	return &AuthService{store: store, captcha: captcha, dummyHash: dummy}, nil
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

func (s *AuthService) Login(ctx context.Context, req model.LoginRequest) (model.LoginResponse, error) {
	if s.store == nil {
		return model.LoginResponse{}, ErrUnavailable
	}
	if len(req.Password) > 72 {
		return model.LoginResponse{}, ErrCredentials
	}
	if s.captcha == nil || !s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer) {
		return model.LoginResponse{}, ErrCaptcha
	}
	u, err := s.store.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.LoginResponse{}, ErrUnavailable
	}
	// Unknown emails and invited users without a password still pay for one bcrypt
	// comparison, so response time does not reveal which accounts exist.
	hash := s.dummyHash
	if err == nil && u.PasswordHash != "" {
		hash = []byte(u.PasswordHash)
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(req.Password))
	if err != nil || u.PasswordHash == "" || match != nil || !u.IsActive {
		return model.LoginResponse{}, ErrCredentials
	}
	token, err := randomHex()
	if err != nil {
		return model.LoginResponse{}, ErrUnavailable
	}
	digest := sha256.Sum256([]byte(token))
	if err = s.store.CreateSession(ctx, u.ID, digest[:], time.Now().UTC().Add(SessionTTL)); err != nil {
		if errors.Is(err, repository.ErrInactiveUser) {
			return model.LoginResponse{}, ErrCredentials
		}
		return model.LoginResponse{}, ErrUnavailable
	}
	return model.LoginResponse{Token: token, TokenType: "Bearer", ExpiresIn: int64(SessionTTL.Seconds()), User: u.Public()}, nil
}

func (s *AuthService) CurrentUser(ctx context.Context, token string) (model.UserDTO, error) {
	if s.store == nil {
		return model.UserDTO{}, ErrUnavailable
	}
	if len(token) != 64 {
		return model.UserDTO{}, ErrSession
	}
	digest := sha256.Sum256([]byte(token))
	u, err := s.store.FindSession(ctx, digest[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return model.UserDTO{}, ErrSession
	}
	if err != nil {
		return model.UserDTO{}, ErrUnavailable
	}
	if !u.IsActive {
		return model.UserDTO{}, ErrSession
	}
	return u.Public(), nil
}

// UpdateProfile changes the session owner's first and last name.
func (s *AuthService) UpdateProfile(ctx context.Context, token string, req model.UpdateProfileRequest) (model.UserDTO, error) {
	current, err := s.CurrentUser(ctx, token)
	if err != nil {
		return model.UserDTO{}, err
	}
	first, last, err := req.Normalize()
	if err != nil {
		return model.UserDTO{}, err
	}
	u, err := s.store.UpdateName(ctx, current.ID, first, last)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.UserDTO{}, ErrSession
	}
	if err != nil {
		return model.UserDTO{}, ErrUnavailable
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
	if err := s.store.DeleteSession(ctx, digest[:]); err != nil {
		return ErrUnavailable
	}
	return nil
}
