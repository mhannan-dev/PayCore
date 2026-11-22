package repositories

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
)

var ErrSettlementNotFound = fmt.Errorf("%w: %w", models.ErrPayoutNotFound, pgx.ErrNoRows)

type SettlementRepository struct{ pool *pgxpool.Pool }

func NewSettlementRepository(pool *pgxpool.Pool) *SettlementRepository {
	return &SettlementRepository{pool: pool}
}

const settlementColumns = `s.id, s.payout_reference, s.source_account_id, a_src.name,
 s.clearing_account_id, a_clr.name, s.destination_account_id,
 s.amount, s.currency, s.status, s.provider_mode, s.provider_reference,
 s.beneficiary_name, s.beneficiary_bank, s.beneficiary_account,
 s.reserve_journal_id, s.capture_journal_id, s.release_journal_id,
 s.created_by, s.created_at, s.updated_at`
const settlementFrom = ` FROM settlements s
 JOIN accounts a_src ON a_src.id = s.source_account_id
 JOIN accounts a_clr ON a_clr.id = s.clearing_account_id`

func scanSettlement(row pgx.Row) (models.Settlement, error) {
	var s models.Settlement
	var amount int64
	err := row.Scan(&s.ID, &s.PayoutReference, &s.SourceAccountID, &s.SourceAccountName,
		&s.ClearingAccountID, &s.ClearingAccountName, &s.DestinationAccountID,
		&amount, &s.Currency, &s.Status, &s.ProviderMode, &s.ProviderReference,
		&s.BeneficiaryName, &s.BeneficiaryBank, &s.BeneficiaryAccount,
		&s.ReserveJournalID, &s.CaptureJournalID, &s.ReleaseJournalID,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	s.Amount = models.Amount(amount)
	return s, notFound(err, ErrSettlementNotFound)
}
func (r *SettlementRepository) GetByID(ctx context.Context, id string) (models.Settlement, error) {
	return scanSettlement(r.pool.QueryRow(ctx, "SELECT "+settlementColumns+settlementFrom+" WHERE s.id = $1", id))
}
func (r *SettlementRepository) List(ctx context.Context, q models.SettlementQuery) ([]models.Settlement, int, error) {
	const where = ` WHERE ($1 = '' OR s.currency = $1)`
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM settlements s"+where, string(q.Currency)).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+settlementColumns+settlementFrom+where+" ORDER BY s.created_at DESC, s.id DESC LIMIT $2 OFFSET $3", string(q.Currency), q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []models.Settlement{}
	for rows.Next() {
		item, err := scanSettlement(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}
func (r *SettlementRepository) Execute(ctx context.Context, clientID, key, hash string,
	op func(context.Context, models.SettlementWriter) (int, []byte, error)) (models.IdempotentResult, error) {
	return runIdempotent(ctx, r.pool, clientID, key, hash, func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		return op(ctx, &settlementWriter{ledgerWriter{tx: tx}})
	})
}

type settlementWriter struct{ ledgerWriter }

func (w *settlementWriter) GetSettlement(ctx context.Context, id string) (models.Settlement, error) {
	return scanSettlement(w.tx.QueryRow(ctx, "SELECT "+settlementColumns+settlementFrom+" WHERE s.id = $1 FOR UPDATE OF s", id))
}
func (w *settlementWriter) Accounts(ctx context.Context, currency models.Currency) ([]models.Account, error) {
	rows, err := w.tx.Query(ctx, `SELECT id, account_number, name, type, currency, is_active FROM accounts WHERE currency = $1 ORDER BY account_number`, string(currency))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []models.Account{}
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.AccountNumber, &a.Name, &a.Type, &a.Currency, &a.IsActive); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}
func (w *settlementWriter) CreateSettlement(ctx context.Context, s models.Settlement) error {
	_, err := w.tx.Exec(ctx, `INSERT INTO settlements (
 id, payout_reference, source_account_id, clearing_account_id, amount, currency, status,
 provider_mode, provider_reference, beneficiary_name, beneficiary_bank, beneficiary_account,
 reserve_journal_id, created_by, created_at, updated_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		s.ID, s.PayoutReference, s.SourceAccountID, s.ClearingAccountID, int64(s.Amount), s.Currency, s.Status,
		s.ProviderMode, s.ProviderReference, s.BeneficiaryName, s.BeneficiaryBank, s.BeneficiaryAccount,
		s.ReserveJournalID, s.CreatedBy, s.CreatedAt, s.UpdatedAt)
	return err
}
func (w *settlementWriter) UpdateSettlement(ctx context.Context, s models.Settlement) error {
	tag, err := w.tx.Exec(ctx, `UPDATE settlements SET status = $2, destination_account_id = $3,
 capture_journal_id = $4, release_journal_id = $5, updated_at = $6 WHERE id = $1 AND status = 'RESERVED'`,
		s.ID, s.Status, s.DestinationAccountID, s.CaptureJournalID, s.ReleaseJournalID, s.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return models.ErrPayoutState
	}
	return nil
}
