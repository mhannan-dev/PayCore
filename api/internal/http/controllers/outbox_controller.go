package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/requests"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

// OutboxService specifies the relay capabilities exposed to HTTP controllers.
type OutboxService interface {
	Metrics(ctx context.Context) (models.OutboxMetrics, error)
	Replay(ctx context.Context, ids ...string) (models.OutboxReplayResult, error)
}

// OutboxController serves outbox health metrics and manual replay controls.
type OutboxController struct {
	service OutboxService
}

func NewOutboxController(service OutboxService) *OutboxController {
	return &OutboxController{service: service}
}

func (h *OutboxController) Ready(c *gin.Context) {
	if h.service == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}

func (h *OutboxController) Metrics(c *gin.Context) {
	if h.service == nil {
		fail(c, errNotConfigured)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	metrics, err := h.service.Metrics(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, metrics)
}

func (h *OutboxController) Replay(c *gin.Context) {
	if h.service == nil {
		fail(c, errNotConfigured)
		return
	}
	var req models.OutboxReplayRequest
	if c.Request.ContentLength > 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
		if err := requests.BindJSONMaxBytes(c, &req, 16384); err != nil {
			fail(c, apperr.BadRequest("invalid_outbox_replay", "Send up to 200 valid event IDs as JSON."))
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	res, err := h.service.Replay(ctx, req.IDs...)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, res)
}
