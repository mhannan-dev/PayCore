package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"paycore/api/internal/models"
)

// LogEventPublisher writes dispatched events to structured log output. It is the
// development default: no external broker is required and every commit remains
// observable. Production should use WebhookEventPublisher.
type LogEventPublisher struct{}

func NewLogEventPublisher() *LogEventPublisher { return &LogEventPublisher{} }

func (p *LogEventPublisher) Publish(_ context.Context, event models.OutboxEvent) error {
	slog.Info("outbox_event_published",
		"id", event.ID,
		"aggregate_type", event.AggregateType,
		"aggregate_id", event.AggregateID,
		"event_type", event.EventType,
	)
	return nil
}

// PermanentPublishError marks a rejection that retrying cannot fix, for example
// a malformed event rejected by the consumer. It goes straight to FAILED.
type PermanentPublishError struct{ Err error }

func (e *PermanentPublishError) Error() string { return "permanent publish error: " + e.Err.Error() }
func (e *PermanentPublishError) Unwrap() error { return e.Err }

// WebhookEventPublisher POSTs the stored envelope to an HTTP sink (a broker
// bridge or consumer inbox). Every request is signed so the receiver can verify
// authenticity without sharing the API's session credentials.
type WebhookEventPublisher struct {
	endpoint string
	secret   string
	client   *http.Client
}

func NewWebhookEventPublisher(endpoint, secret string, timeout time.Duration) (*WebhookEventPublisher, error) {
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid outbox webhook URL")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookEventPublisher{endpoint: endpoint, secret: secret, client: &http.Client{Timeout: timeout}}, nil
}

func (p *WebhookEventPublisher) Publish(ctx context.Context, event models.OutboxEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return &PermanentPublishError{Err: fmt.Errorf("encode outbox event: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return &PermanentPublishError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PayCore-Event-ID", event.ID)
	req.Header.Set("X-PayCore-Event-Type", event.EventType)
	if p.secret != "" {
		mac := hmac.New(sha256.New, []byte(p.secret))
		mac.Write(body) // hash.Hash never returns an error
		req.Header.Set("X-PayCore-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	response, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }() // Read-only: a close error cannot lose data.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	publishErr := fmt.Errorf("outbox webhook returned HTTP %d", response.StatusCode)
	if permanentHTTPStatus(response.StatusCode) {
		return &PermanentPublishError{Err: publishErr}
	}
	return publishErr
}

func permanentHTTPStatus(code int) bool {
	switch code {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusConflict,
		http.StatusGone, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}
