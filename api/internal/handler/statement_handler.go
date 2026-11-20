package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

type Statements interface {
	Preview(context.Context, string, model.StatementRequest) (model.StatementPreview, error)
	Import(context.Context, string, model.StatementRequest) (model.StatementImport, error)
	List(context.Context, string, int, int) ([]model.StatementImport, error)
	Detail(context.Context, string, string, int, int) (model.StatementDetail, error)
}
type StatementHandler struct {
	svc   Statements
	guard *Guard
}

func NewStatementHandler(svc Statements, guard *Guard) *StatementHandler {
	return &StatementHandler{svc, guard}
}
func (h *StatementHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/reconciliation", middleware.ErrorFallback(apperr.Unavailable("reconciliation_unavailable", "Statement reconciliation is unavailable. Try again shortly.")), h.ready)
	g.POST("/preview", h.guard.RequireAll(model.PermReconciliationImport, model.PermAttachmentsRead, model.PermLedgerRead), h.Preview)
	g.POST("/imports", h.guard.RequireAll(model.PermReconciliationImport, model.PermAttachmentsRead, model.PermLedgerRead), h.Import)
	g.GET("/imports", h.guard.RequireAll(model.PermReconciliationRead, model.PermLedgerRead), h.List)
	g.GET("/imports/:id", h.guard.RequireAll(model.PermReconciliationRead, model.PermLedgerRead), h.Detail)
}
func (h *StatementHandler) ready(c *gin.Context) {
	if h.svc == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func statementInput(c *gin.Context) (model.StatementRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.StatementRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_statement_request", "Supply attachment_id, account_id and, when importing, expected_hash from preview."))
		return req, false
	}
	return req, true
}
func (h *StatementHandler) Preview(c *gin.Context) {
	req, ok := statementInput(c)
	if !ok {
		return
	}
	v, err := h.svc.Preview(c.Request.Context(), sessionUser(c).ID, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, v)
}
func (h *StatementHandler) Import(c *gin.Context) {
	req, ok := statementInput(c)
	if !ok {
		return
	}
	v, err := h.svc.Import(c.Request.Context(), sessionUser(c).ID, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, v)
}
func (h *StatementHandler) List(c *gin.Context) {
	limit, offset, ok := page(c, 20, 100)
	if !ok {
		return
	}
	v, err := h.svc.List(c.Request.Context(), sessionUser(c).ID, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, v)
}
func (h *StatementHandler) Detail(c *gin.Context) {
	if !validUUID(c.Param("id")) {
		fail(c, model.ErrStatementNotFound)
		return
	}
	limit, offset, ok := page(c, 50, 100)
	if !ok {
		return
	}
	v, err := h.svc.Detail(c.Request.Context(), sessionUser(c).ID, c.Param("id"), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, v)
}
