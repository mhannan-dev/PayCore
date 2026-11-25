package services

import (
	"time"

	"paycore/api/internal/models"
)

func evaluateQuantumContractPayment(w models.QuantumWork, r models.QuantumReceipt, minimum int64) models.QuantumDecision {
	i := w.Invoice.Invoice
	if i.PaymentContractAddress == nil || r.TxRecipient != *i.PaymentContractAddress {
		return quantumRejection(models.QuantumWrongContract, true)
	}
	var match *models.QuantumInvoicePaid
	foundContract := false
	for _, event := range r.Payments {
		if event.Contract != *i.PaymentContractAddress {
			continue
		}
		foundContract = true
		if event.OrderID != i.OrderID {
			continue
		}
		if match != nil {
			return quantumRejection(models.QuantumDuplicateTransfer, true)
		}
		copy := event
		match = &copy
	}
	if !foundContract {
		return quantumRejection(models.QuantumWrongContract, true)
	}
	if match == nil || !models.ValidQuantumHash(i.OrderID) {
		return quantumRejection(models.QuantumWrongOrder, true)
	}
	if match.Payer != w.Attempt.PayerAddress || r.TxSender != w.Attempt.PayerAddress {
		return quantumRejection(models.QuantumWrongPayer, true)
	}
	if match.Merchant != i.ReceiverAddress {
		return quantumRejection(models.QuantumWrongRecipient, true)
	}
	if match.Overflow || match.Amount > int64(i.AmountBaseUnits) {
		return quantumRejection(models.QuantumOverpaid, true)
	}
	if match.Amount < int64(i.AmountBaseUnits) {
		return quantumRejection(models.QuantumUnderpaid, true)
	}
	if r.BlockTime.After(i.ExpiresAt) || r.BlockTime.Before(w.Attempt.CreatedAt.Truncate(time.Second)) {
		return quantumRejection(models.QuantumLate, true)
	}
	if r.Confirmations < minimum {
		return models.QuantumDecision{Status: models.QuantumVerifying, Receipt: r, RetryAfter: 3 * time.Second}
	}
	// InvoicePaid is authoritative for this validated, immutable contract snapshot.
	// Reuse the canonical claim store, reserving the InvoicePaid log index.
	claim := &models.QuantumTransfer{LogIndex: match.LogIndex, Token: i.TokenAddress, From: match.Payer, To: match.Merchant, Amount: match.Amount}
	return models.QuantumDecision{Status: models.QuantumConfirmed, Receipt: r, Transfer: claim, RetryAfter: time.Minute}
}
