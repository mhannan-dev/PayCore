package models

import (
	"errors"
	"time"
)

var (
	ErrDeliveryNotFound = errors.New("notification delivery not found")
	ErrDeliveryStale    = errors.New("notification delivery is not retryable or has changed")
)

type DeliveryChannel string

const (
	DeliveryEmail   DeliveryChannel = "EMAIL"
	DeliveryWebhook DeliveryChannel = "WEBHOOK"
)

func (c DeliveryChannel) Valid() bool { return c == DeliveryEmail || c == DeliveryWebhook }

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "PENDING"
	DeliveryDelivered DeliveryStatus = "DELIVERED"
	DeliveryFailed    DeliveryStatus = "FAILED"
)

func (s DeliveryStatus) Valid() bool {
	return s == DeliveryPending || s == DeliveryDelivered || s == DeliveryFailed
}

type DeliveryQuery struct {
	Currency Currency
	Status   DeliveryStatus
	Channel  DeliveryChannel
	Limit    int
	Offset   int
}
type NotificationDelivery struct {
	ID               string          `json:"id"`
	EventID          string          `json:"event_id"`
	EventType        string          `json:"event_type"`
	JournalID        string          `json:"journal_id"`
	JournalReference string          `json:"journal_reference"`
	Currency         Currency        `json:"currency"`
	SettlementID     *string         `json:"settlement_id"`    // Null for non-payout journals.
	PayoutReference  *string         `json:"payout_reference"` // Null for non-payout journals.
	Channel          DeliveryChannel `json:"channel"`
	Destination      string          `json:"destination"`
	Status           DeliveryStatus  `json:"status"`
	DispatchStatus   OutboxStatus    `json:"dispatch_status"`
	Attempts         int             `json:"attempts"`
	LastError        *string         `json:"last_error"`      // Null when no last-attempt failure exists.
	Receipt          *string         `json:"receipt"`         // Null until the sandbox accepts delivery.
	DeliveredAt      *time.Time      `json:"delivered_at"`    // Null until delivery succeeds.
	NextAttemptAt    *time.Time      `json:"next_attempt_at"` // Null when no automatic retry is queued.
	CanRetry         bool            `json:"can_retry"`
	CreatedAt        time.Time       `json:"created_at"`
}
type DeliveryAttempt struct {
	Attempt    int            `json:"attempt"`
	Status     DeliveryStatus `json:"status"`
	Receipt    *string        `json:"receipt"` // Null for failed attempts.
	Error      *string        `json:"error"`   // Null for successful attempts.
	OccurredAt time.Time      `json:"occurred_at"`
}
type DeliveryDetail struct {
	Delivery NotificationDelivery `json:"delivery"`
	Attempts []DeliveryAttempt    `json:"attempts"`
}
type DeliveryMetrics struct {
	Pending   int `json:"pending"`
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
}
type RetryDeliveryRequest struct {
	Attempts int `json:"attempts" binding:"required,gte=1,lte=1000000"`
}
type RetryDeliveryResult struct {
	Queued bool `json:"queued"`
}
