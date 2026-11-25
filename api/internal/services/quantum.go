package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"paycore/api/internal/models"
)

type QuantumStore interface {
	CreateMerchant(context.Context, string, models.QuantumMerchantRequest) (models.QuantumMerchant, error)
	Merchant(context.Context, string) (models.QuantumMerchant, error)
	CreateInvoice(context.Context, string, string, string, string, string, models.QuantumConfig, models.QuantumInvoiceRequest) (models.IdempotentResult, error)
	Invoice(context.Context, string, string) (models.QuantumInvoice, error)
	Invoices(context.Context, string, models.QuantumQuery) ([]models.QuantumInvoice, int, error)
	Cancel(context.Context, string, string) (models.QuantumInvoice, error)
	Checkout(context.Context, string) (models.QuantumInvoiceRecord, error)
	CheckoutStatus(context.Context, string) (models.QuantumCheckoutStatus, error)
	RegisterAttempt(context.Context, string, string) (models.QuantumAttempt, error)
	Submit(context.Context, string, string, string) (models.QuantumAttempt, error)
}
type QuantumService struct {
	store   QuantumStore
	config  models.QuantumConfig
	baseURL string
}

func NewQuantumService(store QuantumStore, cfg models.QuantumConfig, baseURL string) *QuantumService {
	return &QuantumService{store: store, config: cfg, baseURL: baseURL}
}
func (s *QuantumService) Config() models.QuantumConfig { return s.config }
func (s *QuantumService) Merchant(ctx context.Context, actor models.UserDTO) (models.QuantumMerchant, error) {
	return s.store.Merchant(ctx, actor.ID)
}
func (s *QuantumService) CreateMerchant(ctx context.Context, actor models.UserDTO, req models.QuantumMerchantRequest) (models.QuantumMerchant, error) {
	if err := req.Normalize(); err != nil {
		return models.QuantumMerchant{}, err
	}
	return s.store.CreateMerchant(ctx, actor.ID, req)
}
func NewQuantumToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}
func validCheckoutToken(token string) bool {
	data, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(data) == 32
}
func (s *QuantumService) CreateInvoice(ctx context.Context, actor models.UserDTO, key string, req models.QuantumInvoiceRequest) (models.IdempotentResult, error) {
	if err := req.Normalize(); err != nil {
		return models.IdempotentResult{}, err
	}
	if len(key) < 8 || len(key) > 255 {
		return models.IdempotentResult{}, models.InvalidRequestError{Code: "idempotency_key_required", Reason: "Supply an Idempotency-Key of 8–255 characters."}
	}
	hash, err := requestHash("POST", "/api/v1/quantum/invoices", req)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	token, err := NewQuantumToken()
	if err != nil {
		return models.IdempotentResult{}, err
	}
	return s.store.CreateInvoice(ctx, actor.ID, key, hash, token, s.baseURL, s.config, req)
}
func (s *QuantumService) Invoice(ctx context.Context, actor models.UserDTO, id string) (models.QuantumInvoice, error) {
	if !models.ValidUUID(id) {
		return models.QuantumInvoice{}, models.ErrQuantumInvoiceNotFound
	}
	return s.store.Invoice(ctx, actor.ID, id)
}
func (s *QuantumService) Invoices(ctx context.Context, actor models.UserDTO, q models.QuantumQuery) ([]models.QuantumInvoice, int, error) {
	if q.Status != "" && !q.Status.Valid() || q.Limit < 1 || q.Limit > 200 || q.Offset < 0 || q.Offset > 1000000 {
		return nil, 0, models.InvalidRequestError{Code: "invalid_invoice_query", Reason: "Use a supported status and valid pagination."}
	}
	return s.store.Invoices(ctx, actor.ID, q)
}
func (s *QuantumService) Cancel(ctx context.Context, actor models.UserDTO, id string) (models.QuantumInvoice, error) {
	if !models.ValidUUID(id) {
		return models.QuantumInvoice{}, models.ErrQuantumInvoiceNotFound
	}
	return s.store.Cancel(ctx, actor.ID, id)
}
func (s *QuantumService) Checkout(ctx context.Context, token string) (models.QuantumCheckout, error) {
	if !validCheckoutToken(token) {
		return models.QuantumCheckout{}, models.ErrQuantumCheckoutNotFound
	}
	r, err := s.store.Checkout(ctx, token)
	if err != nil {
		return models.QuantumCheckout{}, err
	}
	i := r.Invoice
	return models.QuantumCheckout{OrderID: i.OrderID, PaymentContractAddress: i.PaymentContractAddress, MerchantName: r.MerchantName, Description: i.Description, AmountBaseUnits: i.AmountBaseUnits, ChainID: i.ChainID, TokenAddress: i.TokenAddress, TokenSymbol: s.config.TokenSymbol, TokenDecimals: r.TokenDecimals, ReceiverAddress: i.ReceiverAddress, Status: i.Status, ExpiresAt: i.ExpiresAt, ExplorerURL: r.ExplorerURL}, nil
}
func (s *QuantumService) CheckoutStatus(ctx context.Context, token string) (models.QuantumCheckoutStatus, error) {
	if !validCheckoutToken(token) {
		return models.QuantumCheckoutStatus{}, models.ErrQuantumCheckoutNotFound
	}
	return s.store.CheckoutStatus(ctx, token)
}
func (s *QuantumService) RegisterAttempt(ctx context.Context, token string, req models.QuantumAttemptRequest) (models.QuantumAttempt, error) {
	if !validCheckoutToken(token) {
		return models.QuantumAttempt{}, models.ErrQuantumCheckoutNotFound
	}
	req.PayerAddress = strings.ToLower(strings.TrimSpace(req.PayerAddress))
	if !models.ValidQuantumAddress(req.PayerAddress) {
		return models.QuantumAttempt{}, models.InvalidRequestError{Code: "invalid_payer", Reason: "Supply a nonzero EVM payer address."}
	}
	return s.store.RegisterAttempt(ctx, token, req.PayerAddress)
}
func (s *QuantumService) Submit(ctx context.Context, token, id string, req models.QuantumTransactionRequest) (models.QuantumAttempt, error) {
	if !validCheckoutToken(token) || !models.ValidUUID(id) {
		return models.QuantumAttempt{}, models.ErrQuantumCheckoutNotFound
	}
	req.TxHash = strings.ToLower(strings.TrimSpace(req.TxHash))
	if !models.ValidQuantumHash(req.TxHash) {
		return models.QuantumAttempt{}, models.InvalidRequestError{Code: "invalid_tx_hash", Reason: "Supply a 32-byte hexadecimal transaction hash."}
	}
	return s.store.Submit(ctx, token, id, req.TxHash)
}

type TransferVerifier interface {
	Verify(context.Context, string) (models.QuantumReceipt, error)
}
type QuantumVerificationStore interface {
	Work(context.Context) ([]models.QuantumWork, error)
	RetryWork(context.Context, models.QuantumWork, time.Duration) error
	Apply(context.Context, models.QuantumWork, models.QuantumDecision) error
}
type QuantumVerifierWorker struct {
	store    QuantumVerificationStore
	verifier TransferVerifier
	minimum  int64
	clockSkew time.Duration
}

func NewQuantumVerifierWorker(store QuantumVerificationStore, verifier TransferVerifier, minimum int64) *QuantumVerifierWorker {
	return &QuantumVerifierWorker{store: store, verifier: verifier, minimum: minimum, clockSkew: time.Minute}
}

func (w *QuantumVerifierWorker) WithClockSkew(skew time.Duration) *QuantumVerifierWorker {
	if skew >= 0 && skew <= 5*time.Minute { w.clockSkew = skew }
	return w
}
func quantumRejection(code models.QuantumFailure, review bool) models.QuantumDecision {
	status := models.QuantumFailed
	if review {
		status = models.QuantumReview
	}
	return models.QuantumDecision{Status: status, Failure: &code}
}

// EvaluateQuantumReceipt is pure: only canonical, exactly attributed transfers can settle.
func EvaluateQuantumReceipt(w models.QuantumWork, r models.QuantumReceipt, minimum int64, now time.Time, configuredSkew ...time.Duration) models.QuantumDecision {
	skew := time.Minute
	if len(configuredSkew)>0 && configuredSkew[0]>=0 && configuredSkew[0]<=5*time.Minute { skew=configuredSkew[0] }
	if w.Attempt.Status == models.QuantumConfirmed && r.ChainID == w.Invoice.Invoice.ChainID {
		return recheckQuantumConfirmation(w,r,minimum,now,skew)
	}
	decision := evaluateQuantumReceipt(w, r, minimum, now, skew)
	if w.Attempt.Status == models.QuantumConfirmed && r.ChainID == w.Invoice.Invoice.ChainID && (decision.Status == models.QuantumFailed || decision.Status == models.QuantumReview) {
		return models.QuantumDecision{Status: models.QuantumReorged, Reorg: true, Receipt: r, RetryAfter: 3 * time.Second}
	}
	return decision
}
func evaluateQuantumReceipt(w models.QuantumWork, r models.QuantumReceipt, minimum int64, now time.Time, skew time.Duration) models.QuantumDecision {
	i := w.Invoice.Invoice
	if r.ChainID != i.ChainID {
		return quantumRejection(models.QuantumWrongChain, false)
	}
	wait := models.QuantumDecision{Status: models.QuantumVerifying, Receipt: r, RetryAfter: 3 * time.Second}
	if !r.Found || !r.Canonical {
		if now.Sub(w.Attempt.CreatedAt) > 30*time.Minute && w.Attempt.Status != models.QuantumReorged {
			return quantumRejection(models.QuantumTxNotFound, false)
		}
		if w.Attempt.Status == models.QuantumReorged {
			wait.Status = models.QuantumReorged
		}
		return wait
	}
	if !r.Success {
		return quantumRejection(models.QuantumTxFailed, false)
	}
	if i.PaymentContractAddress != nil {
		return evaluateQuantumContractPayment(w, r, minimum, skew)
	}
	candidates := []models.QuantumTransfer{}
	for _, t := range r.Transfers {
		if t.Token == i.TokenAddress {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 || r.TxRecipient != i.TokenAddress {
		return quantumRejection(models.QuantumWrongToken, false)
	}
	recipients := []models.QuantumTransfer{}
	for _, t := range candidates {
		if t.To == i.ReceiverAddress {
			recipients = append(recipients, t)
		}
	}
	if len(recipients) == 0 {
		return quantumRejection(models.QuantumWrongRecipient, false)
	}
	if r.TxSender != w.Attempt.PayerAddress {
		return quantumRejection(models.QuantumWrongPayer, true)
	}
	var match *models.QuantumTransfer
	for _, t := range recipients {
		if t.From == w.Attempt.PayerAddress {
			if match != nil {
				return quantumRejection(models.QuantumDuplicateTransfer, true)
			}
			copy := t
			match = &copy
		}
	}
	if match == nil {
		return quantumRejection(models.QuantumWrongPayer, true)
	}
	if match.Overflow || match.Amount > int64(i.AmountBaseUnits) {
		return quantumRejection(models.QuantumOverpaid, true)
	}
	if match.Amount < int64(i.AmountBaseUnits) {
		return quantumRejection(models.QuantumUnderpaid, true)
	}
	if r.BlockTime.After(i.ExpiresAt) || r.BlockTime.Before(w.Attempt.CreatedAt.Truncate(time.Second).Add(-skew)) {
		return quantumRejection(models.QuantumLate, true)
	}
	if r.Confirmations < minimum {
		return wait
	}
	return models.QuantumDecision{Status: models.QuantumConfirmed, Receipt: r, Transfer: match, RetryAfter: time.Minute}
}
func (w *QuantumVerifierWorker) ProcessBatch(ctx context.Context) error {
	jobs, err := w.store.Work(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Attempt.TxHash == nil {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		receipt, verifyErr := w.verifier.Verify(callCtx, *job.Attempt.TxHash)
		cancel()
		if verifyErr != nil || (job.Attempt.Status == models.QuantumConfirmed && receipt.ChainID != job.Invoice.Invoice.ChainID) {
			if job.Attempt.Status == models.QuantumConfirmed {
				missing := models.QuantumReceipt{ChainID:job.Invoice.Invoice.ChainID}
				d := EvaluateQuantumReceipt(job,missing,w.minimum,time.Now().UTC(),w.clockSkew)
				if err:=w.store.Apply(ctx,job,d);err!=nil && !errors.Is(err,models.ErrQuantumStale) { return err }
				continue
			}
			delay := 3 * time.Second
			for n := 0; n < job.Failures && delay < time.Minute; n++ {
				delay *= 2
			}
			if delay > time.Minute {
				delay = time.Minute
			}
			if err := w.store.RetryWork(ctx, job, delay); err != nil && !errors.Is(err, models.ErrQuantumStale) {
				return err
			}
			continue
		}
		decision := EvaluateQuantumReceipt(job, receipt, w.minimum, time.Now().UTC(),w.clockSkew)
		if err := w.store.Apply(ctx, job, decision); err != nil && !errors.Is(err, models.ErrQuantumStale) && !errors.Is(err, models.ErrQuantumDuplicate) {
			return err
		}
	}
	return nil
}
func (w *QuantumVerifierWorker) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if err := w.ProcessBatch(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("quantum_verification_batch_failed", "error_type", "verification_store")
		}
		timer.Reset(3 * time.Second)
	}
}
