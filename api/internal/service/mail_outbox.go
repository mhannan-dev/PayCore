package service

import (
	"context"
	"errors"
	"log/slog"
	"net/textproto"
	"sync"
	"time"

	"paycore/api/internal/mailer"
)

// Email delivery outcomes reported to MailOutbox callers.
const (
	EmailPending       = "pending"
	EmailSent          = "sent"
	EmailFailed        = "failed"
	EmailNotConfigured = "not_configured"
)

// MailOutbox sends emails in the background so slow mail servers never hold up an
// HTTP request. Temporary failures are retried; permanent SMTP errors (5xx) are not.
// Every attempt's outcome is reported to the caller's record function.
type MailOutbox struct {
	mail mailer.Sender
	// retryDelays are the waits before the 2nd, 3rd, … delivery attempts.
	retryDelays []time.Duration
	// attemptTimeout bounds one SMTP conversation.
	attemptTimeout time.Duration

	wg       sync.WaitGroup
	stopping chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	closed   bool
	pending  chan struct{}
	active   chan struct{}
	rootCtx  context.Context
	cancel   context.CancelFunc
}

var ErrMailQueueFull = errors.New("mail queue capacity reached")
var ErrMailStopping = errors.New("mail delivery is stopping")

// NewMailOutbox: mail may be nil (nothing is sent; Configured reports false).
type MailOptions struct {
	Workers        int
	Capacity       int
	RetryDelays    []time.Duration
	AttemptTimeout time.Duration
}

// Nonzero options override defaults; retry delays are copied, never shared.
func NewMailOutbox(mail mailer.Sender, options ...MailOptions) *MailOutbox {
	ctx, cancel := context.WithCancel(context.Background())
	workers, capacity, timeout := 4, 64, 45*time.Second
	delays := []time.Duration{10 * time.Second, 60 * time.Second}
	if len(options) > 0 {
		o := options[0]
		if o.Workers > 0 {
			workers = o.Workers
		}
		if o.Capacity > 0 {
			capacity = o.Capacity
		}
		if o.AttemptTimeout > 0 {
			timeout = o.AttemptTimeout
		}
		if o.RetryDelays != nil {
			delays = append([]time.Duration{}, o.RetryDelays...)
		}
	}
	return &MailOutbox{
		mail:           mail,
		retryDelays:    delays,
		attemptTimeout: timeout,
		stopping:       make(chan struct{}),
		pending:        make(chan struct{}, capacity),
		active:         make(chan struct{}, workers),
		rootCtx:        ctx,
		cancel:         cancel,
	}
}

// Configured reports whether a mail sender exists.
func (o *MailOutbox) Configured() bool { return o.mail != nil }

// RecordFunc receives each attempt's outcome: EmailPending (will retry), EmailSent,
// EmailFailed or EmailNotConfigured, plus the error text when there is one.
type RecordFunc func(ctx context.Context, status, emailError string)

// EmailQueue is the lifecycle/delivery contract consumed by invitation/reset use cases.
type EmailQueue interface {
	Configured() bool
	Enqueue(context.Context, string, mailer.Message, RecordFunc) error
	Wait(context.Context)
	Shutdown(context.Context)
}

// Enqueue sends msg in the background. label identifies the email in logs and must
// not contain secrets (never the link or token). Requires Configured().
func (o *MailOutbox) Enqueue(ctx context.Context, label string, msg mailer.Message, record RecordFunc) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrMailStopping
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !o.Configured() {
		return mailer.ErrNotDelivered
	}
	select {
	case o.pending <- struct{}{}:
	default:
		return ErrMailQueueFull
	}
	o.wg.Add(1)
	go o.deliver(context.WithoutCancel(ctx), label, msg, record)
	return nil
}

func (o *MailOutbox) deliver(ctx context.Context, label string, msg mailer.Message, record RecordFunc) {
	defer o.wg.Done()
	defer func() { <-o.pending }()
	jobCtx, stop := context.WithCancel(ctx)
	defer stop()
	stopWatch := context.AfterFunc(o.rootCtx, stop)
	defer stopWatch()
	select {
	case o.active <- struct{}{}:
		defer func() { <-o.active }()
	case <-o.stopping:
		record(ctx, EmailFailed, "mail delivery stopped before sending")
		return
	}
	for attempt := 0; ; attempt++ {
		if jobCtx.Err() != nil {
			record(ctx, EmailFailed, "mail delivery stopped")
			return
		}
		sendCtx, cancel := context.WithTimeout(jobCtx, o.attemptTimeout)
		err := o.mail.Send(sendCtx, msg)
		cancel()
		switch {
		case err == nil:
			slog.Info("mail_accepted", "operation", label, "attempt", attempt+1)
			record(ctx, EmailSent, "")
			return
		case errors.Is(err, mailer.ErrNotDelivered):
			record(ctx, EmailNotConfigured, "no mail server is configured; the link was written to the API log")
			return
		}
		final := attempt >= len(o.retryDelays) || permanent(err)
		slog.Warn("mail_delivery_failed", "operation", label, "attempt", attempt+1, "final", final, "permanent", permanent(err))
		if final {
			record(ctx, EmailFailed, err.Error())
			return
		}
		record(ctx, EmailPending, err.Error())
		select {
		case <-time.After(o.retryDelays[attempt]):
		case <-o.stopping:
			record(ctx, EmailFailed, "server shut down before the email could be retried: "+err.Error())
			return
		}
	}
}

// Shutdown cancels pending retries and waits for in-flight emails, up to ctx's deadline.
func (o *MailOutbox) Shutdown(ctx context.Context) {
	o.stopOnce.Do(func() { o.mu.Lock(); o.closed = true; o.cancel(); close(o.stopping); o.mu.Unlock() })
	o.Wait(ctx)
}

// Wait blocks until every queued email (including retries) has finished, or ctx ends.
func (o *MailOutbox) Wait(ctx context.Context) {
	done := make(chan struct{})
	go func() { o.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// permanent reports SMTP 5xx replies, which retrying will not fix.
func permanent(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply) && reply.Code >= 500 && reply.Code < 600
}

// emailLayout is the HTML layout for single-link emails (password reset). Fields:
// Title, Body, Button, Link, Expires, Footer — all escaped by html/template.
const emailLayout = `<!doctype html>
<html><body style="margin:0;background:#f6f7f4;font-family:Arial,Helvetica,sans-serif;color:#20362d">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" style="max-width:520px;background:#ffffff;border:1px solid #e4e9e3;border-radius:10px" cellpadding="0" cellspacing="0">
<tr><td style="padding:28px 32px 8px"><span style="display:inline-block;background:#245f43;color:#fff;font-weight:700;border-radius:8px;padding:6px 10px">p·</span>
<strong style="font-size:18px;margin-left:8px">PayCore</strong></td></tr>
<tr><td style="padding:12px 32px 0"><h1 style="font-size:20px;margin:0 0 12px">{{.Title}}</h1>
<p style="font-size:14px;line-height:1.6;margin:0 0 20px">{{.Body}}</p>
<p style="margin:0 0 24px"><a href="{{.Link}}" style="display:inline-block;background:#245f43;color:#fff;text-decoration:none;font-weight:600;font-size:14px;padding:12px 20px;border-radius:6px">{{.Button}}</a></p>
<p style="font-size:12px;line-height:1.6;color:#728078;margin:0 0 8px">This link works once and expires {{.Expires}}. If the button does not work, paste this address into your browser:</p>
<p style="font-size:12px;word-break:break-all;color:#245f43;margin:0 0 24px">{{.Link}}</p>
<p style="font-size:12px;color:#728078;margin:0 0 28px">{{.Footer}}</p></td></tr>
</table></td></tr></table></body></html>`
