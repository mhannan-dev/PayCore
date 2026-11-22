package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

var (
	ErrOutboxPublisher     = errors.New("outbox publisher is not configured")
	ErrInvalidOutboxReplay = errors.New("outbox replay must reference valid event IDs")
)

// EventPublisher abstracts destination message brokers (Kafka, RabbitMQ, webhooks or logs).
type EventPublisher interface {
	Publish(ctx context.Context, event models.OutboxEvent) error
}

// OutboxStore defines database interactions required by OutboxRelay.
type OutboxStore interface {
	Claim(ctx context.Context, batchSize int, lease time.Duration) ([]models.OutboxEvent, error)
	Complete(ctx context.Context, event models.OutboxEvent, status models.OutboxStatus, retryAfter time.Duration, lastError *string) error
	GetMetrics(ctx context.Context) (models.OutboxMetrics, error)
	ReplayFailed(ctx context.Context, ids ...string) (int64, error)
}

// OutboxRelay claims committed outbox events, publishes them and records the
// outcome. Claims are leased rather than locked for the duration of a publish,
// so a broker outage cannot block the ledger or hold database transactions open.
type OutboxRelay struct {
	store       OutboxStore
	publisher   EventPublisher
	Audit       audit.Recorder
	batchSize   int
	maxRetries  int
	interval    time.Duration
	lease       time.Duration
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

// NewOutboxRelay creates a background relay with sensible delivery defaults.
func NewOutboxRelay(store OutboxStore, publisher EventPublisher) *OutboxRelay {
	return &OutboxRelay{
		store:       store,
		publisher:   publisher,
		batchSize:   50,
		maxRetries:  5,
		interval:    time.Second,
		lease:       2 * time.Minute,
		baseBackoff: time.Second,
		maxBackoff:  5 * time.Minute,
	}
}

func (r *OutboxRelay) WithBatchSize(size int) *OutboxRelay {
	if size > 0 {
		r.batchSize = size
	}
	return r
}

func (r *OutboxRelay) WithInterval(d time.Duration) *OutboxRelay {
	if d > 0 {
		r.interval = d
	}
	return r
}

func (r *OutboxRelay) WithMaxRetries(retries int) *OutboxRelay {
	if retries > 0 {
		r.maxRetries = retries
	}
	return r
}

// ProcessBatch claims and publishes a single batch of events.
func (r *OutboxRelay) ProcessBatch(ctx context.Context) (int, error) {
	if r.store == nil || r.publisher == nil {
		return 0, ErrOutboxPublisher
	}
	events, err := r.store.Claim(ctx, r.batchSize, r.lease)
	if err != nil || len(events) == 0 {
		return 0, err
	}

	for _, event := range events {
		status := models.OutboxPublished
		var retryAfter time.Duration
		var lastError *string

		if publishErr := r.publisher.Publish(ctx, event); publishErr != nil {
			status = models.OutboxPending
			retryAfter = r.backoff(event.RetryCount)
			message := truncateError(publishErr)
			lastError = &message
			var permanent *PermanentPublishError
			if errors.As(publishErr, &permanent) || event.RetryCount+1 >= r.maxRetries {
				status = models.OutboxFailed
				retryAfter = 0
			}
			slog.Warn("outbox_event_publish_failed", "id", event.ID, "event_type", event.EventType,
				"attempt", event.RetryCount+1, "status", string(status), "err", publishErr)
		} else {
			slog.Info("outbox_event_published", "id", event.ID, "event_type", event.EventType,
				"aggregate_type", event.AggregateType, "aggregate_id", event.AggregateID)
		}

		if err := r.store.Complete(ctx, event, status, retryAfter, lastError); err != nil {
			return 0, err
		}
	}
	return len(events), nil
}

// backoff grows exponentially with the number of attempts already made.
func (r *OutboxRelay) backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 20 {
		attempt = 20
	}
	delay := r.baseBackoff
	for i := 0; i < attempt; i++ {
		if delay >= r.maxBackoff {
			return r.maxBackoff
		}
		delay *= 2
	}
	if delay > r.maxBackoff {
		return r.maxBackoff
	}
	return delay
}

// Run starts the background worker loop. It drains a backlog in batches and
// exits promptly when ctx is cancelled so unfinished claims can be reclaimed.
func (r *OutboxRelay) Run(ctx context.Context) {
	if r.store == nil || r.publisher == nil {
		return
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for {
			if ctx.Err() != nil {
				return
			}
			count, err := r.ProcessBatch(ctx)
			if err != nil {
				slog.Error("outbox_relay_error", "err", err)
				break
			}
			if count < r.batchSize {
				break
			}
		}
		timer.Reset(r.interval)
	}
}

// Metrics returns the current health and lag of the outbox.
func (r *OutboxRelay) Metrics(ctx context.Context) (models.OutboxMetrics, error) {
	if r.store == nil {
		return models.OutboxMetrics{}, ErrOutboxPublisher
	}
	return r.store.GetMetrics(ctx)
}

// Replay resets failed events so the worker can re-attempt delivery. Operators
// trigger it explicitly, so the action is recorded in the append-only audit log.
func (r *OutboxRelay) Replay(ctx context.Context, ids ...string) (models.OutboxReplayResult, error) {
	if r.store == nil {
		return models.OutboxReplayResult{}, ErrOutboxPublisher
	}
	for _, id := range ids {
		if !validUUID(id) {
			return models.OutboxReplayResult{}, ErrInvalidOutboxReplay
		}
	}
	count, err := r.store.ReplayFailed(ctx, ids...)
	if err != nil {
		return models.OutboxReplayResult{}, err
	}
	result := models.OutboxReplayResult{ReplayedCount: count}
	audit.Record(ctx, r.Audit, audit.Event{
		Action:  audit.OutboxReplay,
		Details: audit.Details{Count: &count},
	})
	return result, nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, character := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}

func truncateError(err error) string {
	message := err.Error()
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
