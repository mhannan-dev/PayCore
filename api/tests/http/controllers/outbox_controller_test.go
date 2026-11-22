package controllers_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
)

type fakeOutboxService struct {
	metrics  models.OutboxMetrics
	replayed []string
}

func (f *fakeOutboxService) Metrics(context.Context) (models.OutboxMetrics, error) {
	return f.metrics, nil
}

func (f *fakeOutboxService) Replay(_ context.Context, ids ...string) (models.OutboxReplayResult, error) {
	f.replayed = ids
	return models.OutboxReplayResult{ReplayedCount: int64(len(ids))}, nil
}

func TestOutboxHTTP(t *testing.T) {
	caller := models.User{
		ID: adminID, Email: "admin@example.test", Roles: []string{"ADMIN"}, IsActive: true,
		Permissions: []string{models.PermSystemRead, models.PermSystemWrite},
	}
	auth, token := sessionAuth(t, caller)
	svc := &fakeOutboxService{metrics: models.OutboxMetrics{PendingCount: 3, Healthy: true, LagSeconds: 2, CheckedAt: time.Now().UTC()}}
	router := server.New(nil, server.Controllers{
		Outbox:         controllers.NewOutboxController(svc),
		AuthMiddleware: middleware.NewAuth(auth),
	})
	r := &testRouter{router}

	w := doWithKey(r, http.MethodGet, "/api/v1/outbox/metrics", "", token, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"pending_count":3`) || !strings.Contains(w.Body.String(), `"healthy":true`) {
		t.Fatalf("metrics: %d %s", w.Code, w.Body)
	}

	w = doWithKey(r, http.MethodPost, "/api/v1/outbox/replay", `{"ids":["3f1c2a9e-6f0b-4b8e-9b51-2d0f6c1e7a10"]}`, token, "")
	if w.Code != http.StatusOK || len(svc.replayed) != 1 || !strings.Contains(w.Body.String(), `"replayed_count":1`) {
		t.Fatalf("replay: %d %s %v", w.Code, w.Body, svc.replayed)
	}

	w = doWithKey(r, http.MethodPost, "/api/v1/outbox/replay", `{"ids":["not-a-uuid"]}`, token, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid id must be rejected: %d %s", w.Code, w.Body)
	}
	w = doWithKey(r, http.MethodPost, "/api/v1/outbox/replay", `{"ids":[],"unexpected":true}`, token, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown fields must be rejected: %d %s", w.Code, w.Body)
	}
}
