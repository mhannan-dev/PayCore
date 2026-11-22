package repositories_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

const qOwner = "11111111-1111-4111-8111-111111111111"
const qOther = "22222222-2222-4222-8222-222222222222"
const qPayer = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const qReceiver = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const qToken = "0xcccccccccccccccccccccccccccccccccccccccc"
const qTx = "0x1111111111111111111111111111111111111111111111111111111111111111"

type quantumFixture struct {
	pool    *pgxpool.Pool
	repo    *repositories.QuantumRepository
	service *services.QuantumService
	actor   models.UserDTO
	config  models.QuantumConfig
}

func newQuantumFixture(t *testing.T) *quantumFixture {
	t.Helper()
	pool := isolatedPool(t)
	ctx := context.Background()
	for i, id := range []string{qOwner, qOther} {
		if _, err := pool.Exec(ctx, `INSERT INTO users(id,first_name,email,password_hash) VALUES($1,'Quantum tester',$2,'not-a-login-hash')`, id, fmt.Sprintf("quantum%d@example.test", i)); err != nil {
			t.Fatal(err)
		}
	}
	cfg := models.QuantumConfig{Enabled: true, ChainID: 12345, TokenAddress: qToken, TokenSymbol: "tUSD8", TokenDecimals: 6, MinConfirmations: 2, ExplorerURL: "https://explorer.example.test"}
	repo := repositories.NewQuantumRepository(pool)
	svc := services.NewQuantumService(repo, cfg, "https://checkout.example.test")
	for _, id := range []string{qOwner, qOther} {
		if _, err := svc.CreateMerchant(ctx, models.UserDTO{ID: id}, models.QuantumMerchantRequest{DisplayName: "Demo merchant", ReceiverAddress: qReceiver}); err != nil {
			t.Fatal(err)
		}
	}
	return &quantumFixture{pool: pool, repo: repo, service: svc, actor: models.UserDTO{ID: qOwner}, config: cfg}
}
func (f *quantumFixture) invoice(t *testing.T, key string) (models.QuantumInvoice, string) {
	t.Helper()
	result, err := f.service.CreateInvoice(context.Background(), f.actor, key, models.QuantumInvoiceRequest{AmountBaseUnits: 1250000, Description: "Test invoice", ExpiresInMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	var invoice models.QuantumInvoice
	if err := json.Unmarshal(result.Body, &invoice); err != nil {
		t.Fatal(err)
	}
	var token string
	if err := f.pool.QueryRow(context.Background(), "SELECT checkout_token FROM quantum_invoices WHERE id=$1", invoice.ID).Scan(&token); err != nil {
		t.Fatal(err)
	}
	return invoice, token
}
func qProof(w models.QuantumWork) models.QuantumReceipt {
	return models.QuantumReceipt{Found: true, Canonical: true, ChainID: 12345, Success: true, TxSender: qPayer, TxRecipient: qToken, BlockNumber: 20, BlockHash: qTx, BlockTime: time.Now().UTC(), Confirmations: 2, Transfers: []models.QuantumTransfer{{Token: qToken, From: qPayer, To: qReceiver, Amount: int64(w.Invoice.Invoice.AmountBaseUnits), LogIndex: 0}}}
}
func TestQuantumOwnershipIdempotencyAndConstraints(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	request := models.QuantumInvoiceRequest{AmountBaseUnits: 1250000, Description: "Invoice", ExpiresInMinutes: 30}
	first, err := f.service.CreateInvoice(ctx, f.actor, "invoice-key-001", request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.service.CreateInvoice(ctx, f.actor, "invoice-key-001", request)
	if err != nil || !replay.Replayed || !bytes.Equal(first.Body, replay.Body) {
		t.Fatalf("replay: %v", err)
	}
	request.AmountBaseUnits++
	if _, err := f.service.CreateInvoice(ctx, f.actor, "invoice-key-001", request); !errors.Is(err, models.ErrQuantumIdempotency) {
		t.Fatalf("changed key: %v", err)
	}
	var invoice models.QuantumInvoice
	if err := json.Unmarshal(first.Body, &invoice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Invoice(ctx, models.UserDTO{ID: qOther}, invoice.ID); !errors.Is(err, models.ErrQuantumInvoiceNotFound) {
		t.Fatalf("other merchant read: %v", err)
	}
	if _, err := f.service.Cancel(ctx, models.UserDTO{ID: qOther}, invoice.ID); !errors.Is(err, models.ErrQuantumInvoiceNotFound) {
		t.Fatalf("other merchant cancel: %v", err)
	}
	items, total, err := f.service.Invoices(ctx, models.UserDTO{ID: qOther}, models.QuantumQuery{Limit: 25})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatal("merchant invoice list leaked")
	}
	for _, query := range []string{`UPDATE quantum_invoices SET amount_base_units=0 WHERE id=$1`, `UPDATE quantum_invoices SET status='OTHER' WHERE id=$1`, `UPDATE quantum_invoices SET receiver_address='bad' WHERE id=$1`, `UPDATE quantum_invoices SET status='PAID' WHERE id=$1`} {
		if _, err := f.pool.Exec(ctx, query, invoice.ID); err == nil {
			t.Fatalf("constraint accepted %s", query)
		}
	}
	if _, err := f.service.Cancel(ctx, f.actor, invoice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Cancel(ctx, f.actor, invoice.ID); !errors.Is(err, models.ErrQuantumNotPending) {
		t.Fatalf("cancel replay: %v", err)
	}
}
func TestQuantumConfirmationAndReorg(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	invoice, token := f.invoice(t, "invoice-key-001")
	attempt, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: qPayer})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := f.service.Submit(ctx, token, attempt.ID, models.QuantumTransactionRequest{TxHash: qTx})
	if err != nil || submitted.Status != models.QuantumBroadcast {
		t.Fatalf("submit: %+v %v", submitted, err)
	}
	if _, err := f.service.Submit(ctx, token, attempt.ID, models.QuantumTransactionRequest{TxHash: qTx}); !errors.Is(err, models.ErrQuantumTxSubmitted) {
		t.Fatal("transaction overwritten")
	}
	jobs, err := f.repo.Work(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("work: %v", err)
	}
	job := jobs[0]
	receipt := qProof(job)
	receipt.Confirmations = 1
	if err := f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, receipt, 2, time.Now())); err != nil {
		t.Fatal(err)
	}
	if current, err := f.service.Invoice(ctx, f.actor, invoice.ID); err != nil || current.Status != models.QuantumPending {
		t.Fatal("insufficient confirmation settled")
	}
	if _, err := f.pool.Exec(ctx, "UPDATE quantum_payment_attempts SET next_verify_at=NOW() WHERE id=$1", attempt.ID); err != nil {
		t.Fatal(err)
	}
	jobs, err = f.repo.Work(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("work: %v", err)
	}
	job = jobs[0]
	receipt = qProof(job)
	if err := f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, receipt, 2, time.Now())); err != nil {
		t.Fatal(err)
	}
	paid, err := f.service.Invoice(ctx, f.actor, invoice.ID)
	if err != nil || paid.Status != models.QuantumPaid || paid.PaidTxHash == nil {
		t.Fatalf("paid: %+v %v", paid, err)
	}
	if err := f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, receipt, 2, time.Now())); !errors.Is(err, models.ErrQuantumStale) {
		t.Fatalf("stale lease reused: %v", err)
	}
	events, err := repositories.NewOutboxRepository(f.pool).Claim(ctx, 20, time.Minute)
	if err != nil || len(events) != 0 {
		t.Fatal("P0 dispatched a Quantum event")
	}
	if _, err := f.pool.Exec(ctx, "UPDATE quantum_payment_attempts SET next_verify_at=NOW() WHERE id=$1", attempt.ID); err != nil {
		t.Fatal(err)
	}
	jobs, err = f.repo.Work(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatal(err)
	}
	job = jobs[0]
	receipt = qProof(job)
	receipt.Canonical = false
	if err := f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, receipt, 2, time.Now())); err != nil {
		t.Fatal(err)
	}
	reopened, err := f.service.Invoice(ctx, f.actor, invoice.ID)
	if err != nil || reopened.Status != models.QuantumPending || reopened.PaidTxHash != nil || reopened.Attempts[0].Status != models.QuantumReorged {
		t.Fatalf("reorg: %+v %v", reopened, err)
	}
	var audited int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='quantum_payment_reorg'").Scan(&audited); err != nil || audited != 1 {
		t.Fatal("reorg audit missing")
	}
	// Reinclusion restores the original claim instead of creating a second one.
	if _, err := f.pool.Exec(ctx, "UPDATE quantum_payment_attempts SET next_verify_at=NOW() WHERE id=$1", attempt.ID); err != nil {
		t.Fatal(err)
	}
	jobs, err = f.repo.Work(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("reorg recovery work: %v", err)
	}
	job = jobs[0]
	receipt = qProof(job)
	receipt.BlockNumber++
	receipt.BlockHash = "0x2222222222222222222222222222222222222222222222222222222222222222"
	if err := f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, receipt, 2, time.Now())); err != nil {
		t.Fatal(err)
	}
	reconfirmed, err := f.service.Invoice(ctx, f.actor, invoice.ID)
	if err != nil || reconfirmed.Status != models.QuantumPaid || reconfirmed.Attempts[0].Status != models.QuantumConfirmed {
		t.Fatalf("reinclusion: %+v %v", reconfirmed, err)
	}
	var claims, canonical, paidEvents, reorgEvents int
	if err := f.pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE canonical) FROM quantum_transfer_claims WHERE invoice_id=$1", invoice.ID).Scan(&claims, &canonical); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE event_type='quantum.invoice.paid'),count(*) FILTER(WHERE event_type='quantum.invoice.reorged') FROM outbox_events WHERE aggregate_id=$1", invoice.ID).Scan(&paidEvents, &reorgEvents); err != nil {
		t.Fatal(err)
	}
	if claims != 1 || canonical != 1 || paidEvents != 2 || reorgEvents != 1 {
		t.Fatalf("recovery history: claims=%d canonical=%d paid=%d reorg=%d", claims, canonical, paidEvents, reorgEvents)
	}
}
func TestQuantumClaimRaceAndAmbiguity(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	_, firstToken := f.invoice(t, "invoice-key-001")
	second, secondToken := f.invoice(t, "invoice-key-002")
	first, err := f.service.RegisterAttempt(ctx, firstToken, models.QuantumAttemptRequest{PayerAddress: qPayer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RegisterAttempt(ctx, secondToken, models.QuantumAttemptRequest{PayerAddress: qPayer}); !errors.Is(err, models.ErrQuantumAmbiguous) {
		t.Fatalf("ambiguous intent accepted: %v", err)
	}
	if _, err := f.service.Submit(ctx, firstToken, first.ID, models.QuantumTransactionRequest{TxHash: qTx}); err != nil {
		t.Fatal(err)
	}
	// Simulate a second legacy/untrusted producer bypassing attempt admission. The
	// database claim remains the last line of defence against crediting twice.
	if _, err := f.pool.Exec(ctx, `INSERT INTO quantum_payment_attempts(invoice_id,payer_address,tx_hash,status) VALUES($1,$2,$3,'BROADCAST')`, second.ID, qPayer, qTx); err != nil {
		t.Fatal(err)
	}
	jobs, err := f.repo.Work(ctx)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs: %d %v", len(jobs), err)
	}
	outcomes := make(chan error, 2)
	for _, job := range jobs {
		go func(job models.QuantumWork) {
			proof := qProof(job)
			outcomes <- f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, proof, 2, time.Now()))
		}(job)
	}
	successes := 0
	for range 2 {
		err := <-outcomes
		if err == nil {
			successes++
		} else if !errors.Is(err, models.ErrQuantumDuplicate) {
			t.Fatal(err)
		}
	}
	var paid, claims, events int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM quantum_invoices WHERE status='PAID'").Scan(&paid); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM quantum_transfer_claims").Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE event_type='quantum.invoice.paid'").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || paid != 1 || claims != 1 || events != 1 {
		t.Fatalf("double credit: success=%d paid=%d claims=%d events=%d", successes, paid, claims, events)
	}
}
func TestQuantumCancelVersusConfirmation(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	invoice, token := f.invoice(t, "invoice-key-001")
	attempt, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: qPayer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Submit(ctx, token, attempt.ID, models.QuantumTransactionRequest{TxHash: qTx}); err != nil {
		t.Fatal(err)
	}
	jobs, err := f.repo.Work(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatal(err)
	}
	job := jobs[0]
	proof := qProof(job)
	outcomes := make(chan error, 2)
	go func() { _, err := f.service.Cancel(ctx, f.actor, invoice.ID); outcomes <- err }()
	go func() { outcomes <- f.repo.Apply(ctx, job, services.EvaluateQuantumReceipt(job, proof, 2, time.Now())) }()
	for range 2 {
		if err := <-outcomes; err != nil && !errors.Is(err, models.ErrQuantumNotPending) {
			t.Fatal(err)
		}
	}
	current, err := f.service.Invoice(ctx, f.actor, invoice.ID)
	if err != nil {
		t.Fatal(err)
	}
	switch current.Status {
	case models.QuantumPaid:
		if current.Attempts[0].Status != models.QuantumConfirmed {
			t.Fatal("paid without confirmed attempt")
		}
	case models.QuantumCancelled:
		if current.Attempts[0].Status != models.QuantumReview {
			t.Fatal("cancelled transfer was silently credited")
		}
	default:
		t.Fatalf("inconsistent cancellation race: %+v", current)
	}
}
func TestQuantumAttemptAdmissionLimits(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	_, token := f.invoice(t, "invoice-key-001")
	var first models.QuantumAttempt
	for n := 1; n <= 5; n++ {
		a, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: fmt.Sprintf("0x%040x", n)})
		if err != nil {
			t.Fatal(err)
		}
		if n == 1 {
			first = a
		}
	}
	same, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: fmt.Sprintf("0x%040x", 1)})
	if err != nil || same.ID != first.ID {
		t.Fatal("registration retry created another intent")
	}
	if _, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: fmt.Sprintf("0x%040x", 6)}); !errors.Is(err, models.ErrQuantumAttempts) {
		t.Fatalf("sixth attempt accepted: %v", err)
	}
}

func TestQuantumExpiredIntentCannotBroadcast(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprintf("replacement=%t", replacement), func(t *testing.T) {
			f := newQuantumFixture(t)
			ctx := context.Background()
			invoice, token := f.invoice(t, "expired-intent-invoice")
			attempt, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: qPayer})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, "UPDATE quantum_payment_attempts SET created_at=clock_timestamp()-INTERVAL '16 minutes' WHERE id=$1", attempt.ID); err != nil {
				t.Fatal(err)
			}
			if replacement {
				_, nextToken := f.invoice(t, "replacement-intent-invoice")
				next, err := f.service.RegisterAttempt(ctx, nextToken, models.QuantumAttemptRequest{PayerAddress: qPayer})
				if err != nil {
					t.Fatalf("expired intent blocked replacement: %v", err)
				}
				if _, err := f.service.Submit(ctx, nextToken, next.ID, models.QuantumTransactionRequest{TxHash: qTx}); err != nil {
					t.Fatalf("fresh intent rejected: %v", err)
				}
			}
			if _, err := f.service.Submit(ctx, token, attempt.ID, models.QuantumTransactionRequest{TxHash: qTx}); !errors.Is(err, models.ErrQuantumAttemptExpired) {
				t.Fatalf("expired intent accepted: %v", err)
			}
			current, err := f.service.Invoice(ctx, f.actor, invoice.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(current.Attempts) != 1 || current.Attempts[0].TxHash != nil || current.Attempts[0].Status != models.QuantumCreated {
				t.Fatal("rejected submission changed the old intent")
			}
		})
	}
}

func TestQuantumExpiredIntentCanBeRegisteredAgain(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	_, token := f.invoice(t, "expired-intent-invoice")
	old, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: qPayer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE quantum_payment_attempts SET created_at=clock_timestamp()-INTERVAL '16 minutes' WHERE id=$1", old.ID); err != nil {
		t.Fatal(err)
	}
	fresh, err := f.service.RegisterAttempt(ctx, token, models.QuantumAttemptRequest{PayerAddress: qPayer})
	if err != nil || fresh.ID == old.ID {
		t.Fatalf("intent not renewed: %v", err)
	}
	if _, err := f.service.Submit(ctx, token, old.ID, models.QuantumTransactionRequest{TxHash: qTx}); !errors.Is(err, models.ErrQuantumCheckoutNotFound) {
		t.Fatalf("removed intent accepted: %v", err)
	}
	if _, err := f.service.Submit(ctx, token, fresh.ID, models.QuantumTransactionRequest{TxHash: qTx}); err != nil {
		t.Fatalf("fresh intent rejected: %v", err)
	}
}
