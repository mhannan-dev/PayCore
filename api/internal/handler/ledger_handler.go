package handler

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

// Ledger is what the ledger endpoints need (implemented by service.LedgerService).
type Ledger interface {
	CreateAccount(ctx context.Context, actor model.UserDTO, req model.CreateAccountRequest) (model.Account, error)
	Account(ctx context.Context, id string) (model.Account, error)
	Accounts(ctx context.Context, q model.AccountQuery) ([]model.Account, int, error)
	AccountPostings(ctx context.Context, accountID string, limit, offset int) ([]model.AccountPosting, int, error)
	Journal(ctx context.Context, id string) (model.Journal, error)
	Journals(ctx context.Context, q model.JournalQuery) ([]model.Journal, int, error)
	ZeroSum(ctx context.Context, currency model.Currency) (model.ZeroSumReport, error)
	Transfer(ctx context.Context, actor model.UserDTO, key string, req model.TransferRequest) (model.IdempotentResult, error)
	Reverse(ctx context.Context, actor model.UserDTO, key, journalID string, req model.ReverseRequest) (model.IdempotentResult, error)
}

// LedgerHandler serves the chart of accounts, transfers, reversals and the zero-sum audit.
type LedgerHandler struct {
	ledger Ledger
	guard  *Guard
}

// NewLedgerHandler: nil dependencies make the routes answer 503 ledger_unavailable.
func NewLedgerHandler(ledger Ledger, guard *Guard) *LedgerHandler {
	return &LedgerHandler{ledger: ledger, guard: guard}
}

// Register adds:
//
//	GET  /accounts                  ledger:read        ?q=&type=&currency=&limit=50&offset=0
//	POST /accounts                  accounts:write
//	GET  /accounts/:id              ledger:read        account with derived balance
//	GET  /accounts/:id/postings     ledger:read        ?limit=50&offset=0
//	GET  /transfers                 transfers:read     ?q=&account_id=&limit=20&offset=0
//	GET  /transfers/:id             transfers:read
//	POST /transfers                 transfers:create   Idempotency-Key required
//	POST /transfers/:id/reverse     transfers:reverse  Idempotency-Key required
//	GET  /audit/zero-sum            audit:verify       ?currency=JPY
func (h *LedgerHandler) Register(api *gin.RouterGroup) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("ledger_unavailable", "The ledger is not available right now. Try again shortly."))
	g := api.Group("", fallback, h.ready)

	g.GET("/accounts", h.guard.Require(model.PermLedgerRead), h.Accounts)
	g.POST("/accounts", h.guard.Require(model.PermAccountsWrite), h.CreateAccount)
	g.GET("/accounts/:id", h.guard.Require(model.PermLedgerRead), h.Account)
	g.GET("/accounts/:id/postings", h.guard.Require(model.PermLedgerRead), h.AccountPostings)

	g.GET("/transfers", h.guard.Require(model.PermTransfersRead), h.Transfers)
	g.GET("/transfers/:id", h.guard.Require(model.PermTransfersRead), h.Transfer)
	g.POST("/transfers", h.guard.Require(model.PermTransfersCreate), h.CreateTransfer)
	g.POST("/transfers/:id/reverse", h.guard.Require(model.PermTransfersReverse), h.Reverse)

	g.GET("/audit/zero-sum", h.guard.Require(model.PermAuditVerify), h.ZeroSum)
}

func (h *LedgerHandler) ready(c *gin.Context) {
	if h.ledger == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}

var (
	errAccountID   = apperr.NotFound("account_not_found", "No ledger account has that ID.")
	errTransferID  = apperr.NotFound("transfer_not_found", "No transfer has that ID.")
	idempotencyKey = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,255}$`)
)

// page parses limit (1..max, default def) and offset (0..1,000,000).
func page(c *gin.Context, def, max int) (int, int, bool) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(def)))
	if err != nil || limit < 1 || limit > max {
		fail(c, apperr.BadRequest("invalid_limit", "limit must be a whole number from 1 to "+strconv.Itoa(max)+"."))
		return 0, 0, false
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 1000000 {
		fail(c, apperr.BadRequest("invalid_offset", "offset must be a whole number from 0 to 1000000."))
		return 0, 0, false
	}
	return limit, offset, true
}

func search(c *gin.Context) (string, bool) {
	q := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(q) > 100 {
		fail(c, apperr.BadRequest("invalid_query", "q must be at most 100 characters."))
		return "", false
	}
	return q, true
}

func (h *LedgerHandler) Accounts(c *gin.Context) {
	limit, offset, ok := page(c, 50, 200)
	if !ok {
		return
	}
	q, ok := search(c)
	if !ok {
		return
	}
	typ := model.AccountType(strings.ToUpper(strings.TrimSpace(c.Query("type"))))
	if typ != "" && !typ.Valid() {
		fail(c, apperr.BadRequest("invalid_type", "type must be ASSET, LIABILITY, EQUITY, REVENUE or EXPENSE."))
		return
	}
	currency := model.Currency(strings.ToUpper(strings.TrimSpace(c.Query("currency"))))
	if currency != "" && !currency.Valid() {
		fail(c, apperr.BadRequest("invalid_currency", "currency must be JPY, USD or EUR."))
		return
	}
	accounts, total, err := h.ledger.Accounts(c.Request.Context(), model.AccountQuery{Search: q, Type: typ, Currency: currency, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, accounts, total, limit, offset)
}

func (h *LedgerHandler) CreateAccount(c *gin.Context) {
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.CreateAccountRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send account_number, name, type, currency and optional overdraft_allowed as JSON."))
		return
	}
	account, err := h.ledger.CreateAccount(c.Request.Context(), actor, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, account)
}

func (h *LedgerHandler) Account(c *gin.Context) {
	id := c.Param("id")
	if !model.ValidUUID(id) {
		fail(c, errAccountID)
		return
	}
	account, err := h.ledger.Account(c.Request.Context(), strings.ToLower(id))
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, account)
}

func (h *LedgerHandler) AccountPostings(c *gin.Context) {
	id := c.Param("id")
	if !model.ValidUUID(id) {
		fail(c, errAccountID)
		return
	}
	limit, offset, ok := page(c, 50, 200)
	if !ok {
		return
	}
	postings, total, err := h.ledger.AccountPostings(c.Request.Context(), strings.ToLower(id), limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, postings, total, limit, offset)
}

func (h *LedgerHandler) Transfers(c *gin.Context) {
	limit, offset, ok := page(c, 20, 100)
	if !ok {
		return
	}
	q, ok := search(c)
	if !ok {
		return
	}
	accountID := strings.ToLower(strings.TrimSpace(c.Query("account_id")))
	if accountID != "" && !model.ValidUUID(accountID) {
		fail(c, apperr.BadRequest("invalid_account_id", "account_id must be a UUID."))
		return
	}
	journals, total, err := h.ledger.Journals(c.Request.Context(), model.JournalQuery{Search: q, AccountID: accountID, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, journals, total, limit, offset)
}

func (h *LedgerHandler) Transfer(c *gin.Context) {
	id := c.Param("id")
	if !model.ValidUUID(id) {
		fail(c, errTransferID)
		return
	}
	journal, err := h.ledger.Journal(c.Request.Context(), strings.ToLower(id))
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, journal)
}

// requireKey returns the Idempotency-Key header or fails with 400.
func requireKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if !idempotencyKey.MatchString(key) {
		fail(c, apperr.BadRequest("idempotency_key_required", "Send an Idempotency-Key header (8–255 letters, digits, '.', '_', ':' or '-'; a UUID is ideal)."))
		return "", false
	}
	return key, true
}

// writeIdempotent sends the stored or fresh response of an idempotent request.
func writeIdempotent(c *gin.Context, r model.IdempotentResult) {
	if r.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.Data(r.Status, "application/json; charset=utf-8", r.Body)
}

func (h *LedgerHandler) CreateTransfer(c *gin.Context) {
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	key, ok := requireKey(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req model.TransferRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_transfer", "Send reference_id, description, currency and legs [{account_id, direction, amount}] as JSON; amounts are strings of digits."))
		return
	}
	result, err := h.ledger.Transfer(c.Request.Context(), actor, key, req)
	if err != nil {
		fail(c, err)
		return
	}
	writeIdempotent(c, result)
}

func (h *LedgerHandler) Reverse(c *gin.Context) {
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !model.ValidUUID(id) {
		fail(c, errTransferID)
		return
	}
	key, ok := requireKey(c)
	if !ok {
		return
	}
	var req model.ReverseRequest
	if c.Request.ContentLength != 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		if err := bindJSON(c, &req); err != nil {
			fail(c, apperr.BadRequest("invalid_reversal", "Send an empty body or {\"description\": \"…\"}."))
			return
		}
	}
	result, err := h.ledger.Reverse(c.Request.Context(), actor, key, strings.ToLower(id), req)
	if err != nil {
		fail(c, err)
		return
	}
	writeIdempotent(c, result)
}

func (h *LedgerHandler) ZeroSum(c *gin.Context) {
	currency := model.Currency(strings.ToUpper(strings.TrimSpace(c.DefaultQuery("currency", "JPY"))))
	report, err := h.ledger.ZeroSum(c.Request.Context(), currency)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, report)
}
