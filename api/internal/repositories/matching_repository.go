package repositories

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

type MatchingRepository struct{ pool *pgxpool.Pool }

func NewMatchingRepository(pool *pgxpool.Pool) *MatchingRepository { return &MatchingRepository{pool} }

func matchingImport(ctx context.Context, tx pgx.Tx, owner, id string, lock bool) (models.MatchingReport, error) {
	var out models.MatchingReport
	v := &out.Import
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, `SELECT `+statementColumns+`,direction_mode,matching_version FROM statement_imports WHERE id=$1 AND owner_user_id=$2`+suffix, id, owner).Scan(
		&v.ID, &v.OwnerID, &v.AttachmentID, &v.AccountID, &v.FileName, &v.Currency, &v.ContentHash, &v.RowCount, &v.TotalDebits, &v.TotalCredits, &v.CreatedAt, &out.DirectionMode, &out.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, models.ErrStatementNotFound
	}
	return out, err
}

const matchingRowColumns = `sr.row_number,sr.booked_date::text,sr.reference,sr.amount,sr.direction,sr.description,m.posting_id,n.note,
 (SELECT count(*)::int FROM statement_rejections x WHERE x.import_id=sr.import_id AND x.row_number=sr.row_number)`
const matchingRowFrom = ` FROM statement_rows sr LEFT JOIN statement_matches m ON (m.import_id,m.row_number)=(sr.import_id,sr.row_number)
 LEFT JOIN statement_row_notes n ON (n.import_id,n.row_number)=(sr.import_id,sr.row_number)`

func scanMatchingRow(row pgx.Row, currency models.Currency) (models.MatchingRow, error) {
	r := models.MatchingRow{Entry: models.StatementEntry{Currency: currency}, Candidates: []models.StatementCandidate{}}
	err := row.Scan(&r.RowNumber, &r.Entry.BookedDate, &r.Entry.Reference, &r.Entry.Amount, &r.Entry.Direction, &r.Entry.Description, &r.MatchedPostingID, &r.Note, &r.RejectedCount)
	return r, err
}

func (r *MatchingRepository) Report(ctx context.Context, owner, id string, limit, offset int) (models.MatchingReport, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return models.MatchingReport{}, err
	}
	defer tx.Rollback(context.Background())
	out, err := matchingImport(ctx, tx, owner, id, false)
	if err != nil {
		return out, err
	}
	out.Limit = limit
	out.Offset = offset
	out.Rows = []models.MatchingRow{}
	rows, err := tx.Query(ctx, `SELECT `+matchingRowColumns+matchingRowFrom+` WHERE sr.import_id=$1 ORDER BY sr.row_number LIMIT $2 OFFSET $3`, id, limit, offset)
	if err != nil {
		return out, err
	}
	indexes := map[int]int{}
	numbers := []int32{}
	for rows.Next() {
		row, scanErr := scanMatchingRow(rows, out.Import.Currency)
		if scanErr != nil {
			rows.Close()
			return out, scanErr
		}
		indexes[row.RowNumber] = len(out.Rows)
		if row.RowNumber < 0 || row.RowNumber > math.MaxInt32 {
			rows.Close()
			return out, fmt.Errorf("statement row number %d out of range", row.RowNumber)
		}
		numbers = append(numbers, int32(row.RowNumber))
		out.Rows = append(out.Rows, row)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	if out.DirectionMode != nil && len(numbers) > 0 {
		// The lateral limit bounds the response, while count(*) retains ambiguity
		// information even when more than five candidates meet the exact criteria.
		candidates, queryErr := tx.Query(ctx, `SELECT sr.row_number,c.id,c.journal_entry_id,c.reference_id,c.amount,c.direction,c.posted_at,c.candidate_count
 FROM statement_rows sr JOIN LATERAL (
 SELECT p.id,p.journal_entry_id,j.reference_id,p.amount,p.direction,j.posted_at,count(*) OVER()::int AS candidate_count
 FROM ledger_postings p JOIN journal_entries j ON j.id=p.journal_entry_id
 WHERE p.account_id=$2 AND j.currency=$3 AND p.amount=sr.amount AND j.reference_id=sr.reference
 AND (j.posted_at AT TIME ZONE 'UTC')::date=sr.booked_date
 AND p.direction=CASE WHEN $4::text='OPPOSITE' THEN CASE WHEN sr.direction='DEBIT' THEN 'CREDIT' ELSE 'DEBIT' END ELSE sr.direction END
 AND NOT EXISTS(SELECT 1 FROM statement_matches used WHERE used.posting_id=p.id)
 AND NOT EXISTS(SELECT 1 FROM statement_rejections rejected WHERE rejected.import_id=sr.import_id AND rejected.row_number=sr.row_number AND rejected.posting_id=p.id)
 ORDER BY p.id LIMIT 5
 ) c ON true WHERE sr.import_id=$1 AND sr.row_number=ANY($5)
 AND NOT EXISTS(SELECT 1 FROM statement_matches done WHERE done.import_id=sr.import_id AND done.row_number=sr.row_number)
 ORDER BY sr.row_number,c.id`, id, out.Import.AccountID, out.Import.Currency, string(*out.DirectionMode), numbers)
		if queryErr != nil {
			return out, queryErr
		}
		for candidates.Next() {
			var number, count int
			var c models.StatementCandidate
			if err = candidates.Scan(&number, &c.PostingID, &c.JournalID, &c.Reference, &c.Amount, &c.Direction, &c.PostedAt, &count); err != nil {
				candidates.Close()
				return out, err
			}
			index, ok := indexes[number]
			if !ok {
				continue
			}
			out.Rows[index].Candidates = append(out.Rows[index].Candidates, c)
			out.Rows[index].CandidateCount = count
		}
		candidates.Close()
		if err = candidates.Err(); err != nil {
			return out, err
		}
	}
	return out, tx.Commit(ctx)
}

func matchingAudit(ctx context.Context, tx pgx.Tx, owner, id string, row int, action audit.Action, reason string) error {
	var email string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id=$1 AND is_active`, owner).Scan(&email); errors.Is(err, pgx.ErrNoRows) {
		return models.ErrStatementNotFound
	} else if err != nil {
		return err
	}
	return insertAudit(ctx, tx, audit.Event{Action: action, ActorID: &owner, ActorEmail: &email, Details: audit.Details{Reason: "statement:" + id + "; row:" + strconv.Itoa(row) + "; " + reason}})
}
func bumpMatchingVersion(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `UPDATE statement_imports SET matching_version=matching_version+1 WHERE id=$1`, id)
	return err
}

func (r *MatchingRepository) Configure(ctx context.Context, owner, id string, req models.MatchingConfigRequest) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	report, err := matchingImport(ctx, tx, owner, id, true)
	if err != nil {
		return err
	}
	if report.Version != req.ExpectedVersion {
		return models.ErrMatchStale
	}
	if report.DirectionMode != nil && *report.DirectionMode == req.DirectionMode {
		return nil
	}
	var hasMatches bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM statement_matches WHERE import_id=$1)`, id).Scan(&hasMatches); err != nil {
		return err
	}
	if hasMatches {
		return models.ErrMatchModeLocked
	}
	if _, err = tx.Exec(ctx, `UPDATE statement_imports SET direction_mode=$2,matching_version=matching_version+1 WHERE id=$1`, id, req.DirectionMode); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM statement_rejections WHERE import_id=$1`, id); err != nil {
		return err
	}
	if err = matchingAudit(ctx, tx, owner, id, 0, audit.StatementMatchingConfigured, "mode:"+string(req.DirectionMode)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *MatchingRepository) Review(ctx context.Context, owner, id string, number int, req models.MatchingReviewRequest, validate func(models.MatchingDecisionContext) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	report, err := matchingImport(ctx, tx, owner, id, true)
	if err != nil {
		return err
	}
	if report.Version != req.ExpectedVersion {
		return models.ErrMatchStale
	}
	row, err := scanMatchingRow(tx.QueryRow(ctx, `SELECT `+matchingRowColumns+matchingRowFrom+` WHERE sr.import_id=$1 AND sr.row_number=$2`, id, number), report.Import.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ErrStatementNotFound
	}
	if err != nil {
		return err
	}
	snapshot := models.MatchingDecisionContext{DirectionMode: report.DirectionMode, AccountID: report.Import.AccountID, Row: row}
	if req.PostingID != nil {
		var c models.StatementCandidate
		err = tx.QueryRow(ctx, `SELECT p.id,p.journal_entry_id,j.reference_id,p.amount,p.direction,j.posted_at,p.account_id,j.currency
 FROM ledger_postings p JOIN journal_entries j ON j.id=p.journal_entry_id WHERE p.id=$1
 AND NOT EXISTS(SELECT 1 FROM statement_matches used WHERE used.posting_id=p.id)`, *req.PostingID).Scan(&c.PostingID, &c.JournalID, &c.Reference, &c.Amount, &c.Direction, &c.PostedAt, &c.AccountID, &c.Currency)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			snapshot.Candidate = &c
		}
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM statement_rejections WHERE import_id=$1 AND row_number=$2 AND posting_id=$3)`, id, number, *req.PostingID).Scan(&snapshot.PreviouslyRejected); err != nil {
			return err
		}
	}
	if err = validate(snapshot); err != nil {
		return err
	}
	var action audit.Action
	switch req.Action {
	case models.MatchConfirm:
		action = audit.StatementMatchConfirmed
		_, err = tx.Exec(ctx, `INSERT INTO statement_matches(import_id,row_number,posting_id,matched_by) VALUES($1,$2,$3,$4)`, id, number, req.PostingID, owner)
	case models.MatchReject:
		action = audit.StatementMatchRejected
		_, err = tx.Exec(ctx, `INSERT INTO statement_rejections(import_id,row_number,posting_id,rejected_by,reason) VALUES($1,$2,$3,$4,$5)`, id, number, req.PostingID, owner, req.Reason)
	case models.MatchReset:
		action = audit.StatementMatchReset
		if _, err = tx.Exec(ctx, `DELETE FROM statement_matches WHERE import_id=$1 AND row_number=$2`, id, number); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM statement_rejections WHERE import_id=$1 AND row_number=$2`, id, number)
	case models.MatchNote:
		action = audit.StatementExceptionNoted
		_, err = tx.Exec(ctx, `INSERT INTO statement_row_notes(import_id,row_number,note,updated_by) VALUES($1,$2,$3,$4)
 ON CONFLICT(import_id,row_number) DO UPDATE SET note=EXCLUDED.note,updated_by=EXCLUDED.updated_by,updated_at=now()`, id, number, req.Reason, owner)
	default:
		return models.ErrMatchInvalid
	}
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return models.ErrMatchCandidate
		}
		return err
	}
	if err = bumpMatchingVersion(ctx, tx, id); err != nil {
		return err
	}
	posting := ""
	if req.PostingID != nil {
		posting = "posting:" + *req.PostingID + "; "
	}
	if req.Action == models.MatchReset && row.MatchedPostingID != nil {
		posting = "released_posting:" + *row.MatchedPostingID + "; "
	}
	if err = matchingAudit(ctx, tx, owner, id, number, action, posting+req.Reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
