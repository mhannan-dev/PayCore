package services_test

import (
	"testing"

	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

func contractProof() (models.QuantumWork, models.QuantumReceipt) {
	w, r, _ := quantumProof()
	contract := "0xdddddddddddddddddddddddddddddddddddddddd"
	w.Invoice.Invoice.ID = "11111111-1111-4111-8111-111111111111"
	w.Invoice.Invoice.OrderID = models.QuantumOrderID(w.Invoice.Invoice.ID)
	w.Invoice.Invoice.PaymentContractAddress = &contract
	r.TxRecipient = contract
	r.Payments = []models.QuantumInvoicePaid{{Contract: contract, OrderID: w.Invoice.Invoice.OrderID, Payer: quantumPayer, Merchant: quantumReceiver, Amount: 1250000, LogIndex: 3}}
	return w, r
}

func TestQuantumContractAttribution(t *testing.T) {
	cases := []struct {
		name    string
		alter   func(*models.QuantumWork, *models.QuantumReceipt)
		failure models.QuantumFailure
	}{
		{"direct transfer cannot settle contract invoice", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.TxRecipient = quantumToken; r.Payments = nil }, models.QuantumWrongContract},
		{"wrong emitter", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments[0].Contract = quantumToken }, models.QuantumWrongContract},
		{"missing event", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments = nil }, models.QuantumWrongContract},
		{"wrong order", func(_ *models.QuantumWork, r *models.QuantumReceipt) {
			r.Payments[0].OrderID = models.QuantumOrderID("different-invoice")
		}, models.QuantumWrongOrder},
		{"wrong payer", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments[0].Payer = quantumReceiver }, models.QuantumWrongPayer},
		{"wrong signer", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.TxSender = quantumReceiver }, models.QuantumWrongPayer},
		{"wrong merchant", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments[0].Merchant = quantumPayer }, models.QuantumWrongRecipient},
		{"underpaid", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments[0].Amount-- }, models.QuantumUnderpaid},
		{"overpaid", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments[0].Amount++ }, models.QuantumOverpaid},
		{"overflow", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments[0].Overflow = true }, models.QuantumOverpaid},
		{"duplicate events", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Payments = append(r.Payments, r.Payments[0]) }, models.QuantumDuplicateTransfer},
		{"event without token movement", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers = nil }, models.QuantumWrongToken},
		{"token moved to another address", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].To = quantumPayer }, models.QuantumWrongToken},
		{"token moved a different amount", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].Amount-- }, models.QuantumWrongToken},
		{"another token moved", func(_ *models.QuantumWork, r *models.QuantumReceipt) { r.Transfers[0].Token = quantumReceiver }, models.QuantumWrongToken},
		{"expired", func(w *models.QuantumWork, r *models.QuantumReceipt) {
			r.BlockTime = w.Invoice.Invoice.ExpiresAt.Add(1)
		}, models.QuantumLate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, r := contractProof()
			tc.alter(&w, &r)
			d := services.EvaluateQuantumReceipt(w, r, 2, r.BlockTime)
			if d.Status != models.QuantumReview || d.Failure == nil || *d.Failure != tc.failure || d.Transfer != nil {
				t.Fatalf("decision: %+v", d)
			}
		})
	}
}

// payWithAuthorization pulls funds into the contract, then forwards them.
func TestQuantumContractForwardedTransferSettles(t *testing.T) {
	w, r := contractProof()
	r.Transfers = []models.QuantumTransfer{
		{Token: quantumToken, From: quantumPayer, To: *w.Invoice.Invoice.PaymentContractAddress, Amount: 1250000},
		{Token: quantumToken, From: *w.Invoice.Invoice.PaymentContractAddress, To: quantumReceiver, Amount: 1250000},
	}
	if d := services.EvaluateQuantumReceipt(w, r, 2, r.BlockTime); d.Status != models.QuantumConfirmed {
		t.Fatalf("forwarded contract payment rejected: %+v", d)
	}
}

func TestQuantumContractConfirmationAndReorg(t *testing.T) {
	w, r := contractProof()
	r.Confirmations = 1
	if d := services.EvaluateQuantumReceipt(w, r, 2, r.BlockTime); d.Status != models.QuantumVerifying || d.Transfer != nil {
		t.Fatal("premature payment")
	}
	r.Confirmations = 2
	d := services.EvaluateQuantumReceipt(w, r, 2, r.BlockTime)
	if d.Status != models.QuantumConfirmed || d.Transfer == nil || d.Transfer.LogIndex != 3 {
		t.Fatalf("contract event not claimed: %+v", d)
	}
	w.Attempt.Status = models.QuantumConfirmed
	w.ObservedHash = &r.BlockHash
	r.Payments = nil
	if d := services.EvaluateQuantumReceipt(w, r, 2, r.BlockTime); !d.Reorg || d.Status != models.QuantumReorged {
		t.Fatal("missing event did not invalidate confirmation")
	}
}
