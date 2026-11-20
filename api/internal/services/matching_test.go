package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/models"
)

const matchTestImport = "11111111-1111-4111-8111-111111111111"
const matchTestPosting = "22222222-2222-4222-8222-222222222222"

type matchingFake struct {
	MatchingStore
	report   models.MatchingReport
	snapshot models.MatchingDecisionContext
	writes   int
}

func (f *matchingFake) Report(context.Context, string, string, int, int) (models.MatchingReport, error) {
	return f.report, nil
}
func (f *matchingFake) Configure(context.Context, string, string, models.MatchingConfigRequest) error {
	f.writes++
	return nil
}
func (f *matchingFake) Review(_ context.Context, _, _ string, _ int, _ models.MatchingReviewRequest, validate func(models.MatchingDecisionContext) error) error {
	if err := validate(f.snapshot); err != nil {
		return err
	}
	f.writes++
	return nil
}

func TestMatchingClassification(t *testing.T) {
	mode := models.StatementDirectionOpposite
	posting := matchTestPosting
	for _, tc := range []struct {
		name string
		mode *models.StatementDirectionMode
		row  models.MatchingRow
		want models.MatchStatus
	}{
		{"pending", nil, models.MatchingRow{}, models.MatchPending},
		{"unmatched", &mode, models.MatchingRow{}, models.MatchUnmatched},
		{"suggested", &mode, models.MatchingRow{CandidateCount: 1}, models.MatchSuggested},
		{"ambiguous", &mode, models.MatchingRow{CandidateCount: 8}, models.MatchAmbiguous},
		{"rejected", &mode, models.MatchingRow{RejectedCount: 1}, models.MatchRejected},
		{"alternative remains", &mode, models.MatchingRow{CandidateCount: 1, RejectedCount: 2}, models.MatchSuggested},
		{"confirmed", &mode, models.MatchingRow{MatchedPostingID: &posting}, models.MatchConfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &matchingFake{report: models.MatchingReport{DirectionMode: tc.mode, Rows: []models.MatchingRow{tc.row}}}
			got, err := NewMatchingService(f).Report(context.Background(), "owner", matchTestImport, 50, 0)
			if err != nil || got.Rows[0].Status != tc.want {
				t.Fatalf("got %+v %v want %s", got, err, tc.want)
			}
		})
	}
}

func TestMatchingConfirmationValidatesEveryCriterionInsideTransaction(t *testing.T) {
	mode := models.StatementDirectionOpposite
	original := models.StatementCandidate{PostingID: matchTestPosting, AccountID: "account", Currency: models.CurrencyUSD, Reference: "BANK-1", Amount: 1250, Direction: models.Debit, PostedAt: time.Date(2026, 11, 20, 23, 0, 0, 0, time.UTC)}
	entry := models.StatementEntry{BookedDate: "2026-11-20", Reference: "BANK-1", Currency: models.CurrencyUSD, Amount: 1250, Direction: models.Credit}
	for _, tc := range []struct {
		name   string
		mutate func(*models.MatchingDecisionContext)
		valid  bool
	}{
		{"exact", func(*models.MatchingDecisionContext) {}, true},
		{"account", func(v *models.MatchingDecisionContext) { v.Candidate.AccountID = "another" }, false},
		{"currency", func(v *models.MatchingDecisionContext) { v.Candidate.Currency = models.CurrencyEUR }, false},
		{"amount", func(v *models.MatchingDecisionContext) { v.Candidate.Amount = 1251 }, false},
		{"reference", func(v *models.MatchingDecisionContext) { v.Candidate.Reference = "bank-1" }, false},
		{"direction", func(v *models.MatchingDecisionContext) { v.Candidate.Direction = models.Credit }, false},
		{"UTC date", func(v *models.MatchingDecisionContext) {
			v.Candidate.PostedAt = time.Date(2026, 11, 20, 23, 30, 0, 0, time.FixedZone("bank", -2*3600))
		}, false},
		{"mapping missing", func(v *models.MatchingDecisionContext) { v.DirectionMode = nil }, false},
		{"mapping invalid", func(v *models.MatchingDecisionContext) {
			bad := models.StatementDirectionMode("UNKNOWN")
			v.DirectionMode = &bad
		}, false},
		{"already matched", func(v *models.MatchingDecisionContext) { id := matchTestPosting; v.Row.MatchedPostingID = &id }, false},
		{"already rejected", func(v *models.MatchingDecisionContext) { v.PreviouslyRejected = true }, false},
		{"posting unavailable", func(v *models.MatchingDecisionContext) { v.Candidate = nil }, false},
		{"same mapping", func(v *models.MatchingDecisionContext) {
			same := models.StatementDirectionSame
			v.DirectionMode = &same
			v.Candidate.Direction = models.Credit
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := original
			f := &matchingFake{snapshot: models.MatchingDecisionContext{DirectionMode: &mode, AccountID: "account", Row: models.MatchingRow{Entry: entry}, Candidate: &c}}
			tc.mutate(&f.snapshot)
			posting := matchTestPosting
			err := NewMatchingService(f).Review(context.Background(), "owner", matchTestImport, 2, models.MatchingReviewRequest{Action: models.MatchConfirm, PostingID: &posting})
			if tc.valid {
				if err != nil || f.writes != 1 {
					t.Fatalf("valid match rejected %v", err)
				}
			} else if !errors.Is(err, models.ErrMatchCandidate) || f.writes != 0 {
				t.Fatalf("invalid match persisted %v", err)
			}
		})
	}
}

func TestMatchingInvalidRequestsNeverReachStore(t *testing.T) {
	posting := matchTestPosting
	for _, req := range []models.MatchingReviewRequest{
		{Action: models.MatchAction("INVALID")}, {Action: models.MatchConfirm}, {Action: models.MatchReject, PostingID: &posting, Reason: "x"},
		{Action: models.MatchNote, PostingID: &posting, Reason: "not allowed"}, {Action: models.MatchReset, Reason: ""},
		{Action: models.MatchNote, Reason: "bad\x00note"}, {Action: models.MatchNote, Reason: "valid reason", ExpectedVersion: -1},
	} {
		f := &matchingFake{}
		if err := NewMatchingService(f).Review(context.Background(), "owner", matchTestImport, 2, req); !errors.Is(err, models.ErrMatchInvalid) || f.writes != 0 {
			t.Fatalf("invalid input persisted %+v %v", req, err)
		}
	}
	f := &matchingFake{}
	s := NewMatchingService(f)
	if err := s.Configure(context.Background(), "owner", matchTestImport, models.MatchingConfigRequest{DirectionMode: models.StatementDirectionMode("UNKNOWN")}); !errors.Is(err, models.ErrMatchInvalid) || f.writes != 0 {
		t.Fatal("unknown mapping accepted")
	}
	for _, action := range []models.MatchAction{models.MatchReset, models.MatchNote} {
		if err := s.Review(context.Background(), "owner", matchTestImport, 2, models.MatchingReviewRequest{Action: action, Reason: "Investigate fee"}); err != nil {
			t.Fatal(err)
		}
	}
}
