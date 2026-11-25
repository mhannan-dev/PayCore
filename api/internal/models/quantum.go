package models

import (
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/sha3"
)

var (
	ErrQuantumMerchantExists   = errors.New("quantum merchant exists")
	ErrQuantumMerchantNotFound = errors.New("quantum merchant not found")
	ErrQuantumInvoiceNotFound  = errors.New("quantum invoice not found")
	ErrQuantumCheckoutNotFound = errors.New("quantum checkout not found")
	ErrQuantumNotPending       = errors.New("quantum invoice is not pending")
	ErrQuantumTxSubmitted      = errors.New("quantum transaction already submitted")
	ErrQuantumAttemptExpired   = errors.New("quantum payment intent expired")
	ErrQuantumAttempts         = errors.New("too many open quantum attempts")
	ErrQuantumAmbiguous        = errors.New("ambiguous quantum payment intent")
	ErrQuantumDuplicate        = errors.New("quantum transfer already claimed")
	ErrQuantumIdempotency      = errors.New("quantum idempotency key changed")
	ErrQuantumStale            = errors.New("quantum verification lease lost")
)

type QuantumInvoiceStatus string

const (
	QuantumPending   QuantumInvoiceStatus = "PENDING"
	QuantumPaid      QuantumInvoiceStatus = "PAID"
	QuantumExpired   QuantumInvoiceStatus = "EXPIRED"
	QuantumCancelled QuantumInvoiceStatus = "CANCELLED"
)

func (s QuantumInvoiceStatus) Valid() bool {
	return s == QuantumPending || s == QuantumPaid || s == QuantumExpired || s == QuantumCancelled
}

type QuantumAttemptStatus string

const (
	QuantumCreated   QuantumAttemptStatus = "CREATED"
	QuantumBroadcast QuantumAttemptStatus = "BROADCAST"
	QuantumVerifying QuantumAttemptStatus = "VERIFYING"
	QuantumConfirmed QuantumAttemptStatus = "CONFIRMED"
	QuantumFailed    QuantumAttemptStatus = "FAILED"
	QuantumReorged   QuantumAttemptStatus = "REORGED"
	QuantumReview    QuantumAttemptStatus = "REVIEW_REQUIRED"
)

func (s QuantumAttemptStatus) Valid() bool {
	switch s {
	case QuantumCreated, QuantumBroadcast, QuantumVerifying, QuantumConfirmed, QuantumFailed, QuantumReorged, QuantumReview:
		return true
	default:
		return false
	}
}

type QuantumFailure string

const (
	QuantumWrongChain        QuantumFailure = "wrong_chain"
	QuantumWrongToken        QuantumFailure = "wrong_token"
	QuantumWrongContract     QuantumFailure = "wrong_contract"
	QuantumWrongOrder        QuantumFailure = "wrong_order"
	QuantumWrongRecipient    QuantumFailure = "wrong_recipient"
	QuantumWrongPayer        QuantumFailure = "wrong_payer"
	QuantumUnderpaid         QuantumFailure = "underpaid"
	QuantumOverpaid          QuantumFailure = "overpaid"
	QuantumLate              QuantumFailure = "late_payment"
	QuantumTxFailed          QuantumFailure = "tx_failed"
	QuantumTxNotFound        QuantumFailure = "tx_not_found"
	QuantumDuplicateTransfer QuantumFailure = "duplicate_transfer"
)

func (f QuantumFailure) Valid() bool {
	switch f {
	case QuantumWrongChain, QuantumWrongToken, QuantumWrongContract, QuantumWrongOrder, QuantumWrongRecipient, QuantumWrongPayer, QuantumUnderpaid, QuantumOverpaid, QuantumLate, QuantumTxFailed, QuantumTxNotFound, QuantumDuplicateTransfer:
		return true
	default:
		return false
	}
}

type QuantumConfig struct {
	// Nil keeps direct ERC-20 checkout for deployments without a payment contract.
	PaymentContractAddress *string `json:"payment_contract_address"`
	Enabled                bool    `json:"enabled"`
	ChainID                uint64  `json:"chain_id"`
	TokenAddress           string  `json:"token_address"`
	TokenSymbol            string  `json:"token_symbol"`
	TokenDecimals          int     `json:"token_decimals"`
	ExplorerURL            string  `json:"explorer_url"`
	MinConfirmations       int64   `json:"min_confirmations"`
}
type QuantumMerchant struct {
	ID              string    `json:"id"`
	DisplayName     string    `json:"display_name"`
	ReceiverAddress string    `json:"receiver_address"`
	CreatedAt       time.Time `json:"created_at"`
}
type QuantumAttempt struct {
	ID            string               `json:"id"`
	PayerAddress  string               `json:"payer_address"`
	TxHash        *string              `json:"tx_hash"`
	Status        QuantumAttemptStatus `json:"status"`
	FailureCode   *QuantumFailure      `json:"failure_code"`
	Confirmations int64                `json:"confirmations"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
}
type QuantumInvoice struct {
	OrderID string `json:"order_id"`
	// Nil identifies a direct-transfer invoice; this snapshot never follows config changes.
	PaymentContractAddress *string              `json:"payment_contract_address"`
	ID                     string               `json:"id"`
	Description            string               `json:"description"`
	AmountBaseUnits        Amount               `json:"amount_base_units"`
	ChainID                uint64               `json:"chain_id"`
	TokenAddress           string               `json:"token_address"`
	ReceiverAddress        string               `json:"receiver_address"`
	Status                 QuantumInvoiceStatus `json:"status"`
	CheckoutURL            string               `json:"checkout_url"`
	ExpiresAt              time.Time            `json:"expires_at"`
	PaidAt                 *time.Time           `json:"paid_at"`
	PaidTxHash             *string              `json:"paid_tx_hash"`
	Attempts               []QuantumAttempt     `json:"attempts"`
	CreatedAt              time.Time            `json:"created_at"`
}
type QuantumCheckout struct {
	OrderID                string               `json:"order_id"`
	PaymentContractAddress *string              `json:"payment_contract_address"`
	MerchantName           string               `json:"merchant_name"`
	Description            string               `json:"description"`
	AmountBaseUnits        Amount               `json:"amount_base_units"`
	ChainID                uint64               `json:"chain_id"`
	TokenAddress           string               `json:"token_address"`
	TokenSymbol            string               `json:"token_symbol"`
	TokenDecimals          int                  `json:"token_decimals"`
	ReceiverAddress        string               `json:"receiver_address"`
	Status                 QuantumInvoiceStatus `json:"status"`
	ExpiresAt              time.Time            `json:"expires_at"`
	ExplorerURL            string               `json:"explorer_url"`
}
type QuantumCheckoutStatus struct {
	InvoiceStatus QuantumInvoiceStatus `json:"invoice_status"`
	Attempt       *QuantumAttempt      `json:"attempt"`
	PaidTxHash    *string              `json:"paid_tx_hash"`
}
type QuantumMerchantRequest struct {
	DisplayName     string `json:"display_name"`
	ReceiverAddress string `json:"receiver_address"`
}
type QuantumInvoiceRequest struct {
	AmountBaseUnits  Amount `json:"amount_base_units"`
	Description      string `json:"description"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
}
type QuantumAttemptRequest struct {
	PayerAddress string `json:"payer_address"`
}
type QuantumTransactionRequest struct {
	TxHash string `json:"tx_hash"`
}

var quantumAddress = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
var quantumHash = regexp.MustCompile(`^0x[0-9a-f]{64}$`)

func ValidQuantumAddress(s string) bool {
	return quantumAddress.MatchString(s) && s != "0x0000000000000000000000000000000000000000"
}
func ValidQuantumHash(s string) bool { return quantumHash.MatchString(s) }

// QuantumOrderID matches keccak256(abi.encodePacked("paycore-quantum:", lowercaseUUID)).
// Immutable invoice identity makes independent storage of this value unnecessary.
func QuantumOrderID(invoiceID string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("paycore-quantum:" + strings.ToLower(invoiceID))) // hash writes cannot fail
	return "0x" + hex.EncodeToString(h.Sum(nil))
}
func (r *QuantumMerchantRequest) Normalize() error {
	r.DisplayName = strings.TrimSpace(r.DisplayName)
	r.ReceiverAddress = strings.ToLower(strings.TrimSpace(r.ReceiverAddress))
	if utf8.RuneCountInString(r.DisplayName) < 1 || utf8.RuneCountInString(r.DisplayName) > 120 || !ValidQuantumAddress(r.ReceiverAddress) {
		return InvalidRequestError{Code: "invalid_merchant", Reason: "Supply a display name (1–120 characters) and a nonzero EVM receiver address."}
	}
	return nil
}
func (r *QuantumInvoiceRequest) Normalize() error {
	r.Description = strings.TrimSpace(r.Description)
	if r.AmountBaseUnits <= 0 || utf8.RuneCountInString(r.Description) < 1 || utf8.RuneCountInString(r.Description) > 500 || r.ExpiresInMinutes < 5 || r.ExpiresInMinutes > 1440 {
		return InvalidRequestError{Code: "invalid_invoice", Reason: "Supply positive int64 base units, a description (1–500 characters), and expiry from 5 to 1440 minutes."}
	}
	return nil
}

// Internal snapshots never serialize through a public endpoint.
type QuantumInvoiceRecord struct {
	Invoice       QuantumInvoice
	MerchantID    string
	OwnerID       string
	MerchantName  string
	CheckoutToken string
	TokenDecimals int
	ExplorerURL   string
}
type QuantumQuery struct {
	Status QuantumInvoiceStatus
	Limit  int
	Offset int
}
type QuantumTransfer struct {
	LogIndex uint64
	Token    string
	From     string
	To       string
	Amount   int64
	Overflow bool
}
type QuantumReceipt struct {
	Payments      []QuantumInvoicePaid
	Found         bool
	Canonical     bool
	ChainID       uint64
	Success       bool
	TxSender      string
	TxRecipient   string
	BlockNumber   uint64
	BlockHash     string
	BlockTime     time.Time
	Confirmations int64
	Transfers     []QuantumTransfer
}

// QuantumInvoicePaid carries a canonical ABI-decoded event; the service checks
// its emitter and attribution against the immutable invoice snapshot.
type QuantumInvoicePaid struct {
	Contract string
	OrderID  string
	Payer    string
	Merchant string
	Amount   int64
	Overflow bool
	LogIndex uint64
}

type QuantumWork struct {
	Invoice      QuantumInvoiceRecord
	Attempt      QuantumAttempt
	LeaseID      string
	ObservedHash *string
	Failures     int
}
type QuantumDecision struct {
	Status     QuantumAttemptStatus
	Failure    *QuantumFailure
	Receipt    QuantumReceipt
	Transfer   *QuantumTransfer
	RetryAfter time.Duration
	Reorg      bool
}
