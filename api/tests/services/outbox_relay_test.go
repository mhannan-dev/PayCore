package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/audit"
	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

type fakeOutboxStore struct {
	events      []models.OutboxEvent
	claimed     int
	completions []completion
	metrics     models.OutboxMetrics
	replayed    []string
	claimErr    error
	completeErr error
}

type completion struct {
	event      models.OutboxEvent
	status     models.OutboxStatus
	retryAfter time.Duration
	lastError  *string
}

func (f *fakeOutboxStore) Claim(_ context.Context, batchSize int, _ time.Duration) ([]models.OutboxEvent, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if batchSize > len(f.events) {
		batchSize = len(f.events)
	}
	events := f.events[:batchSize]
	f.events = f.events[batchSize:]
	f.claimed += len(events)
	return events, nil
}

func (f *fakeOutboxStore) Complete(_ context.Context, event models.OutboxEvent, status models.OutboxStatus, retryAfter time.Duration, lastError *string) error {
	f.completions = append(f.completions, completion{event, status, retryAfter, lastError})
	return f.completeErr
}

func (f *fakeOutboxStore) GetMetrics(context.Context) (models.OutboxMetrics, error) {
	return f.metrics, nil
}

func (f *fakeOutboxStore) ReplayFailed(_ context.Context, ids ...string) (int64, error) {
	f.replayed = append(f.replayed, ids...)
	return int64(len(ids)), nil
}

type fakePublisher struct {
	err   error
	calls int
}

func (p *fakePublisher) Publish(context.Context, models.OutboxEvent) error {
	p.calls++
	return p.err
}

type recordingAudit struct{ events []audit.Event }

func (a *recordingAudit) Record(_ context.Context, event audit.Event) error {
	a.events = append(a.events, event)
	return nil
}

func event(id string, retries int) models.OutboxEvent {
	return models.OutboxEvent{ID: id, EventType: "ledger.journal_posted.v1", RetryCount: retries, LeaseID: "lease"}
}

func TestOutboxRelayPublishesAndMarksEvents(t *testing.T) {
	store := &fakeOutboxStore{events: []models.OutboxEvent{event("one", 0), event("two", 0)}}
	publisher := &fakePublisher{}
	relay := services.NewOutboxRelay(store, publisher)

	processed, err := relay.ProcessBatch(context.Background())
	if err != nil || processed != 2 || publisher.calls != 2 || len(store.completions) != 2 {
		t.Fatalf("processed=%d err=%v calls=%d completions=%d", processed, err, publisher.calls, len(store.completions))
	}
	for _, c := range store.completions {
		if c.status != models.OutboxPublished || c.retryAfter != 0 || c.lastError != nil {
			t.Fatalf("unexpected completion: %+v", c)
		}
	}
}

func TestOutboxRelayRetriesTransientAndFailsPermanentErrors(t *testing.T) {
	transient := &fakePublisher{err: errors.New("broker timeout")}
	store := &fakeOutboxStore{events: []models.OutboxEvent{event("one", 1)}}
	relay := services.NewOutboxRelay(store, transient).WithMaxRetries(5)
	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := store.completions[0]
	if got.status != models.OutboxPending || got.retryAfter != 2*time.Second || got.lastError == nil {
		t.Fatalf("transient failure must back off: %+v", got)
	}

	permanent := &fakePublisher{err: &services.PermanentPublishError{Err: errors.New("invalid envelope")}}
	store = &fakeOutboxStore{events: []models.OutboxEvent{event("two", 0)}}
	relay = services.NewOutboxRelay(store, permanent)
	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	got = store.completions[0]
	if got.status != models.OutboxFailed || got.retryAfter != 0 {
		t.Fatalf("permanent failure must be terminal: %+v", got)
	}
}

func TestOutboxRelayStopsRetryingAfterMaxRetries(t *testing.T) {
	store := &fakeOutboxStore{events: []models.OutboxEvent{event("one", 4)}}
	relay := services.NewOutboxRelay(store, &fakePublisher{err: errors.New("offline")})
	if _, err := relay.ProcessBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := store.completions[0].status; status != models.OutboxFailed {
		t.Fatalf("exhausted retries should fail, got %s", status)
	}
}

func TestOutboxRelayReplayValidatesAndAudits(t *testing.T) {
	id := "d3b07384-d113-4665-b778-f076efef1111"
	store := &fakeOutboxStore{}
	recorder := &recordingAudit{}
	relay := services.NewOutboxRelay(store, &fakePublisher{})
	relay.Audit = recorder

	if _, err := relay.Replay(context.Background(), "not-a-uuid"); !errors.Is(err, services.ErrInvalidOutboxReplay) {
		t.Fatalf("want invalid replay error, got %v", err)
	}
	result, err := relay.Replay(context.Background(), id)
	if err != nil || result.ReplayedCount != 1 {
		t.Fatalf("replay: %+v %v", result, err)
	}
	if len(store.replayed) != 1 || store.replayed[0] != id {
		t.Fatalf("store replay ids: %v", store.replayed)
	}
	if len(recorder.events) != 1 || recorder.events[0].Action != audit.OutboxReplay {
		t.Fatalf("replay must be audited: %+v", recorder.events)
	}
}

func TestOutboxRelayStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	relay := services.NewOutboxRelay(&fakeOutboxStore{}, &fakePublisher{}).WithInterval(10 * time.Millisecond)
	done := make(chan struct{})
	go func() { defer close(done); relay.Run(ctx) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
}
