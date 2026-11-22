package services_test

import (
	"context"
	"errors"
	"testing"

	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

type notificationMemory struct {
	items     []models.NotificationDelivery
	recordErr error
}

func (s *notificationMemory) Prepare(context.Context, models.OutboxEvent) ([]models.NotificationDelivery, error) {
	return append([]models.NotificationDelivery{}, s.items...), nil
}
func (s *notificationMemory) Record(_ context.Context, _ models.OutboxEvent, id string, receipt, failure *string) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Attempts++
			s.items[i].Receipt = receipt
			s.items[i].LastError = failure
			s.items[i].Status = models.DeliveryFailed
			if receipt != nil {
				s.items[i].Status = models.DeliveryDelivered
			}
		}
	}
	return nil
}

type notificationSink struct {
	calls       map[models.DeliveryChannel]int
	failWebhook bool
}

func (s *notificationSink) Deliver(_ context.Context, d models.NotificationDelivery) (string, error) {
	s.calls[d.Channel]++
	if d.Channel == models.DeliveryWebhook && s.failWebhook {
		return "", errors.New("sandbox temporarily unavailable")
	}
	return "sandbox/" + d.ID, nil
}

type notificationNext struct{ calls int }

func (n *notificationNext) Publish(context.Context, models.OutboxEvent) error { n.calls++; return nil }
func TestNotificationPublisherSkipsSuccessfulChannels(t *testing.T) {
	store := &notificationMemory{items: []models.NotificationDelivery{{ID: "email", Channel: models.DeliveryEmail, Status: models.DeliveryPending}, {ID: "webhook", Channel: models.DeliveryWebhook, Status: models.DeliveryPending}}}
	sink := &notificationSink{calls: map[models.DeliveryChannel]int{}, failWebhook: true}
	next := &notificationNext{}
	publisher := services.NewNotificationPublisher(store, sink, next)
	event := models.OutboxEvent{ID: "event", LeaseID: "lease"}
	if err := publisher.Publish(context.Background(), event); err == nil {
		t.Fatal("expected transient delivery failure")
	}
	if next.calls != 0 || store.items[0].Status != models.DeliveryDelivered || store.items[1].Status != models.DeliveryFailed {
		t.Fatal("partial results were not saved")
	}
	sink.failWebhook = false
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if sink.calls[models.DeliveryEmail] != 1 || sink.calls[models.DeliveryWebhook] != 2 || next.calls != 1 {
		t.Fatalf("duplicate delivery: %+v", sink.calls)
	}
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if sink.calls[models.DeliveryEmail] != 1 || sink.calls[models.DeliveryWebhook] != 2 {
		t.Fatal("published replay resent accepted channels")
	}
}
func TestNotificationPublisherRequiresPersistentReceipt(t *testing.T) {
	store := &notificationMemory{items: []models.NotificationDelivery{{ID: "email", Channel: models.DeliveryEmail, Status: models.DeliveryPending}}, recordErr: errors.New("database unavailable")}
	sink := &notificationSink{calls: map[models.DeliveryChannel]int{}}
	next := &notificationNext{}
	if err := services.NewNotificationPublisher(store, sink, next).Publish(context.Background(), models.OutboxEvent{}); err == nil || next.calls != 0 {
		t.Fatal("event acknowledged without a saved delivery outcome")
	}
}
func TestSandboxNotificationReceiptIsStable(t *testing.T) {
	sink := services.SandboxNotificationSink{}
	delivery := models.NotificationDelivery{ID: "22222222-2222-4222-8222-222222222222", Channel: models.DeliveryEmail}
	first, err := sink.Deliver(context.Background(), delivery)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := sink.Deliver(context.Background(), delivery)
	if err != nil || first != replay {
		t.Fatal("sandbox receipt changed on replay")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sink.Deliver(ctx, delivery); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

type notificationQueries struct {
	calls    int
	retryErr error
}

func (s *notificationQueries) List(context.Context, models.DeliveryQuery) ([]models.NotificationDelivery, int, error) {
	s.calls++
	return nil, 0, nil
}
func (s *notificationQueries) Detail(context.Context, string) (models.DeliveryDetail, error) {
	s.calls++
	return models.DeliveryDetail{}, nil
}
func (s *notificationQueries) Metrics(context.Context, models.Currency) (models.DeliveryMetrics, error) {
	s.calls++
	return models.DeliveryMetrics{}, nil
}
func (s *notificationQueries) Retry(context.Context, string, int) error { s.calls++; return s.retryErr }
func TestNotificationValidationAndStaleRetries(t *testing.T) {
	store := &notificationQueries{}
	service := services.NewNotificationService(store)
	ctx := context.Background()
	for _, q := range []models.DeliveryQuery{{Currency: "GBP", Limit: 25}, {Status: "UNKNOWN", Limit: 25}, {Channel: "SMS", Limit: 25}, {Limit: 0}} {
		if _, _, err := service.List(ctx, q); err == nil {
			t.Fatalf("invalid filter accepted: %+v", q)
		}
	}
	if _, err := service.Detail(ctx, "bad-id"); err == nil {
		t.Fatal("invalid ID accepted")
	}
	if _, err := service.Retry(ctx, "22222222-2222-4222-8222-222222222222", models.RetryDeliveryRequest{}); err == nil {
		t.Fatal("unversioned retry accepted")
	}
	if store.calls != 0 {
		t.Fatal("invalid request reached storage")
	}
	store.retryErr = models.ErrDeliveryStale
	if _, err := service.Retry(ctx, "22222222-2222-4222-8222-222222222222", models.RetryDeliveryRequest{Attempts: 1}); !errors.Is(err, models.ErrDeliveryStale) {
		t.Fatalf("stale error lost: %v", err)
	}
}
