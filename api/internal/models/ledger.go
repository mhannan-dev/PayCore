package models

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Currency is an ISO 4217 code supported by the ledger.
type Currency string

const (
	CurrencyJPY Currency = "JPY"
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
)

// Currencies lists the supported currencies.
var Currencies = []Currency{CurrencyJPY, CurrencyUSD, CurrencyEUR}

// Valid reports whether c is supported.
func (c Currency) Valid() bool {
	switch c {
	case CurrencyJPY, CurrencyUSD, CurrencyEUR:
		return true
	default:
		return false
	}
}

// Scale is the number of minor-unit decimal places (JPY 0, USD/EUR 2).
func (c Currency) Scale() int {
	switch c {
	case CurrencyJPY:
		return 0
	case CurrencyUSD, CurrencyEUR:
		return 2
	default:
		return 0
	}
}

// AccountType is a chart-of-accounts category.
type AccountType string

const (
	AccountAsset     AccountType = "ASSET"
	AccountLiability AccountType = "LIABILITY"
	AccountEquity    AccountType = "EQUITY"
	AccountRevenue   AccountType = "REVENUE"
	AccountExpense   AccountType = "EXPENSE"
)

// Valid reports whether t is a known account type.
func (t AccountType) Valid() bool {
	switch t {
	case AccountAsset, AccountLiability, AccountEquity, AccountRevenue, AccountExpense:
		return true
	default:
		return false
	}
}

// DebitNormal reports whether debits increase the balance (assets, expenses).
// Liabilities, equity and revenue are credit-normal.
func (t AccountType) DebitNormal() bool { return t == AccountAsset || t == AccountExpense }

// AccountSystemRole marks the account a workflow uses for a fixed purpose.
// Each role is held by at most one account per currency (migration 024).
type AccountSystemRole string

const (
	RolePayoutClearing AccountSystemRole = "PAYOUT_CLEARING" // LIABILITY holding reserved payouts.
	RoleOperatingBank  AccountSystemRole = "OPERATING_BANK"  // ASSET receiving captured payouts.
)

// AccountType returns the only account type that may hold the role, or "" for an unknown role.
func (r AccountSystemRole) AccountType() AccountType {
	switch r {
	case RolePayoutClearing:
		return AccountLiability
	case RoleOperatingBank:
		return AccountAsset
	default:
		return ""
	}
}

// Direction is the side of a posting.
type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

// Valid reports whether d is DEBIT or CREDIT.
func (d Direction) Valid() bool { return d == Debit || d == Credit }

// Opposite returns the other side (used by reversals).
func (d Direction) Opposite() Direction {
	if d == Debit {
		return Credit
	}
	return Debit
}

// Amount is a positive number of currency minor units. In JSON it is a string
// of digits ("50000") so JavaScript never rounds it (coding rule 1).
type Amount int64

var amountDigits = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

// ParseAmount parses a positive integer of minor units up to MaxInt64.
func ParseAmount(s string) (Amount, error) {
	if !amountDigits.MatchString(s) {
		return 0, errors.New("amount must be a positive whole number of minor units, as a string of digits")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.New("amount is too large")
	}
	return Amount(v), nil
}

func (a Amount) String() string { return strconv.FormatInt(int64(a), 10) }

func (a Amount) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }

func (a *Amount) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.New(`amount must be a JSON string of digits, e.g. "50000"`)
	}
	v, err := ParseAmount(s)
	if err != nil {
		return err
	}
	*a = v
	return nil
}

// Account is a ledger account with its derived balance (minor units, in the
// account's normal direction; may be negative only with overdraft).
type Account struct {
	ID               string      `json:"id"`
	AccountNumber    string      `json:"account_number"`
	Name             string      `json:"name"`
	Type             AccountType `json:"type"`
	Currency         Currency    `json:"currency"`
	OverdraftAllowed bool        `json:"overdraft_allowed"`
	IsActive         bool        `json:"is_active"`
	// SystemRole is null for ordinary accounts.
	SystemRole *AccountSystemRole `json:"system_role"`
	Balance    string             `json:"balance"`
	CreatedAt  time.Time          `json:"created_at"`
}

// Posting is one debit or credit line of a journal.
type Posting struct {
	ID            string    `json:"id"`
	JournalID     string    `json:"journal_id"`
	AccountID     string    `json:"account_id"`
	AccountNumber string    `json:"account_number"`
	AccountName   string    `json:"account_name"`
	Direction     Direction `json:"direction"`
	Amount        Amount    `json:"amount"`
	CreatedAt     time.Time `json:"created_at"`
}

// AccountPosting is a posting seen from one account, with its journal context.
type AccountPosting struct {
	Posting
	ReferenceID string  `json:"reference_id"`
	Description string  `json:"description"`
	ReversalFor *string `json:"reversal_for_id"`
}

// Journal is a posted, balanced journal entry (a transfer or a reversal).
type Journal struct {
	ID             string    `json:"id"`
	ReferenceID    string    `json:"reference_id"`
	Description    string    `json:"description"`
	Currency       Currency  `json:"currency"`
	PostedAt       time.Time `json:"posted_at"`
	ReversalForID  *string   `json:"reversal_for_id"`
	ReversedByID   *string   `json:"reversed_by_id"`
	CreatedByEmail *string   `json:"created_by_email"`
	Total          Amount    `json:"total"`
	Postings       []Posting `json:"postings"`
}

// --- Requests -------------------------------------------------------------------

// InvalidRequestError explains why a ledger request was rejected (400).
type InvalidRequestError struct {
	Code   string
	Reason string
}

func (e InvalidRequestError) Error() string { return e.Reason }

func invalid(code, reason string) error { return InvalidRequestError{Code: code, Reason: reason} }

var accountNumberPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// CreateAccountRequest adds an account to the chart of accounts.
type CreateAccountRequest struct {
	AccountNumber    string      `json:"account_number"`
	Name             string      `json:"name"`
	Type             AccountType `json:"type"`
	Currency         Currency    `json:"currency"`
	OverdraftAllowed bool        `json:"overdraft_allowed"`
	// SystemRole is optional; null or "" creates an ordinary account.
	SystemRole *AccountSystemRole `json:"system_role"`
}

// Normalize trims and validates the request.
func (r *CreateAccountRequest) Normalize() error {
	r.AccountNumber = strings.TrimSpace(r.AccountNumber)
	r.Name = strings.TrimSpace(r.Name)
	r.Type = AccountType(strings.ToUpper(strings.TrimSpace(string(r.Type))))
	r.Currency = Currency(strings.ToUpper(strings.TrimSpace(string(r.Currency))))
	switch {
	case !accountNumberPattern.MatchString(r.AccountNumber):
		return invalid("invalid_account", "account_number must be 1–64 letters, digits, '.', '_' or '-', starting with a letter or digit.")
	case r.Name == "" || utf8.RuneCountInString(r.Name) > 255:
		return invalid("invalid_account", "name is required and must be at most 255 characters.")
	case !r.Type.Valid():
		return invalid("invalid_account", "type must be ASSET, LIABILITY, EQUITY, REVENUE or EXPENSE.")
	case !r.Currency.Valid():
		return invalid("invalid_account", "currency must be JPY, USD or EUR.")
	}
	if r.SystemRole != nil {
		role := AccountSystemRole(strings.ToUpper(strings.TrimSpace(string(*r.SystemRole))))
		switch {
		case role == "":
			r.SystemRole = nil
		case role.AccountType() == "":
			return invalid("invalid_account", "system_role must be PAYOUT_CLEARING, OPERATING_BANK or null.")
		case role.AccountType() != r.Type:
			return invalid("invalid_account", "PAYOUT_CLEARING requires a LIABILITY account and OPERATING_BANK an ASSET account.")
		default:
			r.SystemRole = &role
		}
	}
	return nil
}

// Leg is one line of a transfer.
type Leg struct {
	AccountID string    `json:"account_id"`
	Direction Direction `json:"direction"`
	Amount    Amount    `json:"amount"`
}

// TransferRequest posts a balanced journal.
type TransferRequest struct {
	ReferenceID string   `json:"reference_id"`
	Description string   `json:"description"`
	Currency    Currency `json:"currency"`
	Legs        []Leg    `json:"legs"`
}

const maxLegs = 20

// Normalize validates shape and balance: 2–20 legs, positive amounts, known
// accounts IDs, and total debits equal to total credits (exact, no overflow).
// Account existence, currency and funds are checked under lock by the store.
func (r *TransferRequest) Normalize() error {
	r.ReferenceID = strings.TrimSpace(r.ReferenceID)
	r.Description = strings.TrimSpace(r.Description)
	r.Currency = Currency(strings.ToUpper(strings.TrimSpace(string(r.Currency))))
	switch {
	case r.ReferenceID == "" || utf8.RuneCountInString(r.ReferenceID) > 128:
		return invalid("invalid_transfer", "reference_id is required and must be at most 128 characters.")
	case r.Description == "" || utf8.RuneCountInString(r.Description) > 500:
		return invalid("invalid_transfer", "description is required and must be at most 500 characters.")
	case !r.Currency.Valid():
		return invalid("invalid_transfer", "currency must be JPY, USD or EUR.")
	case len(r.Legs) < 2 || len(r.Legs) > maxLegs:
		return invalid("invalid_transfer", "a transfer needs 2 to 20 legs.")
	}
	debits, credits := new(big.Int), new(big.Int)
	for i := range r.Legs {
		leg := &r.Legs[i]
		leg.AccountID = strings.ToLower(strings.TrimSpace(leg.AccountID))
		leg.Direction = Direction(strings.ToUpper(strings.TrimSpace(string(leg.Direction))))
		switch {
		case !uuidPattern.MatchString(leg.AccountID):
			return invalid("invalid_transfer", "every leg needs an account_id (UUID).")
		case !leg.Direction.Valid():
			return invalid("invalid_transfer", "every leg needs direction DEBIT or CREDIT.")
		case leg.Amount <= 0:
			return invalid("invalid_transfer", `every leg needs a positive amount, e.g. "50000".`)
		}
		if leg.Direction == Debit {
			debits.Add(debits, big.NewInt(int64(leg.Amount)))
		} else {
			credits.Add(credits, big.NewInt(int64(leg.Amount)))
		}
	}
	if debits.Sign() == 0 || credits.Sign() == 0 {
		return invalid("invalid_transfer", "a transfer needs at least one DEBIT and one CREDIT leg.")
	}
	if debits.Cmp(credits) != 0 {
		return invalid("unbalanced_transfer", "total debits ("+debits.String()+") must equal total credits ("+credits.String()+").")
	}
	if !debits.IsInt64() {
		return invalid("invalid_transfer", "the transfer total is too large.")
	}
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidUUID reports whether s is a lowercase or uppercase canonical UUID.
func ValidUUID(s string) bool { return uuidPattern.MatchString(strings.ToLower(s)) }

// ReverseRequest optionally overrides the reversal's description.
type ReverseRequest struct {
	Description string `json:"description"`
}

// NewJournal is a validated journal for the store to post.
type NewJournal struct {
	ReferenceID    string
	Description    string
	Currency       Currency
	Legs           []Leg
	ReversalForID  *string
	CreatedBy      string
	CreatedByEmail string
}

// ZeroSumReport is the result of the system-wide balance audit for one currency.
type ZeroSumReport struct {
	Currency           Currency  `json:"currency"`
	TotalDebits        string    `json:"total_debits"`
	TotalCredits       string    `json:"total_credits"`
	Difference         string    `json:"difference"`
	Journals           int       `json:"journals"`
	Postings           int       `json:"postings"`
	UnbalancedJournals []string  `json:"unbalanced_journals"`
	Balanced           bool      `json:"balanced"`
	CheckedAt          time.Time `json:"checked_at"`
}

// AccountQuery filters and pages the chart of accounts.
type AccountQuery struct {
	Search   string
	Type     AccountType
	Currency Currency
	Limit    int
	Offset   int
}

// JournalQuery filters and pages journals.
type JournalQuery struct {
	Currency  Currency // empty means all currencies
	Search    string   // reference or description
	AccountID string   // only journals touching this account
	Limit     int
	Offset    int
}

// LedgerWriter posts inside one idempotent ledger transaction (implemented by
// the repository, used by the service). Declared here so neither side imports
// the other.
type LedgerWriter interface {
	PostJournal(ctx context.Context, j NewJournal) (Journal, error)
	LockJournal(ctx context.Context, id string) (Journal, error)
}

// IdempotentResult is a stored or fresh response for an idempotent request.
type IdempotentResult struct {
	Status   int
	Body     []byte
	Replayed bool
}
