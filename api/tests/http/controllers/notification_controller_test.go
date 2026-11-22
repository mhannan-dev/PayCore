package controllers_test

import (
	"context"
	"net/http"
	"testing"

	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
	"paycore/api/internal/services"
)

type notificationTransportStore struct {
	query   models.DeliveryQuery
	retried bool
	stale   bool
}

func (s *notificationTransportStore) List(_ context.Context, q models.DeliveryQuery) ([]models.NotificationDelivery, int, error) {
	s.query = q
	return nil, 0, nil
}
func (s *notificationTransportStore) Metrics(context.Context, models.Currency) (models.DeliveryMetrics, error) {
	return models.DeliveryMetrics{}, nil
}
func (s *notificationTransportStore) Detail(context.Context, string) (models.DeliveryDetail, error) {
	return models.DeliveryDetail{}, models.ErrDeliveryNotFound
}
func (s *notificationTransportStore) Retry(context.Context, string, int) error {
	if s.stale {
		return models.ErrDeliveryStale
	}
	s.retried = true
	return nil
}
func TestNotificationEndpoints(t *testing.T) {
	caller := models.User{ID: adminID, Email: "operator@example.test", IsActive: true, Permissions: []string{models.PermNotificationsRead, models.PermNotificationsRetry}}
	auth, token := sessionAuth(t, caller)
	store := &notificationTransportStore{}
	router := server.New(nil, server.Controllers{Notifications: controllers.NewNotificationController(services.NewNotificationService(store)), AuthMiddleware: middleware.NewAuth(auth)})
	r := &testRouter{router}
	w := doWithKey(r, http.MethodGet, "/api/v1/notifications?currency=usd&status=failed&channel=webhook&limit=10&offset=20", "", token, "")
	if w.Code != http.StatusOK || store.query.Currency != models.CurrencyUSD || store.query.Status != models.DeliveryFailed || store.query.Channel != models.DeliveryWebhook || store.query.Offset != 20 {
		t.Fatalf("filters: %+v %d %s", store.query, w.Code, w.Body)
	}
	for _, query := range []string{"currency=GBP", "status=UNKNOWN", "channel=SMS", "limit=0"} {
		if w := doWithKey(r, http.MethodGet, "/api/v1/notifications?"+query, "", token, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %s: %d", query, w.Code)
		}
	}
	const path = "/api/v1/notifications/22222222-2222-4222-8222-222222222222"
	if w := doWithKey(r, http.MethodGet, path, "", token, ""); w.Code != http.StatusNotFound || errorCode(t, w) != "delivery_not_found" {
		t.Fatalf("not found: %d %s", w.Code, w.Body)
	}
	if w := doWithKey(r, http.MethodPost, path+"/retry", `{}`, token, ""); w.Code != http.StatusBadRequest || store.retried {
		t.Fatal("retry without current attempt count accepted")
	}
	if w := doWithKey(r, http.MethodPost, path+"/retry", `{"attempts":1}`, token, ""); w.Code != http.StatusOK || !store.retried {
		t.Fatalf("retry: %d %s", w.Code, w.Body)
	}
	store.stale = true
	if w := doWithKey(r, http.MethodPost, path+"/retry", `{"attempts":1}`, token, ""); w.Code != http.StatusConflict || errorCode(t, w) != "delivery_stale" {
		t.Fatalf("stale retry: %d %s", w.Code, w.Body)
	}
}
