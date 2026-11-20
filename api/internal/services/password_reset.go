package services

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/audit"
	"paycore/api/internal/mailer"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
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
	IssueResetToken(ctx context.Context, email string, tokenHash []byte, expires time.Time, cooldown time.Duration, draft ...models.EmailDraft) (models.ResetUser, error)
	FindResetToken(ctx context.Context, tokenHash []byte) (models.PasswordResetInfo, error)
	ResetPassword(ctx context.Context, tokenHash []byte, passwordHash string) (string, error)
}

// PasswordResetService implements "forgot password". Requests never reveal whether
// an account exists: unknown emails, inactive users and throttled requests all
// succeed silently. Email intent is committed with the token.
type PasswordResetService struct {
	emailEnabled bool
	store        PasswordResetStore
	captcha      CaptchaVerifier
	appURL       string
	// Audit, when set, records issued reset links and completed resets.
	Audit audit.Recorder
}

func NewPasswordResetService(store PasswordResetStore, captcha CaptchaVerifier, emailEnabled bool, appURL string) *PasswordResetService {
	return &PasswordResetService{emailEnabled: emailEnabled, store: store, captcha: captcha, appURL: strings.TrimRight(appURL, "/")}
}

// Request verifies the captcha and, when an active account has the email, queues a
// reset email. It returns ErrCaptcha, ErrUnavailable, or nil (whether or not an
// account exists).
func (s *PasswordResetService) Request(ctx context.Context, req models.PasswordResetRequest) error {
	if s.captcha == nil || !s.captcha.Verify(req.CaptchaID, req.CaptchaAnswer) {
		return ErrCaptcha
	}
	token, digest, err := newInvitationToken()
	if err != nil {
		return ErrUnavailable
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	expires := time.Now().UTC().Add(PasswordResetTTL)
	u, err := s.store.IssueResetToken(ctx, email, digest, expires, PasswordResetCooldown, s.draft(token, expires))
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, repositories.ErrResetThrottled):
		return nil // same answer as success: no account enumeration
	case err != nil:
		return ErrUnavailable
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.PasswordResetRequested, TargetID: audit.Ptr(u.ID), TargetEmail: audit.Ptr(u.Email)})

	return nil
}

// Inspect returns who a usable reset token belongs to.
func (s *PasswordResetService) Inspect(ctx context.Context, token string) (models.PasswordResetInfo, error) {
	info, err := s.store.FindResetToken(ctx, digestToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.PasswordResetInfo{}, ErrResetToken
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
	if err == nil {
		audit.Record(ctx, s.Audit, audit.Event{Action: audit.PasswordResetCompleted, ActorEmail: audit.Ptr(email), TargetEmail: audit.Ptr(email)})
	}
	return email, err
}

var resetHTML = template.Must(template.New("reset").Parse(emailLayout))

func (s *PasswordResetService) message(u models.ResetUser, token string, expires time.Time) (mailer.Message, error) {
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

func (s *PasswordResetService) draft(token string, expires time.Time) models.EmailDraft {
	if !s.emailEnabled {
		return nil
	}
	return func(recipient models.EmailRecipient) (models.EmailContent, error) {
		msg, err := s.message(models.ResetUser{ID: recipient.ID, FirstName: recipient.FirstName, Email: recipient.Email}, token, expires)
		return models.EmailContent{To: msg.To, Subject: msg.Subject, Text: msg.Text, HTML: msg.HTML}, err
	}
}
