package service

import (
	"context"
	"errors"
	"net/textproto"
	"sync"
	"testing"
	"time"

	"paycore/api/internal/mailer"
	"paycore/api/internal/model"
)

type emailJobsFake struct {
	mu       sync.Mutex
	attempts int
	outcomes []model.EmailOutcome
}

func (s *emailJobsFake) Claim(context.Context) (model.EmailJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	return model.EmailJob{ID: "job", Attempts: s.attempts, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (s *emailJobsFake) Complete(_ context.Context, _ model.EmailJob, result model.EmailOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = append(s.outcomes, result)
	return nil
}

type outcomeSender struct{ err error }

func (s outcomeSender) Send(context.Context, mailer.Message) error { return s.err }

func TestEmailWorkerOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		attempt int
		status  model.EmailStatus
		retry   time.Duration
	}{
		{"accepted", nil, 1, model.EmailDelivered, 0},
		{"temporary", errors.New("sensitive upstream error"), 1, model.EmailQueued, 10 * time.Second},
		{"second retry", &textproto.Error{Code: 421, Msg: "retry"}, 2, model.EmailQueued, time.Minute},
		{"permanent", &textproto.Error{Code: 535, Msg: "credential rejected"}, 1, model.EmailDeliveryFailed, 0},
		{"exhausted", errors.New("timeout"), 5, model.EmailDeliveryFailed, 0},
		{"development", mailer.ErrNotDelivered, 1, model.EmailDisabled, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &emailJobsFake{attempts: tc.attempt - 1}
			worker := NewEmailWorker(jobs, outcomeSender{tc.err})
			if err := worker.RunOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(jobs.outcomes) != 1 || jobs.outcomes[0].Status != tc.status || jobs.outcomes[0].RetryAfter != tc.retry {
				t.Fatalf("unexpected outcome: %+v", jobs.outcomes)
			}
			if jobs.outcomes[0].ErrorCode == "sensitive upstream error" {
				t.Fatal("raw upstream error persisted")
			}
		})
	}
}

type blockingMailer struct{ started chan struct{} }

func (m blockingMailer) Send(ctx context.Context, _ mailer.Message) error {
	m.started <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}
func TestEmailWorkerCancellationLeavesClaimsRecoverable(t *testing.T) {
	jobs := &emailJobsFake{}
	sender := blockingMailer{started: make(chan struct{}, 4)}
	worker := NewEmailWorker(jobs, sender)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	for i := 0; i < 4; i++ {
		select {
		case <-sender.started:
		case <-time.After(3 * time.Second):
			t.Fatal("worker pool did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP work did not cancel")
	}
	if jobs.attempts != 4 || len(jobs.outcomes) != 0 {
		t.Fatal("shutdown should leave only four recoverable claims")
	}
}
