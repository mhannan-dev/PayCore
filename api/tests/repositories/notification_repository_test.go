package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

func TestNotificationReceiptsRetryAndLeaseFencing(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Transfer(ctx, f.actor, f.key(), transfer("NOTIFY-FUND", f.bank, f.alice, 1000)); err != nil {
		t.Fatal(err)
	}
	queue := repositories.NewOutboxRepository(f.pool)
	repo := repositories.NewNotificationRepository(f.pool)
	service := services.NewNotificationService(repo)
	events, err := queue.Claim(ctx, 10, time.Minute)
	if err != nil || len(events) != 1 {
		t.Fatalf("claim: %v %v", events, err)
	}
	event := events[0]
	deliveries, err := repo.Prepare(ctx, event)
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("prepare: %v %v", deliveries, err)
	}
	var email, webhook models.NotificationDelivery
	for _, d := range deliveries {
		if d.Channel == models.DeliveryEmail {
			email = d
		} else {
			webhook = d
		}
	}
	receipt := "sandbox-email-receipt"
	failure := "sandbox webhook temporarily unavailable"
	if err := repo.Record(ctx, event, email.ID, &receipt, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.Record(ctx, event, webhook.ID, nil, &failure); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, event, models.OutboxFailed, 0, &failure); err != nil {
		t.Fatal(err)
	}
	detail, err := service.Detail(ctx, webhook.ID)
	if err != nil || !detail.Delivery.CanRetry || len(detail.Attempts) != 1 {
		t.Fatalf("failed detail: %+v %v", detail, err)
	}
	request := models.RetryDeliveryRequest{Attempts: detail.Delivery.Attempts}
	outcomes := make(chan error, 2)
	for range 2 {
		go func() { _, err := service.Retry(ctx, webhook.ID, request); outcomes <- err }()
	}
	accepted := 0
	for range 2 {
		err := <-outcomes
		if err == nil {
			accepted++
		} else if !errors.Is(err, models.ErrDeliveryStale) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("concurrent retries accepted %d requests", accepted)
	}
	if _, err := service.Retry(ctx, webhook.ID, request); !errors.Is(err, models.ErrDeliveryStale) {
		t.Fatalf("duplicate retry accepted: %v", err)
	}
	queued, err := service.Detail(ctx, webhook.ID)
	if err != nil || queued.Delivery.CanRetry || queued.Delivery.NextAttemptAt == nil {
		t.Fatalf("queued detail: %+v %v", queued, err)
	}
	events, err = queue.Claim(ctx, 10, time.Minute)
	if err != nil || len(events) != 1 {
		t.Fatalf("retry claim: %v", err)
	}
	current := events[0]
	again, err := repo.Prepare(ctx, current)
	if err != nil || len(again) != 2 {
		t.Fatalf("dedup prepare: %v", err)
	}
	for _, d := range again {
		if d.Channel == models.DeliveryEmail && (d.ID != email.ID || d.Status != models.DeliveryDelivered) {
			t.Fatal("successful channel was recreated")
		}
	}
	if err := repo.Record(ctx, event, webhook.ID, &receipt, nil); !errors.Is(err, repositories.ErrOutboxLeaseLost) {
		t.Fatalf("stale worker was not fenced: %v", err)
	}
	if err := repo.Record(ctx, current, webhook.ID, &receipt, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.Record(ctx, current, webhook.ID, &receipt, nil); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, current, models.OutboxPublished, 0, nil); err != nil {
		t.Fatal(err)
	}
	detail, err = service.Detail(ctx, webhook.ID)
	if err != nil || detail.Delivery.Status != models.DeliveryDelivered || detail.Delivery.Attempts != 2 || len(detail.Attempts) != 2 {
		t.Fatalf("delivered detail: %+v %v", detail, err)
	}
	metrics, err := service.Metrics(ctx, models.CurrencyJPY)
	if err != nil || metrics.Delivered != 2 || metrics.Failed != 0 {
		t.Fatalf("metrics: %+v %v", metrics, err)
	}
	rows, total, err := service.List(ctx, models.DeliveryQuery{Currency: models.CurrencyJPY, Status: models.DeliveryDelivered, Limit: 1, Offset: 1})
	if err != nil || total != 2 || len(rows) != 1 {
		t.Fatalf("pagination: %d %d %v", len(rows), total, err)
	}
	rows, total, err = service.List(ctx, models.DeliveryQuery{Currency: models.CurrencyUSD, Limit: 25})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatal("currencies were mixed")
	}
	if _, err := service.Retry(ctx, webhook.ID, request); !errors.Is(err, models.ErrDeliveryStale) {
		t.Fatal("old failed intent retried a delivered notification")
	}
}
