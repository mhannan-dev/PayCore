package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

func TestOutboxRepositoryLeaseRetryAndReplay(t *testing.T) {
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := repositories.NewOutboxRepository(pool)

	if _, err := pool.Exec(ctx, `INSERT INTO outbox_events (aggregate_id, event_type, payload)
		VALUES ('journal-1', 'ledger.journal_posted.v1', '{"amount":"50000"}'::jsonb)`); err != nil {
		t.Fatal(err)
	}

	claimed, err := repository.Claim(ctx, 10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %d %v", len(claimed), err)
	}
	event := claimed[0]
	if event.AggregateID != "journal-1" || event.LeaseID == "" || len(event.Payload) == 0 {
		t.Fatalf("incomplete claim: %+v", event)
	}
	if again, err := repository.Claim(ctx, 10, time.Minute); err != nil || len(again) != 0 {
		t.Fatalf("leased event must not be re-claimed: %d %v", len(again), err)
	}

	message := "broker offline"
	if err := repository.Complete(ctx, event, models.OutboxPending, 30*time.Second, &message); err != nil {
		t.Fatal(err)
	}
	if err := repository.Complete(ctx, event, models.OutboxFailed, 0, &message); !errors.Is(err, repositories.ErrOutboxLeaseLost) {
		t.Fatalf("completed lease must be fenced: %v", err)
	}
	metrics, err := repository.GetMetrics(ctx)
	if err != nil || metrics.PendingCount != 1 || metrics.FailedCount != 0 {
		t.Fatalf("retry metrics: %+v %v", metrics, err)
	}
	if again, err := repository.Claim(ctx, 10, time.Minute); err != nil || len(again) != 0 {
		t.Fatalf("backoff must delay retry: %d %v", len(again), err)
	}

	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET available_at = NOW()`); err != nil {
		t.Fatal(err)
	}
	claimed, err = repository.Claim(ctx, 10, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("re-claim after backoff: %d %v", len(claimed), err)
	}
	if err := repository.Complete(ctx, claimed[0], models.OutboxFailed, 0, &message); err != nil {
		t.Fatal(err)
	}
	metrics, err = repository.GetMetrics(ctx)
	if err != nil || metrics.PendingCount != 0 || metrics.FailedCount != 1 {
		t.Fatalf("failed metrics: %+v %v", metrics, err)
	}

	replayed, err := repository.ReplayFailed(ctx)
	if err != nil || replayed != 1 {
		t.Fatalf("replay: %d %v", replayed, err)
	}
	metrics, err = repository.GetMetrics(ctx)
	if err != nil || metrics.PendingCount != 1 || metrics.FailedCount != 0 || !metrics.Healthy {
		t.Fatalf("replayed metrics: %+v %v", metrics, err)
	}
}
