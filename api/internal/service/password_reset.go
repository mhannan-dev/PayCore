package service

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/mailer"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
)

const (
	// PasswordResetTTL is how long an emailed reset link stays valid.
	PasswordResetTTL = 30 * time.Minute
	// PasswordResetCooldown is the minimum time between reset emails for one account.
	PasswordResetCooldown = time.Minute
)

// ErrResetToken means the reset token is unknown, used or expired.
var ErrResetToken = errors.New("password reset link invalid or expired")

type PasswordResetStore interface {
	IssueResetToken(ctx context.Context, email string, tokenHash []byte, expires time.Time, cooldown time.Duration) (model.ResetUser, error)
	FindResetToken(ctx context.Context, tokenHash []byte) (model.PasswordResetInfo, error)
	ResetPassword(ctx context.Context, tokenHash []byte, passwordHash string) (string, error)
}

// PasswordResetService implements "forgot password". Requests never reveal whether
// an account exists: unknown emails, inactive users and throttled requests all
// succeed silently. Emails go through the embedded MailOutbox.
type PasswordResetService struct {
	*MailOutbox
	store   PasswordResetStore
	captcha CaptchaVerifier
	appURL  string
}

func NewPasswordResetService(store PasswordResetStore, captcha CaptchaVerifier, mail mailer.Sender, appURL string) *PasswordResetService {
	return &PasswordResetService{MailOutbox: NewMailOutbox(mail), store: store, captcha: captcha, appURL: strings.TrimRight(appURL, "/")}
}

// Request verifies the captcha and, when an active account has the email, queues a
// reset email. It returns ErrCaptcha, ErrUnavailable, or nil (whether or not an
// account exists).
func (s *PasswordResetService) Request(ctx context.Context, req model.PasswordResetRequest) error {
	if s.captcha == nil || !s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer) {
		return ErrCaptcha
	}
	token, digest, err := newInvitationToken()
	if err != nil {
		return ErrUnavailable
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	expires := time.Now().UTC().Add(PasswordResetTTL)
	u, err := s.store.IssueResetToken(ctx, email, digest, expires, PasswordResetCooldown)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, repository.ErrResetThrottled):
		return nil // same answer as success: no account enumeration
	case err != nil:
		return ErrUnavailable
	}
	if !s.Configured() {
		log.Printf("password reset for user %s: no mail server is configured; nothing sent", u.ID)
		return nil
	}
	msg, err := s.message(u, token, expires)
	if err != nil {
		return ErrUnavailable
	}
	s.Enqueue(ctx, "password reset email for user "+u.ID, msg, func(context.Context, string, string) {})
	return nil
}

// Inspect returns who a usable reset token belongs to.
func (s *PasswordResetService) Inspect(ctx context.Context, token string) (model.PasswordResetInfo, error) {
	info, err := s.store.FindResetToken(ctx, digestToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.PasswordResetInfo{}, ErrResetToken
	}
	return info, err
}

// Confirm sets the new password for a usable token, signs the user out everywhere,
// and returns their email.
func (s *PasswordResetService) Confirm(ctx context.Context, token, password string) (string, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return "", ErrWeakPassword
	}
	email, err := s.store.ResetPassword(ctx, digestToken(token), hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrResetToken
	}
	return email, err
}

var resetHTML = template.Must(template.New("reset").Parse(emailLayout))

func (s *PasswordResetService) message(u model.ResetUser, token string, expires time.Time) (mailer.Message, error) {
	link := s.appURL + "/reset-password#token=" + token
	when := "at " + expires.Format("15:04 UTC") + " (in 30 minutes)"
	var html bytes.Buffer
	err := resetHTML.Execute(&html, map[string]string{
		"Title":   "Reset your password",
		"Body":    "Hi " + u.FirstName + ", someone (hopefully you) asked to reset the PayCore password for " + u.Email + ". Choose a new password with the button below.",
		"Button":  "Choose a new password",
		"Link":    link,
		"Expires": when,
		"Footer":  "If you did not ask for this, ignore this email: your password stays the same.",
	})
	if err != nil {
		return mailer.Message{}, err
	}
	text := "Hi " + u.FirstName + ",\n\nSomeone (hopefully you) asked to reset the PayCore password for " + u.Email + ".\n" +
		"Choose a new password here:\n\n" + link + "\n\nThe link works once and expires " + when + ".\n" +
		"If you did not ask for this, ignore this email: your password stays the same.\n"
	return mailer.Message{To: u.Email, Subject: "Reset your PayCore password", Text: text, HTML: html.String()}, nil
}
