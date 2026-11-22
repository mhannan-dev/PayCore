package services

import (
	"context"
	"errors"
	"strings"

	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

type NotificationStore interface {
	List(context.Context, models.DeliveryQuery) ([]models.NotificationDelivery, int, error)
	Detail(context.Context, string) (models.DeliveryDetail, error)
	Metrics(context.Context, models.Currency) (models.DeliveryMetrics, error)
	Retry(context.Context, string, int) error
}
type NotificationService struct {
	store NotificationStore
	Audit audit.Recorder
}

func NewNotificationService(store NotificationStore) *NotificationService {
	return &NotificationService{store: store}
}
func (s *NotificationService) List(ctx context.Context, q models.DeliveryQuery) ([]models.NotificationDelivery, int, error) {
	if (q.Currency != "" && !q.Currency.Valid()) || (q.Status != "" && !q.Status.Valid()) || (q.Channel != "" && !q.Channel.Valid()) || q.Limit < 1 || q.Limit > 200 || q.Offset < 0 || q.Offset > 1000000 {
		return nil, 0, models.InvalidRequestError{Code: "invalid_delivery_query", Reason: "Use supported currency, status and channel filters and valid pagination."}
	}
	return s.store.List(ctx, q)
}
func (s *NotificationService) Detail(ctx context.Context, id string) (models.DeliveryDetail, error) {
	if !models.ValidUUID(id) {
		return models.DeliveryDetail{}, models.InvalidRequestError{Code: "invalid_id", Reason: "Delivery ID must be a UUID."}
	}
	return s.store.Detail(ctx, id)
}
func (s *NotificationService) Metrics(ctx context.Context, currency models.Currency) (models.DeliveryMetrics, error) {
	if currency != "" && !currency.Valid() {
		return models.DeliveryMetrics{}, models.InvalidRequestError{Code: "invalid_currency", Reason: "Currency must be JPY, USD or EUR."}
	}
	return s.store.Metrics(ctx, currency)
}
func (s *NotificationService) Retry(ctx context.Context, id string, req models.RetryDeliveryRequest) (models.RetryDeliveryResult, error) {
	if !models.ValidUUID(id) || req.Attempts < 1 || req.Attempts > 1000000 {
		return models.RetryDeliveryResult{}, models.InvalidRequestError{Code: "invalid_delivery_retry", Reason: "Supply a delivery ID and its current attempt count."}
	}
	if err := s.store.Retry(ctx, id, req.Attempts); err != nil {
		return models.RetryDeliveryResult{}, err
	}
	audit.Record(ctx, s.Audit, audit.Event{Action: audit.NotificationRetry, Details: audit.Details{Reason: "Delivery " + id + " queued for retry"}})
	return models.RetryDeliveryResult{Queued: true}, nil
}

type NotificationDeliveryStore interface {
	Prepare(context.Context, models.OutboxEvent) ([]models.NotificationDelivery, error)
	Record(context.Context, models.OutboxEvent, string, *string, *string) error
}
type NotificationSink interface {
	Deliver(context.Context, models.NotificationDelivery) (string, error)
}

// SandboxNotificationSink never uses SMTP or HTTP. The receipt is stable across replays.
type SandboxNotificationSink struct{}

func (s SandboxNotificationSink) Deliver(ctx context.Context, d models.NotificationDelivery) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !d.Channel.Valid() || !models.ValidUUID(d.ID) {
		return "", &PermanentPublishError{Err: errors.New("invalid sandbox notification")}
	}
	return "sandbox/" + strings.ToLower(string(d.Channel)) + "/" + d.ID, nil
}

// NotificationPublisher consumes committed journal events alongside the existing publisher.
// Per-channel receipts survive outbox retries; accepted channels are never resent.
type NotificationPublisher struct {
	store NotificationDeliveryStore
	sink  NotificationSink
	next  EventPublisher
}

func NewNotificationPublisher(store NotificationDeliveryStore, sink NotificationSink, next EventPublisher) *NotificationPublisher {
	return &NotificationPublisher{store: store, sink: sink, next: next}
}
func (p *NotificationPublisher) Publish(ctx context.Context, event models.OutboxEvent) error {
	deliveries, err := p.store.Prepare(ctx, event)
	if err != nil {
		return err
	}
	var failures []error
	for _, d := range deliveries {
		if d.Status == models.DeliveryDelivered {
			continue
		}
		receipt, sendErr := p.sink.Deliver(ctx, d)
		var accepted, lastError *string
		if sendErr == nil && receipt == "" {
			sendErr = errors.New("sandbox notification returned no receipt")
		}
		if sendErr == nil {
			accepted = &receipt
		} else {
			message := truncateError(sendErr)
			lastError = &message
		}
		if err := p.store.Record(ctx, event, d.ID, accepted, lastError); err != nil {
			return err
		}
		if sendErr != nil {
			failures = append(failures, sendErr)
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return p.next.Publish(ctx, event)
}
