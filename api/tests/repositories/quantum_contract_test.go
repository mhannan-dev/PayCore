package repositories_test

import (
	"context"
	"testing"
	"time"

	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

func TestQuantumContractSnapshotAndSettlement(t *testing.T) {
	f := newQuantumFixture(t)
	ctx := context.Background()
	legacy, legacyToken := f.invoice(t, "legacy-contract-key")
	contract := "0xdddddddddddddddddddddddddddddddddddddddd"
	f.config.PaymentContractAddress = &contract
	f.service = services.NewQuantumService(f.repo, f.config, "https://checkout.example.test")
	invoice, token := f.invoice(t, "contract-invoice-key")
	if invoice.PaymentContractAddress == nil || *invoice.PaymentContractAddress != contract || invoice.OrderID != models.QuantumOrderID(invoice.ID) {
		t.Fatalf("invoice snapshot: %+v", invoice)
	}
	if legacy.PaymentContractAddress != nil {
		t.Fatal("legacy invoice changed mode")
	}
	otherContract := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	cfg := f.config
	cfg.PaymentContractAddress = &otherContract
	changed := services.NewQuantumService(f.repo, cfg, "https://checkout.example.test")
	checkout, err := changed.Checkout(ctx, token)
	if err != nil || checkout.PaymentContractAddress == nil || *checkout.PaymentContractAddress != contract || checkout.OrderID != invoice.OrderID {
		t.Fatalf("checkout followed current config: %+v %v", checkout, err)
	}
	oldCheckout, err := changed.Checkout(ctx, legacyToken)
	if err != nil || oldCheckout.PaymentContractAddress != nil || oldCheckout.OrderID != legacy.OrderID {
		t.Fatal("legacy checkout changed mode")
	}
	// Identical payer/amount/receiver no longer make contract invoices ambiguous.
	_, secondToken := f.invoice(t, "contract-second-key")
	for _, checkoutToken := range []string{legacyToken, token, secondToken} {
		if _, err := f.service.RegisterAttempt(ctx, checkoutToken, models.QuantumAttemptRequest{PayerAddress: qPayer}); err != nil {
			t.Fatal(err)
		}
	}
	status, err := f.service.CheckoutStatus(ctx, token)
	if err != nil || status.Attempt == nil {
		t.Fatal("missing attempt")
	}
	if _, err := f.service.Submit(ctx, token, status.Attempt.ID, models.QuantumTransactionRequest{TxHash: qTx}); err != nil {
		t.Fatal(err)
	}
	jobs, err := f.repo.Work(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("work: %v %v", jobs, err)
	}
	w := jobs[0]
	r := qProof(w)
	r.TxRecipient = contract
	r.Payments = []models.QuantumInvoicePaid{{Contract: contract, OrderID: invoice.OrderID, Payer: qPayer, Merchant: qReceiver, Amount: int64(invoice.AmountBaseUnits), LogIndex: 7}}
	d := services.EvaluateQuantumReceipt(w, r, 2, time.Now())
	if d.Status != models.QuantumConfirmed {
		t.Fatalf("decision: %+v", d)
	}
	if err := f.repo.Apply(ctx, w, d); err != nil {
		t.Fatal(err)
	}
	paid, err := f.service.Invoice(ctx, f.actor, invoice.ID)
	if err != nil || paid.Status != models.QuantumPaid || paid.PaidTxHash == nil || *paid.PaidTxHash != qTx {
		t.Fatalf("paid: %+v %v", paid, err)
	}
	var logIndex int
	if err := f.pool.QueryRow(ctx, "SELECT log_index FROM quantum_transfer_claims WHERE invoice_id=$1", invoice.ID).Scan(&logIndex); err != nil || logIndex != 7 {
		t.Fatal("InvoicePaid log was not reserved")
	}
	for _, address := range []string{"bad", "0x0000000000000000000000000000000000000000", qToken} {
		if _, err := f.pool.Exec(ctx, "UPDATE quantum_invoices SET payment_contract_address=$1 WHERE id=$2", address, invoice.ID); err == nil {
			t.Fatal("invalid contract snapshot accepted")
		}
	}
}
