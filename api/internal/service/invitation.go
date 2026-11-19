package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/audit"
	"paycore/api/internal/mailer"
	"paycore/api/internal/model"
)

// InvitationTTL is how long an emailed set-password link stays valid.
const InvitationTTL = 72 * time.Hour

// ErrInvitation means the token is unknown, used, expired, or the user already has a password.
var ErrInvitation = errors.New("invitation invalid or expired")

// ErrWeakPassword means the password breaks the length rule (see PasswordLength).
var ErrWeakPassword = ErrPasswordLength

type InvitationStore interface {
	CreateInvitedUser(ctx context.Context, firstName string, lastName *string, email string, roles []string, tokenHash []byte, expires time.Time, createdBy string) (model.ManagedUser, error)
	ReissueInvitation(ctx context.Context, userID string, tokenHash []byte, expires time.Time, createdBy string) (model.ManagedUser, error)
	FindInvitation(ctx context.Context, tokenHash []byte) (model.InvitationInfo, error)
	AcceptInvitation(ctx context.Context, tokenHash []byte, passwordHash string) (string, error)
	// RecordEmail stores one delivery attempt's outcome (status: pending, sent, failed, not_configured).
	RecordEmail(ctx context.Context, tokenHash []byte, status, emailError string) error
}

// InvitationService invites users by email and lets them set their own password.
// Emails go through an injected delivery queue with a shared lifecycle.
type InvitationService struct {
	EmailQueue
	store  InvitationStore
	appURL string
	// Audit, when set, records invitations sent, re-sent and accepted.
	Audit audit.Recorder
}

// NewInvitationService requires a queue; an unconfigured queue creates invitations
// without sending mail.
func NewInvitationService(store InvitationStore, queue EmailQueue, appURL string) *InvitationService {
	return &InvitationService{EmailQueue: queue, store: store, appURL: strings.TrimRight(appURL, "/")}
}

// Invite creates the user (already normalized by the caller) and queues the invitation
// email. The returned emailStatus is "pending" (sending in the background) or
// "not_configured"; the final outcome is recorded on the invitation.
func (s *InvitationService) Invite(ctx context.Context, req model.CreateUserRequest, invitedBy string) (u model.ManagedUser, emailStatus string, err error) {
	token, digest, err := newInvitationToken()
	if err != nil {
		return model.ManagedUser{}, "", err
	}
	u, err = s.store.CreateInvitedUser(ctx, req.FirstName, req.LastName, req.Email, req.Roles, digest, time.Now().UTC().Add(InvitationTTL), invitedBy)
	if err != nil {
		return model.ManagedUser{}, "", err
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.UserInvited, TargetID: audit.Ptr(u.ID), TargetEmail: audit.Ptr(u.Email),
		Details: audit.Details{RolesAfter: u.Roles}})
	return s.dispatch(ctx, u, token, digest)
}

// Resend replaces the user's outstanding invitation with a fresh link and queues the email.
func (s *InvitationService) Resend(ctx context.Context, userID, invitedBy string) (u model.ManagedUser, emailStatus string, err error) {
	token, digest, err := newInvitationToken()
	if err != nil {
		return model.ManagedUser{}, "", err
	}
	u, err = s.store.ReissueInvitation(ctx, userID, digest, time.Now().UTC().Add(InvitationTTL), invitedBy)
	if err != nil {
		return model.ManagedUser{}, "", err
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.InvitationResent, TargetID: audit.Ptr(u.ID), TargetEmail: audit.Ptr(u.Email)})
	return s.dispatch(ctx, u, token, digest)
}

// dispatch records "not_configured" right away, or starts background delivery.
func (s *InvitationService) dispatch(ctx context.Context, u model.ManagedUser, token string, digest []byte) (model.ManagedUser, string, error) {
	record := func(ctx context.Context, status, emailError string) {
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.store.RecordEmail(recordCtx, digest, status, emailError); err != nil {
			log.Printf("invitation email status for user %s not recorded: %v", u.ID, err)
		}
	}
	status := EmailPending
	if !s.Configured() {
		status = EmailNotConfigured
		record(ctx, status, "no mail server is configured")
	} else {
		msg, err := s.message(u, token)
		if err != nil {
			return u, "", err
		}
		if err := s.Enqueue(ctx, "invitation email for user "+u.ID, msg, record); err != nil {
			status = EmailFailed
			record(ctx, status, "mail queue unavailable; resend the invitation")
		}
	}
	u.InviteEmailStatus = &status
	return u, status, nil
}

// Inspect returns who a usable token belongs to.
func (s *InvitationService) Inspect(ctx context.Context, token string) (model.InvitationInfo, error) {
	info, err := s.store.FindInvitation(ctx, digestToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.InvitationInfo{}, ErrInvitation
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
	if err == nil {
		audit.Record(ctx, s.Audit, audit.Event{Action: audit.InvitationAccepted, ActorEmail: audit.Ptr(email), TargetEmail: audit.Ptr(email)})
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
func (s *InvitationService) message(u model.ManagedUser, token string) (mailer.Message, error) {
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
