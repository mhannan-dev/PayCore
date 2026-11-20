package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"paycore/api/internal/model"
)

const matchTestImport = "11111111-1111-4111-8111-111111111111"
const matchTestPosting = "22222222-2222-4222-8222-222222222222"

type matchingFake struct {
	MatchingStore
	report   model.MatchingReport
	snapshot model.MatchingDecisionContext
	writes   int
}

func (f *matchingFake) Report(context.Context, string, string, int, int) (model.MatchingReport, error) {
	return f.report, nil
}
func (f *matchingFake) Configure(context.Context, string, string, model.MatchingConfigRequest) error {
	f.writes++
	return nil
}
func (f *matchingFake) Review(_ context.Context, _, _ string, _ int, _ model.MatchingReviewRequest, validate func(model.MatchingDecisionContext) error) error {
	if err := validate(f.snapshot); err != nil {
		return err
	}
	f.writes++
	return nil
}

func TestMatchingClassification(t *testing.T) {
	mode := model.StatementDirectionOpposite
	posting := matchTestPosting
	for _, tc := range []struct {
		name string
		mode *model.StatementDirectionMode
		row  model.MatchingRow
		want model.MatchStatus
	}{
		{"pending", nil, model.MatchingRow{}, model.MatchPending},
		{"unmatched", &mode, model.MatchingRow{}, model.MatchUnmatched},
		{"suggested", &mode, model.MatchingRow{CandidateCount: 1}, model.MatchSuggested},
		{"ambiguous", &mode, model.MatchingRow{CandidateCount: 8}, model.MatchAmbiguous},
		{"rejected", &mode, model.MatchingRow{RejectedCount: 1}, model.MatchRejected},
		{"alternative remains", &mode, model.MatchingRow{CandidateCount: 1, RejectedCount: 2}, model.MatchSuggested},
		{"confirmed", &mode, model.MatchingRow{MatchedPostingID: &posting}, model.MatchConfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &matchingFake{report: model.MatchingReport{DirectionMode: tc.mode, Rows: []model.MatchingRow{tc.row}}}
			got, err := NewMatchingService(f).Report(context.Background(), "owner", matchTestImport, 50, 0)
			if err != nil || got.Rows[0].Status != tc.want {
				t.Fatalf("got %+v %v want %s", got, err, tc.want)
			}
		})
	}
}

func TestMatchingConfirmationValidatesEveryCriterionInsideTransaction(t *testing.T) {
	mode := model.StatementDirectionOpposite
	original := model.StatementCandidate{PostingID: matchTestPosting, AccountID: "account", Currency: model.CurrencyUSD, Reference: "BANK-1", Amount: 1250, Direction: model.Debit, PostedAt: time.Date(2026, 11, 20, 23, 0, 0, 0, time.UTC)}
	entry := model.StatementEntry{BookedDate: "2026-11-20", Reference: "BANK-1", Currency: model.CurrencyUSD, Amount: 1250, Direction: model.Credit}
	for _, tc := range []struct {
		name   string
		mutate func(*model.MatchingDecisionContext)
		valid  bool
	}{
		{"exact", func(*model.MatchingDecisionContext) {}, true},
		{"account", func(v *model.MatchingDecisionContext) { v.Candidate.AccountID = "another" }, false},
		{"currency", func(v *model.MatchingDecisionContext) { v.Candidate.Currency = model.CurrencyEUR }, false},
		{"amount", func(v *model.MatchingDecisionContext) { v.Candidate.Amount = 1251 }, false},
		{"reference", func(v *model.MatchingDecisionContext) { v.Candidate.Reference = "bank-1" }, false},
		{"direction", func(v *model.MatchingDecisionContext) { v.Candidate.Direction = model.Credit }, false},
		{"UTC date", func(v *model.MatchingDecisionContext) {
			v.Candidate.PostedAt = time.Date(2026, 11, 20, 23, 30, 0, 0, time.FixedZone("bank", -2*3600))
		}, false},
		{"mapping missing", func(v *model.MatchingDecisionContext) { v.DirectionMode = nil }, false},
		{"mapping invalid", func(v *model.MatchingDecisionContext) {
			bad := model.StatementDirectionMode("UNKNOWN")
			v.DirectionMode = &bad
		}, false},
		{"already matched", func(v *model.MatchingDecisionContext) { id := matchTestPosting; v.Row.MatchedPostingID = &id }, false},
		{"already rejected", func(v *model.MatchingDecisionContext) { v.PreviouslyRejected = true }, false},
		{"posting unavailable", func(v *model.MatchingDecisionContext) { v.Candidate = nil }, false},
		{"same mapping", func(v *model.MatchingDecisionContext) {
			same := model.StatementDirectionSame
			v.DirectionMode = &same
			v.Candidate.Direction = model.Credit
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := original
			f := &matchingFake{snapshot: model.MatchingDecisionContext{DirectionMode: &mode, AccountID: "account", Row: model.MatchingRow{Entry: entry}, Candidate: &c}}
			tc.mutate(&f.snapshot)
			posting := matchTestPosting
			err := NewMatchingService(f).Review(context.Background(), "owner", matchTestImport, 2, model.MatchingReviewRequest{Action: model.MatchConfirm, PostingID: &posting})
			if tc.valid {
				if err != nil || f.writes != 1 {
					t.Fatalf("valid match rejected %v", err)
				}
			} else if !errors.Is(err, model.ErrMatchCandidate) || f.writes != 0 {
				t.Fatalf("invalid match persisted %v", err)
			}
		})
	}
}

func TestMatchingInvalidRequestsNeverReachStore(t *testing.T) {
	posting := matchTestPosting
	for _, req := range []model.MatchingReviewRequest{
		{Action: model.MatchAction("INVALID")}, {Action: model.MatchConfirm}, {Action: model.MatchReject, PostingID: &posting, Reason: "x"},
		{Action: model.MatchNote, PostingID: &posting, Reason: "not allowed"}, {Action: model.MatchReset, Reason: ""},
		{Action: model.MatchNote, Reason: "bad\x00note"}, {Action: model.MatchNote, Reason: "valid reason", ExpectedVersion: -1},
	} {
		f := &matchingFake{}
		if err := NewMatchingService(f).Review(context.Background(), "owner", matchTestImport, 2, req); !errors.Is(err, model.ErrMatchInvalid) || f.writes != 0 {
			t.Fatalf("invalid input persisted %+v %v", req, err)
		}
	}
	f := &matchingFake{}
	s := NewMatchingService(f)
	if err := s.Configure(context.Background(), "owner", matchTestImport, model.MatchingConfigRequest{DirectionMode: model.StatementDirectionMode("UNKNOWN")}); !errors.Is(err, model.ErrMatchInvalid) || f.writes != 0 {
		t.Fatal("unknown mapping accepted")
	}
	for _, action := range []model.MatchAction{model.MatchReset, model.MatchNote} {
		if err := s.Review(context.Background(), "owner", matchTestImport, 2, model.MatchingReviewRequest{Action: action, Reason: "Investigate fee"}); err != nil {
			t.Fatal(err)
		}
	}
}
