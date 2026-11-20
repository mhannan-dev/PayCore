package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

type Matching interface {
	Report(context.Context, string, string, int, int) (model.MatchingReport, error)
	Configure(context.Context, string, string, model.MatchingConfigRequest) error
	Review(context.Context, string, string, int, model.MatchingReviewRequest) error
}
type MatchingHandler struct {
	svc   Matching
	guard *Guard
}

func NewMatchingHandler(svc Matching, guard *Guard) *MatchingHandler {
	return &MatchingHandler{svc, guard}
}
func (h *MatchingHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/reconciliation/imports/:id", middleware.ErrorFallback(apperr.Unavailable("matching_unavailable", "Statement matching is unavailable.")), h.ready)
	g.GET("/matches", h.guard.RequireAll(model.PermReconciliationRead, model.PermLedgerRead), h.Report)
	g.POST("/matching", h.guard.RequireAll(model.PermReconciliationMatch, model.PermReconciliationRead, model.PermLedgerRead), h.Configure)
	g.POST("/rows/:row/match", h.guard.RequireAll(model.PermReconciliationMatch, model.PermReconciliationRead, model.PermLedgerRead), h.Review)
}
func (h *MatchingHandler) ready(c *gin.Context) {
	if h.svc == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func matchingID(c *gin.Context) bool {
	if !validUUID(c.Param("id")) {
		fail(c, model.ErrStatementNotFound)
		return false
	}
	return true
}
func (h *MatchingHandler) Report(c *gin.Context) {
	if !matchingID(c) {
		return
	}
	limit, offset, ok := page(c, 50, 100)
	if !ok {
		return
	}
	report, err := h.svc.Report(c.Request.Context(), sessionUser(c).ID, c.Param("id"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, report)
}
func (h *MatchingHandler) Configure(c *gin.Context) {
	if !matchingID(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.MatchingConfigRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, model.ErrMatchInvalid)
		return
	}
	if err := h.svc.Configure(c.Request.Context(), sessionUser(c).ID, c.Param("id"), req); err != nil {
		fail(c, err)
		return
	}
	response.NoContent(c)
}
func (h *MatchingHandler) Review(c *gin.Context) {
	if !matchingID(c) {
		return
	}
	number, err := strconv.Atoi(c.Param("row"))
	if err != nil || number < 1 {
		fail(c, model.ErrMatchInvalid)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.MatchingReviewRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, model.ErrMatchInvalid)
		return
	}
	if err := h.svc.Review(c.Request.Context(), sessionUser(c).ID, c.Param("id"), number, req); err != nil {
		fail(c, err)
		return
	}
	response.NoContent(c)
}
