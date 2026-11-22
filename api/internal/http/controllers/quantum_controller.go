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

type Quantum interface {
	Config() models.QuantumConfig
	Merchant(context.Context, models.UserDTO) (models.QuantumMerchant, error)
	CreateMerchant(context.Context, models.UserDTO, models.QuantumMerchantRequest) (models.QuantumMerchant, error)
	CreateInvoice(context.Context, models.UserDTO, string, models.QuantumInvoiceRequest) (models.IdempotentResult, error)
	Invoice(context.Context, models.UserDTO, string) (models.QuantumInvoice, error)
	Invoices(context.Context, models.UserDTO, models.QuantumQuery) ([]models.QuantumInvoice, int, error)
	Cancel(context.Context, models.UserDTO, string) (models.QuantumInvoice, error)
	Checkout(context.Context, string) (models.QuantumCheckout, error)
	CheckoutStatus(context.Context, string) (models.QuantumCheckoutStatus, error)
	RegisterAttempt(context.Context, string, models.QuantumAttemptRequest) (models.QuantumAttempt, error)
	Submit(context.Context, string, string, models.QuantumTransactionRequest) (models.QuantumAttempt, error)
}
type QuantumController struct{ service Quantum }

func NewQuantumController(service Quantum) *QuantumController {
	return &QuantumController{service: service}
}
func (h *QuantumController) Config(c *gin.Context) {
	cfg := models.QuantumConfig{}
	if h.service != nil {
		cfg = h.service.Config()
	}
	responses.OK(c, cfg)
}
func (h *QuantumController) Ready(c *gin.Context) {
	if h.service == nil || !h.service.Config().Enabled {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func (h *QuantumController) Merchant(c *gin.Context) {
	result, err := h.service.Merchant(c.Request.Context(), sessionUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *QuantumController) CreateMerchant(c *gin.Context) {
	var req models.QuantumMerchantRequest
	if !bindQuantum(c, &req) {
		return
	}
	result, err := h.service.CreateMerchant(c.Request.Context(), sessionUser(c), req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.Created(c, result)
}
func (h *QuantumController) CreateInvoice(c *gin.Context) {
	key, ok := requireKey(c)
	if !ok {
		return
	}
	var req models.QuantumInvoiceRequest
	if !bindQuantum(c, &req) {
		return
	}
	result, err := h.service.CreateInvoice(c.Request.Context(), sessionUser(c), key, req)
	if err != nil {
		fail(c, err)
		return
	}
	writeIdempotent(c, result)
}
func (h *QuantumController) Invoices(c *gin.Context) {
	limit, offset, ok := page(c, 25, 200)
	if !ok {
		return
	}
	q := models.QuantumQuery{Status: models.QuantumInvoiceStatus(strings.ToUpper(strings.TrimSpace(c.Query("status")))), Limit: limit, Offset: offset}
	items, total, err := h.service.Invoices(c.Request.Context(), sessionUser(c), q)
	if err != nil {
		fail(c, err)
		return
	}
	responses.Page(c, items, total, limit, offset)
}
func (h *QuantumController) Invoice(c *gin.Context) {
	result, err := h.service.Invoice(c.Request.Context(), sessionUser(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *QuantumController) Cancel(c *gin.Context) {
	result, err := h.service.Cancel(c.Request.Context(), sessionUser(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *QuantumController) Checkout(c *gin.Context) {
	result, err := h.service.Checkout(c.Request.Context(), c.Param("token"))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *QuantumController) Status(c *gin.Context) {
	result, err := h.service.CheckoutStatus(c.Request.Context(), c.Param("token"))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *QuantumController) Attempt(c *gin.Context) {
	var req models.QuantumAttemptRequest
	if !bindQuantum(c, &req) {
		return
	}
	result, err := h.service.RegisterAttempt(c.Request.Context(), c.Param("token"), req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.Created(c, result)
}
func (h *QuantumController) Submit(c *gin.Context) {
	var req models.QuantumTransactionRequest
	if !bindQuantum(c, &req) {
		return
	}
	result, err := h.service.Submit(c.Request.Context(), c.Param("token"), c.Param("id"), req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.JSON(c, http.StatusAccepted, result)
}
func bindQuantum[T any](c *gin.Context, req *T) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := requests.BindJSON(c, req); err != nil {
		fail(c, apperr.BadRequest("invalid_quantum_request", "Supply a valid JSON request with the documented fields."))
		return false
	}
	return true
}
