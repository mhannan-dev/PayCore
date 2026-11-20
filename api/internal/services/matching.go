package services

import (
	"context"
	"strings"
	"unicode/utf8"

	"paycore/api/internal/models"
)

type MatchingStore interface {
	Report(context.Context, string, string, int, int) (models.MatchingReport, error)
	Configure(context.Context, string, string, models.MatchingConfigRequest) error
	Review(context.Context, string, string, int, models.MatchingReviewRequest, func(models.MatchingDecisionContext) error) error
}
type MatchingService struct{ store MatchingStore }

func NewMatchingService(store MatchingStore) *MatchingService { return &MatchingService{store} }

func (s *MatchingService) Report(ctx context.Context, owner, id string, limit, offset int) (models.MatchingReport, error) {
	if owner == "" || !models.ValidUUID(id) || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return models.MatchingReport{}, models.ErrMatchInvalid
	}
	report, err := s.store.Report(ctx, owner, id, limit, offset)
	if err != nil {
		return report, err
	}
	for i := range report.Rows {
		r := &report.Rows[i]
		switch {
		case r.MatchedPostingID != nil:
			r.Status = models.MatchConfirmed
		case report.DirectionMode == nil:
			r.Status = models.MatchPending
		case r.CandidateCount > 1:
			r.Status = models.MatchAmbiguous
		case r.CandidateCount == 1:
			r.Status = models.MatchSuggested
		case r.RejectedCount > 0:
			r.Status = models.MatchRejected
		default:
			r.Status = models.MatchUnmatched
		}
	}
	return report, nil
}
func (s *MatchingService) Configure(ctx context.Context, owner, id string, req models.MatchingConfigRequest) error {
	if owner == "" || !models.ValidUUID(id) || !req.DirectionMode.Valid() || req.ExpectedVersion < 0 || req.ExpectedVersion >= 2147483647 {
		return models.ErrMatchInvalid
	}
	return s.store.Configure(ctx, owner, id, req)
}
func (s *MatchingService) Review(ctx context.Context, owner, id string, row int, req models.MatchingReviewRequest) error {
	req.Reason = strings.TrimSpace(req.Reason)
	if owner == "" || !models.ValidUUID(id) || row < 1 || req.ExpectedVersion < 0 || req.ExpectedVersion >= 2147483647 || !statementText(req.Reason, 500, true) {
		return models.ErrMatchInvalid
	}
	switch req.Action {
	case models.MatchConfirm, models.MatchReject:
		if req.PostingID == nil || !models.ValidUUID(*req.PostingID) {
			return models.ErrMatchInvalid
		}
	case models.MatchReset, models.MatchNote:
		if req.PostingID != nil {
			return models.ErrMatchInvalid
		}
	default:
		return models.ErrMatchInvalid
	}
	if req.Action != models.MatchConfirm && utf8.RuneCountInString(req.Reason) < 3 {
		return models.ErrMatchInvalid
	}
	return s.store.Review(ctx, owner, id, row, req, func(v models.MatchingDecisionContext) error {
		switch req.Action {
		case models.MatchConfirm, models.MatchReject:
			if v.DirectionMode == nil || !v.DirectionMode.Valid() || v.Row.MatchedPostingID != nil || v.PreviouslyRejected || v.Candidate == nil {
				return models.ErrMatchCandidate
			}
			e, c := v.Row.Entry, v.Candidate
			if c.AccountID != v.AccountID || c.Currency != e.Currency || c.Amount != e.Amount || c.Reference != e.Reference || c.Direction != v.DirectionMode.LedgerDirection(e.Direction) || c.PostedAt.UTC().Format("2006-01-02") != e.BookedDate {
				return models.ErrMatchCandidate
			}
		case models.MatchReset, models.MatchNote:
			// Notes and explicit resets are available even on unmatched rows.
		default:
			return models.ErrMatchInvalid
		}
		return nil
	})
}
