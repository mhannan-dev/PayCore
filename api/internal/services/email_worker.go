package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"paycore/api/internal/mailer"
	"paycore/api/internal/models"
)

// EmailJobs owns durable claims and fenced outcome updates.
type EmailJobs interface {
	Claim(context.Context) (models.EmailJob, error)
	Complete(context.Context, models.EmailJob, models.EmailOutcome) error
}

type EmailWorker struct {
	jobs   EmailJobs
	sender mailer.Sender
}

func NewEmailWorker(jobs EmailJobs, sender mailer.Sender) *EmailWorker {
	return &EmailWorker{jobs: jobs, sender: sender}
}

// Run starts four bounded delivery loops and returns after cancellation. The
// caller must wait for Run before closing the database pool.
func (w *EmailWorker) Run(ctx context.Context) {
	if w.sender == nil {
		return
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.loop(ctx) }()
	}
	wg.Wait()
}

func (w *EmailWorker) loop(ctx context.Context) {
	for ctx.Err() == nil {
		err := w.RunOne(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, models.ErrNoEmailJob) {
			slog.ErrorContext(ctx, "email_worker_operation_failed")
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// RunOne processes at most one persisted job. SMTP acceptance and database commit
// cannot be atomic: delivery is at least once, including across process crashes.
func (w *EmailWorker) RunOne(ctx context.Context) error {
	if w.sender == nil {
		return models.ErrNoEmailJob
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	claimCtx, endClaim := context.WithTimeout(ctx, 5*time.Second)
	job, err := w.jobs.Claim(claimCtx)
	endClaim()
	if err != nil {
		return err
	}
	sendDeadline := time.Now().Add(45 * time.Second)
	if job.ExpiresAt.Before(sendDeadline) {
		sendDeadline = job.ExpiresAt
	}
	sendCtx, cancel := context.WithDeadline(ctx, sendDeadline)
	err = w.sender.Send(sendCtx, mailer.Message{To: job.Content.To, Subject: job.Content.Subject, Text: job.Content.Text, HTML: job.Content.HTML})
	cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	} // Keep the lease recoverable after shutdown.
	result := models.EmailOutcome{Status: models.EmailDelivered}
	switch {
	case err == nil:
	case errors.Is(err, mailer.ErrNotDelivered):
		result.Status = models.EmailDisabled
		result.ErrorCode = "development_preview"
	case permanent(err):
		result.Status = models.EmailDeliveryFailed
		result.ErrorCode = "smtp_permanent_failure"
	case job.Attempts >= 5:
		result.Status = models.EmailDeliveryFailed
		result.ErrorCode = "attempts_exhausted"
	default:
		result.Status = models.EmailQueued
		result.ErrorCode = "smtp_temporary_failure"
		// Bounded exponential delays: 10s, 60s, 5m, 15m.
		delays := [4]time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute}
		attempt := job.Attempts
		if attempt < 1 {
			attempt = 1
		}
		if attempt > len(delays) {
			attempt = len(delays)
		}
		result.RetryAfter = delays[attempt-1]
	}
	updateCtx, finish := context.WithTimeout(ctx, 5*time.Second)
	defer finish()
	if err := w.jobs.Complete(updateCtx, job, result); err != nil {
		return fmt.Errorf("record email outcome: %w", err)
	}
	slog.InfoContext(ctx, "email_job_completed", "job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "status", result.Status)
	return nil
}
