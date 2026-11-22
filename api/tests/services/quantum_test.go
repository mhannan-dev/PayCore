package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

const quantumPayer = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const quantumReceiver = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const quantumToken = "0xcccccccccccccccccccccccccccccccccccccccc"

func quantumProof() (models.QuantumWork, models.QuantumReceipt, time.Time) {
	now := time.Now().UTC()
	hash := "0x1111111111111111111111111111111111111111111111111111111111111111"
	work := models.QuantumWork{Invoice: models.QuantumInvoiceRecord{Invoice: models.QuantumInvoice{ChainID: 12345, TokenAddress: quantumToken, ReceiverAddress: quantumReceiver, AmountBaseUnits: 1250000, ExpiresAt: now.Add(time.Minute), Status: models.QuantumPending}}, Attempt: models.QuantumAttempt{PayerAddress: quantumPayer, Status: models.QuantumBroadcast, CreatedAt: now.Add(-time.Minute), TxHash: &hash}}
	receipt := models.QuantumReceipt{Found: true, Canonical: true, ChainID: 12345, Success: true, TxSender: quantumPayer, TxRecipient: quantumToken, BlockNumber: 20, BlockHash: hash, BlockTime: now, Confirmations: 2, Transfers: []models.QuantumTransfer{{Token: quantumToken, From: quantumPayer, To: quantumReceiver, Amount: 1250000}}}
	return work, receipt, now
}
func TestQuantumVerifierDecisions(t *testing.T) {
	cases := []struct {
		name    string
		alter   func(*models.QuantumWork, *models.QuantumReceipt)
		failure models.QuantumFailure
		status  models.QuantumAttemptStatus
	}{
		{"chain", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.ChainID = 1 }, models.QuantumWrongChain, models.QuantumFailed},
		{"failed receipt", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Success = false }, models.QuantumTxFailed, models.QuantumFailed},
		{"token", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].Token = quantumPayer }, models.QuantumWrongToken, models.QuantumFailed},
		{"recipient", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].To = quantumPayer }, models.QuantumWrongRecipient, models.QuantumFailed},
		{"payer", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].From = quantumReceiver }, models.QuantumWrongPayer, models.QuantumReview},
		{"transaction signer", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.TxSender = quantumReceiver }, models.QuantumWrongPayer, models.QuantumReview},
		{"underpayment", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].Amount-- }, models.QuantumUnderpaid, models.QuantumReview},
		{"overpayment", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].Amount++ }, models.QuantumOverpaid, models.QuantumReview},
		{"overflow", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].Overflow = true }, models.QuantumOverpaid, models.QuantumReview},
		{"late", func(w *models.QuantumWork, r *models.QuantumReceipt) {
			r.BlockTime = w.Invoice.Invoice.ExpiresAt.Add(time.Second)
		}, models.QuantumLate, models.QuantumReview},
		{"old transfer", func(w *models.QuantumWork, r *models.QuantumReceipt) {
			r.BlockTime = w.Attempt.CreatedAt.Add(-time.Second)
		}, models.QuantumLate, models.QuantumReview},
		{"ambiguous logs", func(_ *models.QuantumWork, r *models.QuantumReceipt) {
			r.Transfers = append(r.Transfers, r.Transfers[0])
		}, models.QuantumDuplicateTransfer, models.QuantumReview},
		{"missing transaction", func(w *models.QuantumWork, r *models.QuantumReceipt) {
			r.Found = false
			w.Attempt.CreatedAt = w.Attempt.CreatedAt.Add(-time.Hour)
		}, models.QuantumTxNotFound, models.QuantumFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, r, now := quantumProof()
			tc.alter(&w, &r)
			d := services.EvaluateQuantumReceipt(w, r, 2, now)
			if d.Status != tc.status || d.Failure == nil || *d.Failure != tc.failure {
				t.Fatalf("decision: %+v", d)
			}
		})
	}
	w, r, now := quantumProof()
	r.Confirmations = 1
	if d := services.EvaluateQuantumReceipt(w, r, 2, now); d.Status != models.QuantumVerifying || d.Transfer != nil {
		t.Fatal("premature confirmation")
	}
	r.Confirmations = 2
	if d := services.EvaluateQuantumReceipt(w, r, 2, now); d.Status != models.QuantumConfirmed || d.Transfer == nil {
		t.Fatalf("exact transfer rejected: %+v", d)
	}
	w.Attempt.Status = models.QuantumConfirmed
	w.ObservedHash = &r.BlockHash
	r.Canonical = false
	if d := services.EvaluateQuantumReceipt(w, r, 2, now); !d.Reorg || d.Status != models.QuantumReorged {
		t.Fatal("reorg not detected")
	}
}
func TestQuantumTokenEntropy(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		token, err := services.NewQuantumToken()
		if err != nil || len(token) != 43 || seen[token] {
			t.Fatal("invalid or repeated checkout token")
		}
		seen[token] = true
	}
}

type quantumWorkerStore struct {
	jobs    []models.QuantumWork
	applied []models.QuantumDecision
	retried int
}

func (s *quantumWorkerStore) Work(context.Context) ([]models.QuantumWork, error) { return s.jobs, nil }
func (s *quantumWorkerStore) Apply(_ context.Context, _ models.QuantumWork, d models.QuantumDecision) error {
	s.applied = append(s.applied, d)
	return nil
}
func (s *quantumWorkerStore) RetryWork(context.Context, models.QuantumWork, time.Duration) error {
	s.retried++
	return nil
}

type quantumFakeVerifier struct {
	receipt models.QuantumReceipt
	err     error
}

func (f quantumFakeVerifier) Verify(context.Context, string) (models.QuantumReceipt, error) {
	return f.receipt, f.err
}
func TestQuantumWorkerRPCOutage(t *testing.T) {
	w, _, _ := quantumProof()
	store := &quantumWorkerStore{jobs: []models.QuantumWork{w}}
	worker := services.NewQuantumVerifierWorker(store, quantumFakeVerifier{err: errors.New("offline")}, 2)
	if err := worker.ProcessBatch(context.Background()); err != nil || store.retried != 1 || len(store.applied) != 0 {
		t.Fatal("RPC outage changed invoice state")
	}
}

// quantumInvoiceFake records the actor and request fingerprint at the service boundary.
type quantumInvoiceFake struct {
	services.QuantumStore
	owner string
	hash  string
	body  []byte
}

func (f *quantumInvoiceFake) CreateInvoice(_ context.Context, actor, key, hash, token, base string, cfg models.QuantumConfig, req models.QuantumInvoiceRequest) (models.IdempotentResult, error) {
	if f.hash != "" {
		if f.owner != actor || f.hash != hash {
			return models.IdempotentResult{}, models.ErrQuantumIdempotency
		}
		return models.IdempotentResult{Body: f.body, Status: 201, Replayed: true}, nil
	}
	f.owner = actor
	f.hash = hash
	f.body = []byte(`{"status":"PENDING"}`)
	return models.IdempotentResult{Body: f.body, Status: 201}, nil
}
func (f *quantumInvoiceFake) Invoice(_ context.Context, actor, id string) (models.QuantumInvoice, error) {
	if actor != f.owner {
		return models.QuantumInvoice{}, models.ErrQuantumInvoiceNotFound
	}
	return models.QuantumInvoice{ID: id, Status: models.QuantumPending}, nil
}
func TestQuantumServiceIdentityAndReplay(t *testing.T) {
	store := &quantumInvoiceFake{}
	svc := services.NewQuantumService(store, models.QuantumConfig{Enabled: true}, "https://checkout.example.test")
	ctx := context.Background()
	actor := models.UserDTO{ID: "owner"}
	req := models.QuantumInvoiceRequest{Description: "Invoice", AmountBaseUnits: 1, ExpiresInMinutes: 30}
	first, err := svc.CreateInvoice(ctx, actor, "invoice-key", req)
	if err != nil || first.Replayed {
		t.Fatal(err)
	}
	replay, err := svc.CreateInvoice(ctx, actor, "invoice-key", req)
	if err != nil || !replay.Replayed {
		t.Fatal("same intent not replayed")
	}
	req.AmountBaseUnits = 2
	if _, err := svc.CreateInvoice(ctx, actor, "invoice-key", req); !errors.Is(err, models.ErrQuantumIdempotency) {
		t.Fatal("changed intent replayed")
	}
	if _, err := svc.Invoice(ctx, models.UserDTO{ID: "other"}, "11111111-1111-4111-8111-111111111111"); !errors.Is(err, models.ErrQuantumInvoiceNotFound) {
		t.Fatal("caller identity was not passed to ownership boundary")
	}
}
