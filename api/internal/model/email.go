package model

import (
	"errors"
	"time"
)

var ErrNoEmailJob = errors.New("no email job available")
var ErrEmailLeaseLost = errors.New("email lease no longer owned")

type EmailKind string

const (
	EmailInvitation    EmailKind = "invitation"
	EmailPasswordReset EmailKind = "password_reset"
)

type EmailStatus string

const (
	EmailQueued         EmailStatus = "pending"
	EmailSending        EmailStatus = "sending"
	EmailDelivered      EmailStatus = "sent"
	EmailDeliveryFailed EmailStatus = "failed"
	EmailDisabled       EmailStatus = "not_configured"
	EmailCanceled       EmailStatus = "canceled"
)

// EmailRecipient is the identity read within the token-issuance transaction.
type EmailRecipient struct{ ID, Email, FirstName string }

// EmailContent contains sensitive one-time links. It is never an API response.
type EmailContent struct{ To, Subject, Text, HTML string }

// EmailDraft renders content before commit; nil means delivery is unconfigured.
// Implementations must only render templates, without network or database I/O.
type EmailDraft func(EmailRecipient) (EmailContent, error)

type EmailJob struct {
	ID, LeaseID string
	Kind        EmailKind
	Content     EmailContent
	Attempts    int
	ExpiresAt   time.Time
}

type EmailOutcome struct {
	Status     EmailStatus
	ErrorCode  string
	RetryAfter time.Duration
}
