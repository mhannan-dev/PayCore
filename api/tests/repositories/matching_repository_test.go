package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

func TestMatchingLifecycleAtomicityAndGlobalPostingUniqueness(t *testing.T) {
	f := newLedgerFixture(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(f.pool)
	owner, err := users.Create(ctx, "Reviewer", nil, "reviewer@example.test", "fixture hash", []string{models.RoleAccountant})
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.Create(ctx, "Other", nil, "reviewer-other@example.test", "fixture hash", []string{models.RoleAccountant})
	if err != nil {
		t.Fatal(err)
	}
	post := func(ref string, legs []models.Leg) models.Journal {
		t.Helper()
		result, err := f.svc.Transfer(ctx, f.actor, f.key(), models.TransferRequest{ReferenceID: ref, Description: "statement fixture", Currency: models.CurrencyJPY, Legs: legs})
		if err != nil {
			t.Fatal(err)
		}
		return decodeJournal(t, result)
	}
	exact := post("BANK-1", []models.Leg{{AccountID: f.bank.ID, Direction: models.Debit, Amount: 1000}, {AccountID: f.alice.ID, Direction: models.Credit, Amount: 1000}})
	ambiguous := post("BANK-2", []models.Leg{{AccountID: f.bank.ID, Direction: models.Debit, Amount: 500}, {AccountID: f.bank.ID, Direction: models.Debit, Amount: 500}, {AccountID: f.alice.ID, Direction: models.Credit, Amount: 1000}})
	legs := []models.Leg{}
	for i := 0; i < 7; i++ {
		legs = append(legs, models.Leg{AccountID: f.bank.ID, Direction: models.Debit, Amount: 100})
	}
	legs = append(legs, models.Leg{AccountID: f.alice.ID, Direction: models.Credit, Amount: 700})
	post("BANK-MANY", legs)
	date := exact.PostedAt.UTC().Format("2006-01-02")
	entry := func(ref string, amount models.Amount) models.StatementEntry {
		return models.StatementEntry{BookedDate: date, Reference: ref, Currency: models.CurrencyJPY, Amount: amount, Direction: models.Credit, Description: "statement row"}
	}
	seq := 0
	importRows := func(user string, entries []models.StatementEntry) string {
		t.Helper()
		seq++
		hash := fmt.Sprintf("%064x", seq)
		var attachment string
		if err := f.pool.QueryRow(ctx, `INSERT INTO attachments(owner_user_id,file_name,content_type,size_bytes,sha256,storage_kind,storage_key) VALUES($1,'matching.csv','text/csv',100,$2,'LOCAL',$3) RETURNING id`, user, hash, hash+".csv").Scan(&attachment); err != nil {
			t.Fatal(err)
		}
		p := models.StatementPreview{AttachmentID: attachment, AccountID: f.bank.ID, FileName: "matching.csv", Currency: models.CurrencyJPY, ContentHash: &hash, TotalDebits: "0"}
		var total int64
		for i, e := range entries {
			e := e
			p.Rows = append(p.Rows, models.StatementRow{RowNumber: i + 2, Entry: &e})
			total += int64(e.Amount)
		}
		p.TotalCredits = strconv.FormatInt(total, 10)
		saved, err := repositories.NewStatementRepository(f.pool).Save(ctx, user, p, hash)
		if err != nil {
			t.Fatal(err)
		}
		return saved.ID
	}
	badDate := entry("BANK-1", 1000)
	badDate.BookedDate = "1900-01-01"
	id := importRows(owner.ID, []models.StatementEntry{entry("BANK-1", 1000), entry("BANK-2", 500), entry("UNKNOWN", 42), badDate, entry("BANK-MANY", 100), entry("BANK-1", 999)})
	otherID := importRows(other.ID, []models.StatementEntry{entry("BANK-1", 1000)})
	svc := services.NewMatchingService(repositories.NewMatchingRepository(f.pool))
	report := func(user, id string) models.MatchingReport {
		t.Helper()
		r, err := svc.Report(ctx, user, id, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	row := func(r models.MatchingReport, number int) models.MatchingRow {
		t.Helper()
		for _, v := range r.Rows {
			if v.RowNumber == number {
				return v
			}
		}
		t.Fatalf("row %d missing", number)
		return models.MatchingRow{}
	}
	current := report(owner.ID, id)
	if row(current, 2).Status != models.MatchPending {
		t.Fatal("matching must require explicit direction mapping")
	}
	if _, err = svc.Report(ctx, other.ID, id, 50, 0); !errors.Is(err, models.ErrStatementNotFound) {
		t.Fatal("foreign import exposed")
	}
	if err = svc.Configure(ctx, other.ID, id, models.MatchingConfigRequest{DirectionMode: models.StatementDirectionOpposite}); !errors.Is(err, models.ErrStatementNotFound) {
		t.Fatal("foreign import configured")
	}
	// Audit failure must also roll back configuration changes.
	if _, err = f.pool.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT reject_matching_audit CHECK(action<>'statement_matching_configured') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if err = svc.Configure(ctx, owner.ID, id, models.MatchingConfigRequest{DirectionMode: models.StatementDirectionOpposite}); err == nil {
		t.Fatal("audit failure accepted")
	}
	current = report(owner.ID, id)
	if current.Version != 0 || current.DirectionMode != nil {
		t.Fatal("configuration partially committed")
	}
	if _, err = f.pool.Exec(ctx, `ALTER TABLE audit_events DROP CONSTRAINT reject_matching_audit`); err != nil {
		t.Fatal(err)
	}
	if err = svc.Configure(ctx, owner.ID, id, models.MatchingConfigRequest{DirectionMode: models.StatementDirectionOpposite}); err != nil {
		t.Fatal(err)
	}
	if err = svc.Configure(ctx, other.ID, otherID, models.MatchingConfigRequest{DirectionMode: models.StatementDirectionOpposite}); err != nil {
		t.Fatal(err)
	}
	current = report(owner.ID, id)
	if row(current, 2).Status != models.MatchSuggested || row(current, 3).Status != models.MatchAmbiguous || row(current, 3).CandidateCount != 2 || row(current, 4).Status != models.MatchUnmatched || row(current, 5).Status != models.MatchUnmatched || row(current, 7).Status != models.MatchUnmatched {
		t.Fatalf("classification %+v", current.Rows)
	}
	many := row(current, 6)
	if many.CandidateCount != 7 || len(many.Candidates) != 5 {
		t.Fatalf("unbounded candidate response %+v", many)
	}
	candidate := row(current, 2).Candidates[0]
	if candidate.PostingID == "" || candidate.JournalID != exact.ID {
		t.Fatal("wrong candidate")
	}
	balance := f.balance(t, f.bank)
	confirm := models.MatchingReviewRequest{Action: models.MatchConfirm, PostingID: &candidate.PostingID, ExpectedVersion: current.Version}
	// A failed audit insert rolls back the match and the optimistic version.
	if _, err = f.pool.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT reject_matching_audit CHECK(action<>'statement_match_confirmed') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if err = svc.Review(ctx, owner.ID, id, 2, confirm); err == nil {
		t.Fatal("match without audit committed")
	}
	after := report(owner.ID, id)
	if after.Version != current.Version || row(after, 2).Status != models.MatchSuggested {
		t.Fatal("match partially committed")
	}
	if _, err = f.pool.Exec(ctx, `ALTER TABLE audit_events DROP CONSTRAINT reject_matching_audit`); err != nil {
		t.Fatal(err)
	}
	if err = svc.Review(ctx, owner.ID, id, 2, confirm); err != nil {
		t.Fatal(err)
	}
	current = report(owner.ID, id)
	if row(current, 2).Status != models.MatchConfirmed {
		t.Fatal("match not persisted")
	}
	if row(report(other.ID, otherID), 2).CandidateCount != 0 {
		t.Fatal("reconciled posting reused across owners")
	}
	if err = svc.Configure(ctx, owner.ID, id, models.MatchingConfigRequest{DirectionMode: models.StatementDirectionSame, ExpectedVersion: current.Version}); !errors.Is(err, models.ErrMatchModeLocked) {
		t.Fatalf("mapping changed beneath confirmed match: %v", err)
	}
	if err = svc.Review(ctx, owner.ID, id, 2, confirm); !errors.Is(err, models.ErrMatchStale) {
		t.Fatalf("stale review accepted: %v", err)
	}
	ambiguousCandidate := row(current, 3).Candidates[0]
	if ambiguousCandidate.JournalID != ambiguous.ID {
		t.Fatal("wrong ambiguous candidate")
	}
	reject := models.MatchingReviewRequest{Action: models.MatchReject, PostingID: &ambiguousCandidate.PostingID, Reason: "Bank evidence differs", ExpectedVersion: current.Version}
	if err = svc.Review(ctx, owner.ID, id, 3, reject); err != nil {
		t.Fatal(err)
	}
	current = report(owner.ID, id)
	if row(current, 3).Status != models.MatchSuggested || row(current, 3).RejectedCount != 1 {
		t.Fatal("rejected candidate still suggested")
	}
	reset := func(number int) {
		t.Helper()
		current = report(owner.ID, id)
		if err = svc.Review(ctx, owner.ID, id, number, models.MatchingReviewRequest{Action: models.MatchReset, Reason: "Reopen after review", ExpectedVersion: current.Version}); err != nil {
			t.Fatal(err)
		}
		current = report(owner.ID, id)
	}
	reset(3)
	if row(current, 3).CandidateCount != 2 || row(current, 3).RejectedCount != 0 {
		t.Fatal("reset failed")
	}
	if err = svc.Review(ctx, owner.ID, id, 4, models.MatchingReviewRequest{Action: models.MatchNote, Reason: "Investigate missing bank fee", ExpectedVersion: current.Version}); err != nil {
		t.Fatal(err)
	}
	current = report(owner.ID, id)
	note := row(current, 4).Note
	if note == nil || *note != "Investigate missing bank fee" {
		t.Fatal("exception note missing")
	}
	reset(2)
	if row(report(other.ID, otherID), 2).CandidateCount != 1 {
		t.Fatal("reset did not release posting")
	}
	// Two imports owned by different users race for the same posting. Unique
	// posting_id enforces global one-to-one reconciliation across import locks.
	otherReport := report(other.ID, otherID)
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for _, attempt := range []struct {
		user, id string
		version  int
	}{{owner.ID, id, current.Version}, {other.ID, otherID, otherReport.Version}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes <- svc.Review(ctx, attempt.user, attempt.id, 2, models.MatchingReviewRequest{Action: models.MatchConfirm, PostingID: &candidate.PostingID, ExpectedVersion: attempt.version})
		}()
	}
	wg.Wait()
	close(outcomes)
	success, conflict := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, models.ErrMatchCandidate) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("race results success=%d conflicts=%d", success, conflict)
	}
	if f.balance(t, f.bank) != balance {
		t.Fatal("matching moved money")
	}
	var journals int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM journal_entries`).Scan(&journals); err != nil || journals != 3 {
		t.Fatalf("matching posted journals %d %v", journals, err)
	}
}
