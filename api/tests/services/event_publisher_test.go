package services_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

func TestWebhookEventPublisherSignsAndPostsEnvelope(t *testing.T) {
	var received models.OutboxEvent
	var signature, eventType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Error(err)
		}
		signature = r.Header.Get("X-PayCore-Signature")
		eventType = r.Header.Get("X-PayCore-Event-Type")
		mac := hmac.New(sha256.New, []byte("secret"))
		mac.Write(body)
		if signature != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("signature does not match the posted body")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	publisher, err := services.NewWebhookEventPublisher(server.URL, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	original := models.OutboxEvent{ID: "event-1", EventType: "ledger.journal_posted.v1"}
	if err := publisher.Publish(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	if received.ID != original.ID || eventType != original.EventType || signature == "" {
		t.Fatalf("envelope not delivered: %+v %q %q", received, eventType, signature)
	}
}

func TestWebhookEventPublisherClassifiesFailures(t *testing.T) {
	status := http.StatusInternalServerError
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	defer server.Close()
	publisher, err := services.NewWebhookEventPublisher(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	event := models.OutboxEvent{ID: "event-1"}

	if err := publisher.Publish(context.Background(), event); err == nil {
		t.Fatal("5xx must be retryable")
	}
	var permanent *services.PermanentPublishError
	if errors.As(publisher.Publish(context.Background(), event), &permanent) {
		t.Fatal("5xx must not be permanent")
	}
	status = http.StatusBadRequest
	if err := publisher.Publish(context.Background(), event); !errors.As(err, &permanent) {
		t.Fatalf("400 must be permanent, got %v", err)
	}
}

func TestWebhookEventPublisherRejectsInvalidURL(t *testing.T) {
	for _, endpoint := range []string{"", "not a url", "ftp://example.com/events", "http://"} {
		if _, err := services.NewWebhookEventPublisher(endpoint, "", time.Second); err == nil {
			t.Fatalf("accepted invalid endpoint %q", endpoint)
		}
	}
}
