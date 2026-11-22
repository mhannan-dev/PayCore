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

type Settlements interface {
	Reserve(context.Context, models.UserDTO, string, models.CreatePayoutRequest) (models.IdempotentResult, error)
	Capture(context.Context, models.UserDTO, string, string) (models.IdempotentResult, error)
	Release(context.Context, models.UserDTO, string, string) (models.IdempotentResult, error)
	List(context.Context, models.SettlementQuery) ([]models.Settlement, int, error)
	Get(context.Context, string) (models.Settlement, error)
}
type SettlementController struct{ settlements Settlements }

func NewSettlementController(settlements Settlements) *SettlementController {
	return &SettlementController{settlements: settlements}
}
func (h *SettlementController) Ready(c *gin.Context) {
	if h.settlements == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func (h *SettlementController) List(c *gin.Context) {
	limit, offset, ok := page(c, 25, 200)
	if !ok {
		return
	}
	currency := models.Currency(strings.ToUpper(strings.TrimSpace(c.Query("currency"))))
	if currency != "" && !currency.Valid() {
		fail(c, apperr.BadRequest("invalid_currency", "Currency must be JPY, USD or EUR."))
		return
	}
	items, total, err := h.settlements.List(c.Request.Context(), models.SettlementQuery{Currency: currency, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	responses.Page(c, items, total, limit, offset)
}
func (h *SettlementController) Detail(c *gin.Context) {
	item, err := h.settlements.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, item)
}
func (h *SettlementController) Create(c *gin.Context) {
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	key, ok := requireKey(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req models.CreatePayoutRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_payout", "Send source_account_id, amount, currency and beneficiary name, bank and account as JSON."))
		return
	}
	result, err := h.settlements.Reserve(c.Request.Context(), actor, key, req)
	if err != nil {
		fail(c, err)
		return
	}
	writeIdempotent(c, result)
}
func (h *SettlementController) Capture(c *gin.Context) { h.transition(c, h.settlements.Capture) }
func (h *SettlementController) Release(c *gin.Context) { h.transition(c, h.settlements.Release) }

// transition handles capture and release, which share the same transport shape.
func (h *SettlementController) transition(c *gin.Context,
	action func(context.Context, models.UserDTO, string, string) (models.IdempotentResult, error)) {
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	key, ok := requireKey(c)
	if !ok {
		return
	}
	result, err := action(c.Request.Context(), actor, key, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	writeIdempotent(c, result)
}
