package controllers

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/audit"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/repositories"
)

// AuditLog pages the audit trail (implemented by repositories.AuditRepository;
// a read-only query with no business rules, so no service sits in between).
type AuditLog interface {
	List(ctx context.Context, q repositories.AuditQuery) ([]audit.Event, int, error)
}

// AuditController serves the audit log.
type AuditController struct {
	log AuditLog
}

// NewAuditController: nil dependencies make the route answer 503 audit_unavailable.
func NewAuditController(log AuditLog) *AuditController {
	return &AuditController{log: log}
}

func (h *AuditController) Events(c *gin.Context) {
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
	events, total, err := h.log.List(c.Request.Context(), repositories.AuditQuery{Action: action, Search: search, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	responses.Page(c, events, total, limit, offset)
}

// Guard returns the shared authentication and permission middleware provider.
