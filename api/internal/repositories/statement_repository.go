package repositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

type StatementRepository struct{ pool *pgxpool.Pool }

func NewStatementRepository(pool *pgxpool.Pool) *StatementRepository {
	return &StatementRepository{pool}
}

const statementColumns = `id,owner_user_id,attachment_id,account_id,file_name,currency,content_hash,row_count,total_debits::text,total_credits::text,created_at`

func scanStatement(row pgx.Row) (models.StatementImport, error) {
	var v models.StatementImport
	err := row.Scan(&v.ID, &v.OwnerID, &v.AttachmentID, &v.AccountID, &v.FileName, &v.Currency, &v.ContentHash, &v.RowCount, &v.TotalDebits, &v.TotalCredits, &v.CreatedAt)
	return v, err
}
func (r *StatementRepository) Duplicate(ctx context.Context, owner, account, hash string) (*string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM statement_imports WHERE owner_user_id=$1 AND account_id=$2 AND content_hash=$3`, owner, account, hash).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
func (r *StatementRepository) Save(ctx context.Context, owner string, p models.StatementPreview, source string) (models.StatementImport, error) {
	var empty models.StatementImport
	if owner == "" || p.ContentHash == nil || len(p.Rows) == 0 || len(p.Rows) > models.StatementMaxRows || p.InvalidRows != 0 {
		return empty, models.ErrStatementInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(context.Background())
	// Locks keep source deletion and account changes from racing this commit.
	var valid bool
	err = tx.QueryRow(ctx, `SELECT true FROM attachments WHERE id=$1 AND owner_user_id=$2 AND deleted_at IS NULL
 AND sha256=$3 AND storage_kind='LOCAL' AND size_bytes<=$4 FOR SHARE`, p.AttachmentID, owner, source, models.StatementMaxBytes).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, models.ErrStatementChanged
	}
	if err != nil {
		return empty, err
	}
	err = tx.QueryRow(ctx, `SELECT true FROM accounts WHERE id=$1 AND currency=$2 AND type IN ('ASSET','LIABILITY') FOR SHARE`, p.AccountID, p.Currency).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, models.ErrStatementChanged
	}
	if err != nil {
		return empty, err
	}
	var email string
	if err = tx.QueryRow(ctx, `SELECT email FROM users WHERE id=$1 AND is_active`, owner).Scan(&email); errors.Is(err, pgx.ErrNoRows) {
		return empty, models.ErrStatementNotFound
	}
	if err != nil {
		return empty, err
	}
	v, err := scanStatement(tx.QueryRow(ctx, `INSERT INTO statement_imports(owner_user_id,attachment_id,account_id,file_name,currency,content_hash,source_sha256,row_count,total_debits,total_credits)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::numeric,$10::numeric) RETURNING `+statementColumns, owner, p.AttachmentID, p.AccountID, p.FileName, p.Currency, *p.ContentHash, source, len(p.Rows), p.TotalDebits, p.TotalCredits))
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "statement_imports_duplicate_key" {
			return empty, models.ErrStatementDuplicate
		}
		return empty, err
	}
	// Send all row inserts in one batch while keeping them in this transaction.
	batch := &pgx.Batch{}
	for _, row := range p.Rows {
		if row.Entry == nil {
			return empty, models.ErrStatementInvalid
		}
		e := row.Entry
		batch.Queue(`INSERT INTO statement_rows(import_id,row_number,booked_date,reference,amount,direction,description) VALUES($1,$2,$3::text::date,$4,$5,$6,$7)`, v.ID, row.RowNumber, e.BookedDate, e.Reference, int64(e.Amount), string(e.Direction), e.Description)
	}
	result := tx.SendBatch(ctx, batch)
	if err = result.Close(); err != nil {
		return empty, err
	}
	count := int64(v.RowCount)
	if err = insertAudit(ctx, tx, audit.Event{Action: audit.StatementImported, ActorID: &owner, ActorEmail: &email, Details: audit.Details{Reason: "statement_import:" + v.ID, Count: &count}}); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	return v, nil
}
func (r *StatementRepository) List(ctx context.Context, owner string, limit, offset int) ([]models.StatementImport, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+statementColumns+` FROM statement_imports WHERE owner_user_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, owner, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StatementImport{}
	for rows.Next() {
		v, err := scanStatement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *StatementRepository) Detail(ctx context.Context, owner, id string, limit, offset int) (models.StatementDetail, error) {
	out := models.StatementDetail{Rows: []models.StatementEntry{}}
	v, err := scanStatement(r.pool.QueryRow(ctx, `SELECT `+statementColumns+` FROM statement_imports WHERE id=$1 AND owner_user_id=$2`, id, owner))
	if errors.Is(err, pgx.ErrNoRows) {
		return out, models.ErrStatementNotFound
	}
	if err != nil {
		return out, err
	}
	out.Import = v
	rows, err := r.pool.Query(ctx, `SELECT booked_date::text,reference,amount,direction,description FROM statement_rows WHERE import_id=$1 ORDER BY row_number LIMIT $2 OFFSET $3`, v.ID, limit, offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		e := models.StatementEntry{Currency: v.Currency}
		if err = rows.Scan(&e.BookedDate, &e.Reference, &e.Amount, &e.Direction, &e.Description); err != nil {
			return out, err
		}
		out.Rows = append(out.Rows, e)
	}
	return out, rows.Err()
}
