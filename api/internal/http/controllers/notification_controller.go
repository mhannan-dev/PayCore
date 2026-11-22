package controllers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/requests"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

type Notifications interface {
	List(context.Context, models.DeliveryQuery) ([]models.NotificationDelivery, int, error)
	Detail(context.Context, string) (models.DeliveryDetail, error)
	Metrics(context.Context, models.Currency) (models.DeliveryMetrics, error)
	Retry(context.Context, string, models.RetryDeliveryRequest) (models.RetryDeliveryResult, error)
}
type NotificationController struct{ service Notifications }

func NewNotificationController(service Notifications) *NotificationController {
	return &NotificationController{service: service}
}
func (h *NotificationController) Ready(c *gin.Context) {
	if h.service == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func (h *NotificationController) List(c *gin.Context) {
	limit, offset, ok := page(c, 25, 200)
	if !ok {
		return
	}
	q := models.DeliveryQuery{Currency: models.Currency(strings.ToUpper(strings.TrimSpace(c.Query("currency")))), Status: models.DeliveryStatus(strings.ToUpper(strings.TrimSpace(c.Query("status")))), Channel: models.DeliveryChannel(strings.ToUpper(strings.TrimSpace(c.Query("channel")))), Limit: limit, Offset: offset}
	items, total, err := h.service.List(c.Request.Context(), q)
	if err != nil {
		fail(c, err)
		return
	}
	responses.Page(c, items, total, limit, offset)
}
func (h *NotificationController) Detail(c *gin.Context) {
	result, err := h.service.Detail(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *NotificationController) Metrics(c *gin.Context) {
	result, err := h.service.Metrics(c.Request.Context(), models.Currency(strings.ToUpper(strings.TrimSpace(c.Query("currency")))))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *NotificationController) Retry(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req models.RetryDeliveryRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_delivery_retry", "Supply the current attempt count as JSON."))
		return
	}
	result, err := h.service.Retry(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
