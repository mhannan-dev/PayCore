package handler

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/audit"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/pkg/response"
)

// AuditLog pages the audit trail (implemented by repository.AuditRepository;
// a read-only query with no business rules, so no service sits in between).
type AuditLog interface {
	List(ctx context.Context, q repository.AuditQuery) ([]audit.Event, int, error)
}

// AuditHandler serves the audit log.
type AuditHandler struct {
	log   AuditLog
	guard *Guard
}

// NewAuditHandler: nil dependencies make the route answer 503 audit_unavailable.
func NewAuditHandler(log AuditLog, guard *Guard) *AuditHandler {
	return &AuditHandler{log: log, guard: guard}
}

// Register adds GET /audit/events?action=&q=&limit=50&offset=0 (audit:read, newest first).
func (h *AuditHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/audit", middleware.ErrorFallback(apperr.Unavailable("audit_unavailable", "The audit log is not available right now.")))
	g.GET("/events", h.guard.Require(model.PermAuditRead), h.Events)
}

func (h *AuditHandler) Events(c *gin.Context) {
	if h.log == nil {
		fail(c, errNotConfigured)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 200 {
		fail(c, apperr.BadRequest("invalid_limit", "limit must be a whole number from 1 to 200."))
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 1000000 {
		fail(c, apperr.BadRequest("invalid_offset", "offset must be a whole number from 0 to 1000000."))
		return
	}
	action := audit.Action(strings.TrimSpace(c.Query("action")))
	if action != "" && !action.Valid() {
		fail(c, apperr.BadRequest("invalid_action", "action is not a known audit action."))
		return
	}
	search := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(search) > 100 {
		fail(c, apperr.BadRequest("invalid_query", "q must be at most 100 characters."))
		return
	}
	events, total, err := h.log.List(c.Request.Context(), repository.AuditQuery{Action: action, Search: search, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, events, total, limit, offset)
}
