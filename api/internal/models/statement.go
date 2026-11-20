package models

import (
	"errors"
	"time"
)

const StatementMaxBytes = 2 << 20
const StatementMaxRows = 2000

var (
	ErrStatementFormat    = errors.New("invalid CSV statement: use the six-column template, at most 2 MiB and 2000 rows")
	ErrStatementAccount   = errors.New("statement requires an asset or liability account")
	ErrStatementInvalid   = errors.New("statement contains invalid rows")
	ErrStatementChanged   = errors.New("statement changed; preview it again")
	ErrStatementDuplicate = errors.New("statement was already imported for this account")
	ErrStatementNotFound  = errors.New("statement import not found")
)

type StatementRequest struct {
	AttachmentID string `json:"attachment_id" binding:"required,uuid"`
	AccountID    string `json:"account_id" binding:"required,uuid"`
	ExpectedHash string `json:"expected_hash" binding:"omitempty,len=64,hexadecimal"`
}

type StatementEntry struct {
	BookedDate  string    `json:"booked_date"`
	Reference   string    `json:"reference"`
	Currency    Currency  `json:"currency"`
	Amount      Amount    `json:"amount"`
	Direction   Direction `json:"direction"`
	Description string    `json:"description"`
}

type StatementRow struct {
	RowNumber int `json:"row_number"`
	// Values are trimmed and bounded for display; invalid originals are never persisted.
	Values []string        `json:"values"`
	Entry  *StatementEntry `json:"entry"` // nil when this row is invalid
	Errors []string        `json:"errors"`
}

type StatementPreview struct {
	AttachmentID      string         `json:"attachment_id"`
	AccountID         string         `json:"account_id"`
	FileName          string         `json:"file_name"`
	Currency          Currency       `json:"currency"`
	Rows              []StatementRow `json:"rows"`
	InvalidRows       int            `json:"invalid_rows"`
	TotalDebits       string         `json:"total_debits"`
	TotalCredits      string         `json:"total_credits"`
	ContentHash       *string        `json:"content_hash"`        // absent until all rows validate
	DuplicateImportID *string        `json:"duplicate_import_id"` // absent unless already imported
	CanImport         bool           `json:"can_import"`
}

type StatementImport struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"-"`             // immutable actor snapshot, financial history survives user deletion
	AttachmentID *string   `json:"attachment_id"` // null if the source attachment was hard-deleted
	AccountID    string    `json:"account_id"`
	FileName     string    `json:"file_name"`
	Currency     Currency  `json:"currency"`
	ContentHash  string    `json:"content_hash"`
	RowCount     int       `json:"row_count"`
	TotalDebits  string    `json:"total_debits"`
	TotalCredits string    `json:"total_credits"`
	CreatedAt    time.Time `json:"created_at"`
}

type StatementDetail struct {
	Import StatementImport  `json:"import"`
	Rows   []StatementEntry `json:"rows"`
}
