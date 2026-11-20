package model

import (
	"errors"
	"time"
)

type StatementDirectionMode string

const (
	StatementDirectionSame     StatementDirectionMode = "SAME"
	StatementDirectionOpposite StatementDirectionMode = "OPPOSITE"
)

func (m StatementDirectionMode) Valid() bool {
	return m == StatementDirectionSame || m == StatementDirectionOpposite
}
func (m StatementDirectionMode) LedgerDirection(d Direction) Direction {
	if m == StatementDirectionOpposite {
		return d.Opposite()
	}
	return d
}

type MatchStatus string

const (
	MatchPending   MatchStatus = "PENDING"
	MatchUnmatched MatchStatus = "UNMATCHED"
	MatchSuggested MatchStatus = "SUGGESTED"
	MatchAmbiguous MatchStatus = "AMBIGUOUS"
	MatchConfirmed MatchStatus = "MATCHED"
	MatchRejected  MatchStatus = "REJECTED"
)

type MatchAction string

const (
	MatchConfirm MatchAction = "CONFIRM"
	MatchReject  MatchAction = "REJECT"
	MatchReset   MatchAction = "RESET"
	MatchNote    MatchAction = "NOTE"
)

var (
	ErrMatchInvalid    = errors.New("invalid matching request")
	ErrMatchStale      = errors.New("matching review changed; reload it")
	ErrMatchCandidate  = errors.New("posting does not satisfy the matching criteria or is no longer available")
	ErrMatchModeLocked = errors.New("reset confirmed matches before changing direction mapping")
)

type MatchingConfigRequest struct {
	DirectionMode   StatementDirectionMode `json:"direction_mode" binding:"required,oneof=SAME OPPOSITE"`
	ExpectedVersion int                    `json:"expected_version" binding:"gte=0,lte=2147483646"`
}
type MatchingReviewRequest struct {
	Action          MatchAction `json:"action" binding:"required,oneof=CONFIRM REJECT RESET NOTE"`
	PostingID       *string     `json:"posting_id" binding:"omitempty,uuid"` // required for CONFIRM/REJECT only
	Reason          string      `json:"reason" binding:"max=500"`
	ExpectedVersion int         `json:"expected_version" binding:"gte=0,lte=2147483646"`
}
type StatementCandidate struct {
	PostingID string    `json:"posting_id"`
	JournalID string    `json:"journal_id"`
	Reference string    `json:"reference"`
	Amount    Amount    `json:"amount"`
	Direction Direction `json:"direction"`
	PostedAt  time.Time `json:"posted_at"`
	AccountID string    `json:"-"`
	Currency  Currency  `json:"-"`
}
type MatchingRow struct {
	RowNumber        int                  `json:"row_number"`
	Entry            StatementEntry       `json:"entry"`
	Status           MatchStatus          `json:"status"`
	MatchedPostingID *string              `json:"matched_posting_id"` // null until confirmed
	Note             *string              `json:"note"`               // latest review note, if any
	RejectedCount    int                  `json:"rejected_count"`
	CandidateCount   int                  `json:"candidate_count"`
	Candidates       []StatementCandidate `json:"candidates"` // first five available exact candidates
}
type MatchingReport struct {
	Import        StatementImport         `json:"import"`
	DirectionMode *StatementDirectionMode `json:"direction_mode"` // null until configured
	Version       int                     `json:"version"`
	Rows          []MatchingRow           `json:"rows"`
	Limit         int                     `json:"limit"`
	Offset        int                     `json:"offset"`
}

// MatchingDecisionContext is read under the import lock. A validator supplied
// by the service evaluates business rules inside the persistence transaction.
type MatchingDecisionContext struct {
	DirectionMode      *StatementDirectionMode
	AccountID          string
	Row                MatchingRow
	Candidate          *StatementCandidate
	PreviouslyRejected bool
}
