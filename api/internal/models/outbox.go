package models

import (
	"encoding/json"
	"time"
)

// OutboxStatus represents the processing state of an outbox event.
type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "PENDING"
	OutboxSending   OutboxStatus = "SENDING"
	OutboxPublished OutboxStatus = "PUBLISHED"
	OutboxFailed    OutboxStatus = "FAILED"
)

// Valid reports whether s is one of the database-supported delivery states.
func (s OutboxStatus) Valid() bool {
	switch s {
	case OutboxPending, OutboxSending, OutboxPublished, OutboxFailed:
		return true
	default:
		return false
	}
}

// OutboxEvent is the versioned envelope handed to a publisher. Payload is the
// JSON stored by the ledger transaction; it is never parsed or rebuilt here.
type OutboxEvent struct {
	ID            string          `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	RetryCount    int             `json:"retry_count"`
	CreatedAt     time.Time       `json:"created_at"`
	AvailableAt   time.Time       `json:"available_at"`
	// LeaseID is the fencing token used when a claim is completed. It is never exposed.
	LeaseID string `json:"-"`
	// LastError is the most recent delivery failure, if any.
	LastError *string `json:"last_error,omitempty"`
}

// OutboxMetrics summarizes the health and lag of the transactional outbox queue.
type OutboxMetrics struct {
	PendingCount   int64     `json:"pending_count"`
	SendingCount   int64     `json:"sending_count"`
	PublishedCount int64     `json:"published_count"`
	FailedCount    int64     `json:"failed_count"`
	LagSeconds     float64   `json:"lag_seconds"`
	Healthy        bool      `json:"healthy"`
	CheckedAt      time.Time `json:"checked_at"`
}

// OutboxReplayRequest optionally selects specific event IDs to replay. An empty
// list replays every failed event.
type OutboxReplayRequest struct {
	IDs []string `json:"ids" binding:"max=200,dive,uuid"`
}

// OutboxReplayResult indicates how many failed events were reset to PENDING.
type OutboxReplayResult struct {
	ReplayedCount int64 `json:"replayed_count"`
}
