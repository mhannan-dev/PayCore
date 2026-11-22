package models

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrPayoutNotFound = errors.New("payout not found")
	ErrPayoutSource   = errors.New("invalid payout source")
	ErrPayoutClearing = errors.New("payout clearing account missing")
	ErrPayoutBank     = errors.New("payout operating bank missing")
	ErrPayoutState    = errors.New("payout is not reserved")
	ErrPayoutProvider = errors.New("unsupported payout provider")
)

type SettlementQuery struct {
	Currency Currency // Empty means all currencies.
	Limit    int
	Offset   int
}

// SettlementWriter keeps payout state and its ledger postings in one transaction.
type SettlementWriter interface {
	PostJournal(context.Context, NewJournal) (Journal, error)
	Accounts(context.Context, Currency) ([]Account, error)
	GetSettlement(context.Context, string) (Settlement, error)
	CreateSettlement(context.Context, Settlement) error
	UpdateSettlement(context.Context, Settlement) error
}

// SettlementStatus represents the lifecycle state of a payout settlement.
type SettlementStatus string

const (
	SettlementReserved SettlementStatus = "RESERVED"
	SettlementCaptured SettlementStatus = "CAPTURED"
	SettlementReleased SettlementStatus = "RELEASED"
)

type ProviderMode string

const (
	ProviderSandbox ProviderMode = "SANDBOX"
	ProviderLive    ProviderMode = "LIVE"
)

// Settlement represents a two-phase payout record linked to ledger journals.
type Settlement struct {
	ID                   string           `json:"id"`
	PayoutReference      string           `json:"payout_reference"`
	SourceAccountID      string           `json:"source_account_id"`
	SourceAccountName    string           `json:"source_account_name"`
	ClearingAccountID    string           `json:"clearing_account_id"`
	ClearingAccountName  string           `json:"clearing_account_name"`
	DestinationAccountID *string          `json:"destination_account_id"` // Null until captured.
	Amount               Amount           `json:"amount"`                 // Positive minor units; JSON digit string.
	Currency             Currency         `json:"currency"`
	Status               SettlementStatus `json:"status"`
	ProviderMode         ProviderMode     `json:"provider_mode"`
	ProviderReference    string           `json:"provider_reference"`
	BeneficiaryName      string           `json:"beneficiary_name"`
	BeneficiaryBank      string           `json:"beneficiary_bank"`
	BeneficiaryAccount   string           `json:"beneficiary_account"`
	ReserveJournalID     string           `json:"reserve_journal_id"`
	CaptureJournalID     *string          `json:"capture_journal_id"` // Null unless captured.
	ReleaseJournalID     *string          `json:"release_journal_id"` // Null unless released.
	CreatedBy            *string          `json:"created_by"`         // Null if the creator was deleted.
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

func (r *CreatePayoutRequest) Normalize() error {
	r.SourceAccountID = strings.ToLower(strings.TrimSpace(r.SourceAccountID))
	r.Currency = Currency(strings.ToUpper(strings.TrimSpace(string(r.Currency))))
	r.Amount = strings.TrimSpace(r.Amount)
	r.BeneficiaryName = strings.TrimSpace(r.BeneficiaryName)
	r.BeneficiaryBank = strings.TrimSpace(r.BeneficiaryBank)
	r.BeneficiaryAccount = strings.TrimSpace(r.BeneficiaryAccount)
	if !ValidUUID(r.SourceAccountID) || !r.Currency.Valid() {
		return invalid("invalid_payout", "Select a source account and currency (JPY, USD or EUR).")
	}
	if _, err := ParseAmount(r.Amount); err != nil {
		return invalid("invalid_amount", "Amount must be a positive integer minor unit within int64 limits.")
	}
	for _, field := range []string{r.BeneficiaryName, r.BeneficiaryBank, r.BeneficiaryAccount} {
		if field == "" || utf8.RuneCountInString(field) > 120 {
			return invalid("invalid_beneficiary", "Beneficiary name, bank and account are required and must be at most 120 characters each.")
		}
	}
	return nil
}

// CreatePayoutRequest specifies details to initiate a payout reservation.
type CreatePayoutRequest struct {
	SourceAccountID    string   `json:"source_account_id"`
	Amount             string   `json:"amount"`
	Currency           Currency `json:"currency"`
	BeneficiaryName    string   `json:"beneficiary_name"`
	BeneficiaryBank    string   `json:"beneficiary_bank"`
	BeneficiaryAccount string   `json:"beneficiary_account"`
}
