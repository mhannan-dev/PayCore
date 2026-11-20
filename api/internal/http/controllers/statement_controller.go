package controllers

import (
	"context"
	"net/http"
	"paycore/api/internal/http/requests"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

type Statements interface {
	Preview(context.Context, string, models.StatementRequest) (models.StatementPreview, error)
	Import(context.Context, string, models.StatementRequest) (models.StatementImport, error)
	List(context.Context, string, int, int) ([]models.StatementImport, error)
	Detail(context.Context, string, string, int, int) (models.StatementDetail, error)
}
type StatementController struct {
	svc Statements
}

func NewStatementController(svc Statements) *StatementController {
	return &StatementController{svc}
}

func (h *StatementController) Ready(c *gin.Context) {
	if h.svc == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func statementInput(c *gin.Context) (models.StatementRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req models.StatementRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_statement_request", "Supply attachment_id, account_id and, when importing, expected_hash from preview."))
		return req, false
	}
	return req, true
}
func (h *StatementController) Preview(c *gin.Context) {
	req, ok := statementInput(c)
	if !ok {
		return
	}
	v, err := h.svc.Preview(c.Request.Context(), sessionUser(c).ID, req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, v)
}
func (h *StatementController) Import(c *gin.Context) {
	req, ok := statementInput(c)
	if !ok {
		return
	}
	v, err := h.svc.Import(c.Request.Context(), sessionUser(c).ID, req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.Created(c, v)
}
func (h *StatementController) List(c *gin.Context) {
	limit, offset, ok := page(c, 20, 100)
	if !ok {
		return
	}
	v, err := h.svc.List(c.Request.Context(), sessionUser(c).ID, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, v)
}
func (h *StatementController) Detail(c *gin.Context) {
	if !validUUID(c.Param("id")) {
		fail(c, models.ErrStatementNotFound)
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
	responses.OK(c, v)
}

// Guard returns the shared authentication and permission middleware provider.
