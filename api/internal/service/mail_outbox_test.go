package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/mailer"
)

type blockingMailer struct{ started chan struct{} }

func (m blockingMailer) Send(ctx context.Context, _ mailer.Message) error {
	m.started <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

func TestMailAdmissionAndShutdown(t *testing.T) {
	sender := blockingMailer{started: make(chan struct{}, 2)}
	queue := NewMailOutbox(sender, MailOptions{Workers: 1, Capacity: 2, RetryDelays: []time.Duration{}, AttemptTimeout: time.Minute})
	recorded := make(chan string, 2)
	record := func(_ context.Context, status, _ string) { recorded <- status }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := queue.Enqueue(ctx, "test", mailer.Message{}, record); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sender.started:
	case <-ctx.Done():
		t.Fatal("mail did not start")
	}
	if err := queue.Enqueue(ctx, "test", mailer.Message{}, record); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(ctx, "test", mailer.Message{}, record); !errors.Is(err, ErrMailQueueFull) {
		t.Fatalf("capacity not enforced: %v", err)
	}
	queue.Shutdown(ctx)
	if ctx.Err() != nil {
		t.Fatal("shutdown did not cancel active SMTP")
	}
	if err := queue.Enqueue(ctx, "test", mailer.Message{}, record); !errors.Is(err, ErrMailStopping) {
		t.Fatalf("admission remained open: %v", err)
	}
	if len(recorded) != 2 {
		t.Fatal("queued/active jobs were not accounted for at shutdown")
	}
}
