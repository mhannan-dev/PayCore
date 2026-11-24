package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"html/template"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/mailer"
	"paycore/api/internal/models"
)

// InvitationTTL is how long an emailed set-password link stays valid.
const InvitationTTL = 72 * time.Hour

// ErrInvitation means the token is unknown, used, expired, or the user already has a password.
var ErrInvitation = errors.New("invitation invalid or expired")

// ErrWeakPassword means the password breaks the length rule (see PasswordLength).
var ErrWeakPassword = ErrPasswordLength

type InvitationStore interface {
	CreateInvitedUser(ctx context.Context, firstName string, lastName *string, email string, roles []string, tokenHash []byte, expires time.Time, createdBy string, draft ...models.EmailDraft) (models.ManagedUser, error)
	ReissueInvitation(ctx context.Context, userID string, tokenHash []byte, expires time.Time, createdBy string, draft ...models.EmailDraft) (models.ManagedUser, error)
	FindInvitation(ctx context.Context, tokenHash []byte) (models.InvitationInfo, error)
	AcceptInvitation(ctx context.Context, tokenHash []byte, passwordHash string) (string, error)
}

// InvitationService invites users by email and lets them set their own password.
// The store commits invitation state, email intent and its audit event together.
type InvitationService struct {
	emailEnabled bool
	store        InvitationStore
	appURL       string
}

// NewInvitationService receives delivery configuration; workers own delivery.
func NewInvitationService(store InvitationStore, emailEnabled bool, appURL string) *InvitationService {
	return &InvitationService{emailEnabled: emailEnabled, store: store, appURL: strings.TrimRight(appURL, "/")}
}

// Invite creates the user (already normalized by the caller) and queues the invitation
// email. The returned emailStatus is "pending" (sending in the background) or
// "not_configured"; the final outcome is recorded on the invitation.
func (s *InvitationService) Invite(ctx context.Context, req models.CreateUserRequest, invitedBy string) (u models.ManagedUser, emailStatus string, err error) {
	token, digest, err := newInvitationToken()
	if err != nil {
		return models.ManagedUser{}, "", err
	}
	expires := time.Now().UTC().Add(InvitationTTL)
	u, err = s.store.CreateInvitedUser(ctx, req.FirstName, req.LastName, req.Email, req.Roles, digest, expires, invitedBy, s.draft(token, expires))
	if err != nil {
		return models.ManagedUser{}, "", err
	}
	return s.queued(u)
}

// Resend replaces the user's outstanding invitation with a fresh link and queues the email.
func (s *InvitationService) Resend(ctx context.Context, userID, invitedBy string) (u models.ManagedUser, emailStatus string, err error) {
	token, digest, err := newInvitationToken()
	if err != nil {
		return models.ManagedUser{}, "", err
	}
	expires := time.Now().UTC().Add(InvitationTTL)
	u, err = s.store.ReissueInvitation(ctx, userID, digest, expires, invitedBy, s.draft(token, expires))
	if err != nil {
		return models.ManagedUser{}, "", err
	}
	return s.queued(u)
}

func (s *InvitationService) draft(token string, expires time.Time) models.EmailDraft {
	if !s.emailEnabled {
		return nil
	}
	return func(recipient models.EmailRecipient) (models.EmailContent, error) {
		msg, err := s.message(models.ManagedUser{FirstName: recipient.FirstName, Email: recipient.Email, InviteExpiresAt: &expires}, token)
		return models.EmailContent{To: msg.To, Subject: msg.Subject, Text: msg.Text, HTML: msg.HTML}, err
	}
}
func (s *InvitationService) queued(u models.ManagedUser) (models.ManagedUser, string, error) {
	status := EmailNotConfigured
	if s.emailEnabled {
		status = EmailPending
	}
	u.InviteEmailStatus = &status
	return u, status, nil
}

// Inspect returns who a usable token belongs to.
func (s *InvitationService) Inspect(ctx context.Context, token string) (models.InvitationInfo, error) {
	info, err := s.store.FindInvitation(ctx, digestToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.InvitationInfo{}, ErrInvitation
	}
	return info, err
}

// Accept sets the password for a usable token and returns the user's email.
func (s *InvitationService) Accept(ctx context.Context, token, password string) (string, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return "", ErrWeakPassword
	}
	email, err := s.store.AcceptInvitation(ctx, digestToken(token), hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvitation
	}
	return email, err
}

func newInvitationToken() (token string, digest []byte, err error) {
	token, err = randomHex()
	if err != nil {
		return "", nil, err
	}
	return token, digestToken(token), nil
}

func digestToken(token string) []byte {
	sum := sha256.Sum256([]byte(strings.ToLower(token)))
	return sum[:]
}

var invitationHTML = template.Must(template.New("invite").Parse(`<!doctype html>
<html><body style="margin:0;background:#f6f7f4;font-family:Arial,Helvetica,sans-serif;color:#20362d">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" style="max-width:520px;background:#ffffff;border:1px solid #e4e9e3;border-radius:10px" cellpadding="0" cellspacing="0">
<tr><td style="padding:28px 32px 8px"><span style="display:inline-block;background:#245f43;color:#fff;font-weight:700;border-radius:8px;padding:6px 10px">p·</span>
<strong style="font-size:18px;margin-left:8px">PayCore</strong></td></tr>
<tr><td style="padding:12px 32px 0"><h1 style="font-size:20px;margin:0 0 12px">Welcome, {{.Name}}</h1>
<p style="font-size:14px;line-height:1.6;margin:0 0 20px">An administrator created a PayCore account for <strong>{{.Email}}</strong>. Set your password to sign in.</p>
<p style="margin:0 0 24px"><a href="{{.Link}}" style="display:inline-block;background:#245f43;color:#fff;text-decoration:none;font-weight:600;font-size:14px;padding:12px 20px;border-radius:6px">Set your password</a></p>
<p style="font-size:12px;line-height:1.6;color:#728078;margin:0 0 8px">This link works once and expires on {{.Expires}}. If the button does not work, paste this address into your browser:</p>
<p style="font-size:12px;word-break:break-all;color:#245f43;margin:0 0 24px">{{.Link}}</p>
<p style="font-size:12px;color:#728078;margin:0 0 28px">If you did not expect this email, you can ignore it.</p></td></tr>
</table></td></tr></table></body></html>`))

// message renders the invitation email. The token appears only in the link.
func (s *InvitationService) message(u models.ManagedUser, token string) (mailer.Message, error) {
	link := s.appURL + "/set-password#token=" + token
	expires := ""
	if u.InviteExpiresAt != nil {
		expires = u.InviteExpiresAt.UTC().Format("2 Jan 2006 15:04 UTC")
	}
	var html bytes.Buffer
	if err := invitationHTML.Execute(&html, map[string]string{"Name": u.FirstName, "Email": u.Email, "Link": link, "Expires": expires}); err != nil {
		return mailer.Message{}, err
	}
	text := "Welcome to PayCore, " + u.FirstName + ".\n\n" +
		"An administrator created a PayCore account for " + u.Email + ". Set your password here:\n\n" +
		link + "\n\nThe link works once and expires on " + expires + ".\nIf you did not expect this email, you can ignore it.\n"
	return mailer.Message{To: u.Email, Subject: "Set your PayCore password", Text: text, HTML: html.String()}, nil
}
