package models_test

import (
	"math"
	"paycore/api/internal/models"
	"testing"
)

func TestQuantumAmounts(t *testing.T) {
	for _, tc := range []struct {
		input    string
		decimals int
		want     models.Amount
	}{{"1.25", 6, 1250000}, {"00012.500000", 6, 12500000}, {"9223372036854.775807", 6, models.Amount(math.MaxInt64)}, {"1", 0, 1}} {
		got, err := models.ParseQuantumDecimal(tc.input, tc.decimals)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %v %v", tc.input, got, err)
		}
		formatted, err := models.FormatQuantumAmount(got, tc.decimals)
		if err != nil {
			t.Fatal(err)
		}
		back, err := models.ParseQuantumDecimal(formatted, tc.decimals)
		if err != nil || back != got {
			t.Fatalf("round trip %s", formatted)
		}
	}
	for _, s := range []string{"0", "-1", ".5", "1.", "1.0000001", "1e2", "9223372036854.775808"} {
		if _, err := models.ParseQuantumDecimal(s, 6); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if _, err := models.ParseQuantumDecimal("1", -1); err == nil {
		t.Fatal("invalid precision accepted")
	}
}

func TestQuantumOrderID(t *testing.T) {
	// Independently generated with viem keccak256(toHex(...)); shared Solidity encoding.
	const want = "0x797307cb894f69cc35dd9635045e83fb9989baaf0c6a900c0a0caf03324a0634"
	if got := models.QuantumOrderID("11111111-1111-4111-8111-111111111111"); got != want {
		t.Fatalf("order ID %s", got)
	}
	if models.QuantumOrderID("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA") != models.QuantumOrderID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa") {
		t.Fatal("order IDs depend on UUID case")
	}
}
func TestQuantumNormalizationAndEnums(t *testing.T) {
	req := models.QuantumMerchantRequest{DisplayName: " Demo ", ReceiverAddress: "0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa"}
	if err := req.Normalize(); err != nil || req.DisplayName != "Demo" || req.ReceiverAddress != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("normalize: %+v %v", req, err)
	}
	for _, addr := range []string{"bad", "0x0000000000000000000000000000000000000000", "0xzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"} {
		if models.ValidQuantumAddress(addr) {
			t.Fatal("invalid address accepted")
		}
	}
	for _, status := range []models.QuantumInvoiceStatus{models.QuantumPending, models.QuantumPaid, models.QuantumCancelled, models.QuantumExpired} {
		if !status.Valid() {
			t.Fatal(status)
		}
	}
	for _, status := range []models.QuantumAttemptStatus{models.QuantumCreated, models.QuantumBroadcast, models.QuantumVerifying, models.QuantumConfirmed, models.QuantumFailed, models.QuantumReorged, models.QuantumReview} {
		if !status.Valid() {
			t.Fatal(status)
		}
	}
	for _, failure := range []models.QuantumFailure{models.QuantumWrongChain, models.QuantumWrongToken, models.QuantumWrongRecipient, models.QuantumWrongPayer, models.QuantumUnderpaid, models.QuantumOverpaid, models.QuantumLate, models.QuantumTxFailed, models.QuantumTxNotFound, models.QuantumDuplicateTransfer} {
		if !failure.Valid() {
			t.Fatal(failure)
		}
	}
	if models.QuantumInvoiceStatus("OTHER").Valid() || models.QuantumAttemptStatus("OTHER").Valid() || models.QuantumFailure("OTHER").Valid() {
		t.Fatal("unknown enum accepted")
	}
	for _, req := range []models.QuantumInvoiceRequest{{AmountBaseUnits: 0, Description: "x", ExpiresInMinutes: 30}, {AmountBaseUnits: 1, Description: "", ExpiresInMinutes: 30}, {AmountBaseUnits: 1, Description: "x", ExpiresInMinutes: 4}, {AmountBaseUnits: 1, Description: "x", ExpiresInMinutes: 1441}} {
		if err := req.Normalize(); err == nil {
			t.Fatal("invalid invoice accepted")
		}
	}
}
