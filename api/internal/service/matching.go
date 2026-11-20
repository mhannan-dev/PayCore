package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"paycore/api/internal/model"
)

type MatchingStore interface {
	Report(context.Context, string, string, int, int) (model.MatchingReport, error)
	Configure(context.Context, string, string, model.MatchingConfigRequest) error
	Review(context.Context, string, string, int, model.MatchingReviewRequest, func(model.MatchingDecisionContext) error) error
}
type MatchingService struct{ store MatchingStore }

func NewMatchingService(store MatchingStore) *MatchingService { return &MatchingService{store} }

func (s *MatchingService) Report(ctx context.Context, owner, id string, limit, offset int) (model.MatchingReport, error) {
	if owner == "" || !model.ValidUUID(id) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return model.MatchingReport{}, model.ErrMatchInvalid
	}
	report, err := s.store.Report(ctx, owner, id, limit, offset)
	if err != nil {
		return report, err
	}
	for i := range report.Rows {
		r := &report.Rows[i]
		switch {
		case r.MatchedPostingID != nil:
			r.Status = model.MatchConfirmed
		case report.DirectionMode == nil:
			r.Status = model.MatchPending
		case r.CandidateCount > 1:
			r.Status = model.MatchAmbiguous
		case r.CandidateCount == 1:
			r.Status = model.MatchSuggested
		case r.RejectedCount > 0:
			r.Status = model.MatchRejected
		default:
			r.Status = model.MatchUnmatched
		}
	}
	return report, nil
}
func (s *MatchingService) Configure(ctx context.Context, owner, id string, req model.MatchingConfigRequest) error {
	if owner == "" || !model.ValidUUID(id) || !req.DirectionMode.Valid() || req.ExpectedVersion < 0 || req.ExpectedVersion >= 2147483647 {
		return model.ErrMatchInvalid
	}
	return s.store.Configure(ctx, owner, id, req)
}
func (s *MatchingService) Review(ctx context.Context, owner, id string, row int, req model.MatchingReviewRequest) error {
	req.Reason = strings.TrimSpace(req.Reason)
	if owner == "" || !model.ValidUUID(id) || row < 1 || req.ExpectedVersion < 0 || req.ExpectedVersion >= 2147483647 || !statementText(req.Reason, 500, true) {
		return model.ErrMatchInvalid
	}
	switch req.Action {
	case model.MatchConfirm, model.MatchReject:
		if req.PostingID == nil || !model.ValidUUID(*req.PostingID) {
			return model.ErrMatchInvalid
		}
	case model.MatchReset, model.MatchNote:
		if req.PostingID != nil {
			return model.ErrMatchInvalid
		}
	default:
		return model.ErrMatchInvalid
	}
	if req.Action != model.MatchConfirm && utf8.RuneCountInString(req.Reason) < 3 {
		return model.ErrMatchInvalid
	}
	return s.store.Review(ctx, owner, id, row, req, func(v model.MatchingDecisionContext) error {
		switch req.Action {
		case model.MatchConfirm, model.MatchReject:
			if v.DirectionMode == nil || !v.DirectionMode.Valid() || v.Row.MatchedPostingID != nil || v.PreviouslyRejected || v.Candidate == nil {
				return model.ErrMatchCandidate
			}
			e, c := v.Row.Entry, v.Candidate
			if c.AccountID != v.AccountID || c.Currency != e.Currency || c.Amount != e.Amount || c.Reference != e.Reference || c.Direction != v.DirectionMode.LedgerDirection(e.Direction) || c.PostedAt.UTC().Format("2006-01-02") != e.BookedDate {
				return model.ErrMatchCandidate
			}
		case model.MatchReset, model.MatchNote:
			// Notes and explicit resets are available even on unmatched rows.
		default:
			return model.ErrMatchInvalid
		}
		return nil
	})
}
