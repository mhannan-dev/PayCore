package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
)

var (
	errQuantumMerchantNotFound = fmt.Errorf("%w: %w", models.ErrQuantumMerchantNotFound, pgx.ErrNoRows)
	errQuantumInvoiceNotFound  = fmt.Errorf("%w: %w", models.ErrQuantumInvoiceNotFound, pgx.ErrNoRows)
	errQuantumCheckoutNotFound = fmt.Errorf("%w: %w", models.ErrQuantumCheckoutNotFound, pgx.ErrNoRows)
)

type QuantumRepository struct{ pool *pgxpool.Pool }

func quantumInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, errors.New("quantum integer exceeds database bounds")
	}
	return int64(value), nil
}
func NewQuantumRepository(pool *pgxpool.Pool) *QuantumRepository {
	return &QuantumRepository{pool: pool}
}

const quantumInvoiceColumns = `i.id,i.description,i.amount_base_units,i.chain_id,i.token_address,i.receiver_address,i.status,i.checkout_url,i.expires_at,i.paid_at,i.paid_tx_hash,i.created_at,i.merchant_id,m.owner_user_id,m.display_name,i.checkout_token,i.token_decimals,i.explorer_url`
const quantumInvoiceFrom = ` FROM quantum_invoices i JOIN quantum_merchants m ON m.id=i.merchant_id`
const quantumAttemptColumns = `id,payer_address,tx_hash,status,failure_code,confirmations,created_at,updated_at`

func scanQuantumInvoice(row pgx.Row) (models.QuantumInvoiceRecord, error) {
	var r models.QuantumInvoiceRecord
	var amount int64
	err := row.Scan(&r.Invoice.ID, &r.Invoice.Description, &amount, &r.Invoice.ChainID, &r.Invoice.TokenAddress, &r.Invoice.ReceiverAddress, &r.Invoice.Status, &r.Invoice.CheckoutURL, &r.Invoice.ExpiresAt, &r.Invoice.PaidAt, &r.Invoice.PaidTxHash, &r.Invoice.CreatedAt, &r.MerchantID, &r.OwnerID, &r.MerchantName, &r.CheckoutToken, &r.TokenDecimals, &r.ExplorerURL)
	r.Invoice.CreatedAt = r.Invoice.CreatedAt.UTC()
	r.Invoice.ExpiresAt = r.Invoice.ExpiresAt.UTC()
	if r.Invoice.PaidAt != nil {
		at := r.Invoice.PaidAt.UTC()
		r.Invoice.PaidAt = &at
	}
	r.Invoice.AmountBaseUnits = models.Amount(amount)
	r.Invoice.Attempts = []models.QuantumAttempt{}
	return r, err
}
func scanQuantumAttempt(row pgx.Row) (models.QuantumAttempt, error) {
	var a models.QuantumAttempt
	err := row.Scan(&a.ID, &a.PayerAddress, &a.TxHash, &a.Status, &a.FailureCode, &a.Confirmations, &a.CreatedAt, &a.UpdatedAt)
	a.CreatedAt = a.CreatedAt.UTC()
	a.UpdatedAt = a.UpdatedAt.UTC()
	return a, err
}
func quantumVisible(r models.QuantumInvoiceRecord) models.QuantumInvoiceRecord {
	if r.Invoice.Status == models.QuantumPending && !time.Now().Before(r.Invoice.ExpiresAt) {
		r.Invoice.Status = models.QuantumExpired
	}
	return r
}
func quantumMerchant(ctx context.Context, db querier, actor string) (models.QuantumMerchant, error) {
	var m models.QuantumMerchant
	err := db.QueryRow(ctx, `SELECT id,display_name,receiver_address,created_at FROM quantum_merchants WHERE owner_user_id=$1`, actor).Scan(&m.ID, &m.DisplayName, &m.ReceiverAddress, &m.CreatedAt)
	m.CreatedAt = m.CreatedAt.UTC()
	return m, notFound(err, errQuantumMerchantNotFound)
}
func (r *QuantumRepository) Merchant(ctx context.Context, actor string) (models.QuantumMerchant, error) {
	return quantumMerchant(ctx, r.pool, actor)
}
func (r *QuantumRepository) CreateMerchant(ctx context.Context, actor string, req models.QuantumMerchantRequest) (models.QuantumMerchant, error) {
	var m models.QuantumMerchant
	err := r.pool.QueryRow(ctx, `INSERT INTO quantum_merchants(owner_user_id,display_name,receiver_address) VALUES($1,$2,$3) RETURNING id,display_name,receiver_address,created_at`, actor, req.DisplayName, req.ReceiverAddress).Scan(&m.ID, &m.DisplayName, &m.ReceiverAddress, &m.CreatedAt)
	m.CreatedAt = m.CreatedAt.UTC()
	if pgCode(err) == "23505" {
		return m, models.ErrQuantumMerchantExists
	}
	return m, err
}
func (r *QuantumRepository) CreateInvoice(ctx context.Context, actor, key, hash, token, base string, cfg models.QuantumConfig, req models.QuantumInvoiceRequest) (models.IdempotentResult, error) {
	chainID, err := quantumInt64(cfg.ChainID)
	if err != nil {
		return models.IdempotentResult{}, err
	}
	result, err := runIdempotent(ctx, r.pool, actor, key, hash, func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
		merchant, err := quantumMerchant(ctx, tx, actor)
		if err != nil {
			return 0, nil, err
		}
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO quantum_invoices(merchant_id,checkout_token,description,amount_base_units,chain_id,token_address,receiver_address,token_decimals,explorer_url,checkout_url,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW()+make_interval(mins=>$11)) RETURNING id`, merchant.ID, token, req.Description, int64(req.AmountBaseUnits), chainID, cfg.TokenAddress, merchant.ReceiverAddress, cfg.TokenDecimals, cfg.ExplorerURL, base+"/pay/"+token, req.ExpiresInMinutes).Scan(&id)
		if err != nil {
			return 0, nil, err
		}
		invoice, err := scanQuantumInvoice(tx.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.id=$1 AND m.owner_user_id=$2", id, actor))
		if err != nil {
			return 0, nil, err
		}
		body, err := json.Marshal(invoice.Invoice)
		if err != nil {
			return 0, nil, err
		}
		// Match PostgreSQL JSONB's replay representation on the first response as well.
		var canonical string
		if err := tx.QueryRow(ctx, "SELECT $1::jsonb::text", body).Scan(&canonical); err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, []byte(canonical), nil
	})
	if errors.Is(err, ErrIdempotencyMismatch) {
		return models.IdempotentResult{}, models.ErrQuantumIdempotency
	}
	return result, err
}
func quantumAttempts(ctx context.Context, db querier, invoice string) ([]models.QuantumAttempt, error) {
	rows, err := db.Query(ctx, "SELECT "+quantumAttemptColumns+" FROM quantum_payment_attempts WHERE invoice_id=$1 ORDER BY created_at DESC,id DESC", invoice)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.QuantumAttempt{}
	for rows.Next() {
		a, err := scanQuantumAttempt(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func (r *QuantumRepository) Invoice(ctx context.Context, actor, id string) (models.QuantumInvoice, error) {
	rec, err := scanQuantumInvoice(r.pool.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.id=$1 AND m.owner_user_id=$2", id, actor))
	if err != nil {
		return models.QuantumInvoice{}, notFound(err, errQuantumInvoiceNotFound)
	}
	rec = quantumVisible(rec)
	rec.Invoice.Attempts, err = quantumAttempts(ctx, r.pool, id)
	return rec.Invoice, err
}
func (r *QuantumRepository) Invoices(ctx context.Context, actor string, q models.QuantumQuery) ([]models.QuantumInvoice, int, error) {
	const where = ` WHERE m.owner_user_id=$1 AND ($2='' OR (CASE WHEN i.status='PENDING' AND i.expires_at<=NOW() THEN 'EXPIRED' ELSE i.status END)=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*)"+quantumInvoiceFrom+where, actor, string(q.Status)).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+where+" ORDER BY i.created_at DESC,i.id DESC LIMIT $3 OFFSET $4", actor, string(q.Status), q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	invoices := []models.QuantumInvoice{}
	for rows.Next() {
		rec, err := scanQuantumInvoice(rows)
		if err != nil {
			return nil, 0, err
		}
		invoices = append(invoices, quantumVisible(rec).Invoice)
	}
	return invoices, total, rows.Err()
}
func (r *QuantumRepository) Cancel(ctx context.Context, actor, id string) (models.QuantumInvoice, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.QuantumInvoice{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	rec, err := scanQuantumInvoice(tx.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.id=$1 AND m.owner_user_id=$2 FOR UPDATE OF i", id, actor))
	if err != nil {
		return models.QuantumInvoice{}, notFound(err, errQuantumInvoiceNotFound)
	}
	if rec.Invoice.Status != models.QuantumPending || !time.Now().Before(rec.Invoice.ExpiresAt) {
		return models.QuantumInvoice{}, models.ErrQuantumNotPending
	}
	if _, err := tx.Exec(ctx, `UPDATE quantum_invoices SET status='CANCELLED',updated_at=NOW(),version=version+1 WHERE id=$1 AND merchant_id=$2`, id, rec.MerchantID); err != nil {
		return models.QuantumInvoice{}, err
	}
	rec.Invoice.Status = models.QuantumCancelled
	rec.Invoice.Attempts, err = quantumAttempts(ctx, tx, id)
	if err != nil {
		return models.QuantumInvoice{}, err
	}
	return rec.Invoice, tx.Commit(ctx)
}
func (r *QuantumRepository) Checkout(ctx context.Context, token string) (models.QuantumInvoiceRecord, error) {
	rec, err := scanQuantumInvoice(r.pool.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.checkout_token=$1", token))
	return quantumVisible(rec), notFound(err, errQuantumCheckoutNotFound)
}
func (r *QuantumRepository) CheckoutStatus(ctx context.Context, token string) (models.QuantumCheckoutStatus, error) {
	rec, err := r.Checkout(ctx, token)
	if err != nil {
		return models.QuantumCheckoutStatus{}, err
	}
	a, err := scanQuantumAttempt(r.pool.QueryRow(ctx, "SELECT "+quantumAttemptColumns+" FROM quantum_payment_attempts WHERE invoice_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1", rec.Invoice.ID))
	status := models.QuantumCheckoutStatus{InvoiceStatus: rec.Invoice.Status, PaidTxHash: rec.Invoice.PaidTxHash}
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Attempt = &a
	return status, nil
}

// Admission and broadcast share the same lock so an expiring intent cannot be
// submitted while another invoice admits an indistinguishable replacement.
func lockQuantumIntent(ctx context.Context, tx pgx.Tx, invoice models.QuantumInvoice, payer string) error {
	guard := fmt.Sprintf("%d:%s:%s:%s:%d", invoice.ChainID, invoice.TokenAddress, invoice.ReceiverAddress, payer, invoice.AmountBaseUnits)
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", guard)
	return err
}

func (r *QuantumRepository) RegisterAttempt(ctx context.Context, token, payer string) (models.QuantumAttempt, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.QuantumAttempt{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	rec, err := scanQuantumInvoice(tx.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.checkout_token=$1 FOR UPDATE OF i", token))
	if err != nil {
		return models.QuantumAttempt{}, notFound(err, errQuantumCheckoutNotFound)
	}
	if rec.Invoice.Status != models.QuantumPending || !time.Now().Before(rec.Invoice.ExpiresAt) {
		return models.QuantumAttempt{}, models.ErrQuantumNotPending
	}
	if err := lockQuantumIntent(ctx, tx, rec.Invoice, payer); err != nil {
		return models.QuantumAttempt{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM quantum_payment_attempts WHERE invoice_id=$1 AND status='CREATED' AND created_at<=clock_timestamp()-INTERVAL '15 minutes'`, rec.Invoice.ID); err != nil {
		return models.QuantumAttempt{}, err
	}
	chainID, err := quantumInt64(rec.Invoice.ChainID)
	if err != nil {
		return models.QuantumAttempt{}, err
	}
	existing, err := scanQuantumAttempt(tx.QueryRow(ctx, "SELECT "+quantumAttemptColumns+" FROM quantum_payment_attempts WHERE invoice_id=$1 AND payer_address=$2 AND status IN('CREATED','BROADCAST','VERIFYING')", rec.Invoice.ID, payer))
	if err == nil {
		if existing.Status != models.QuantumCreated {
			return models.QuantumAttempt{}, models.ErrQuantumNotPending
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return models.QuantumAttempt{}, err
	}
	var ambiguous bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quantum_payment_attempts a JOIN quantum_invoices i ON i.id=a.invoice_id WHERE a.payer_address=$1 AND i.chain_id=$2 AND i.token_address=$3 AND i.receiver_address=$4 AND i.amount_base_units=$5 AND a.status IN('CREATED','BROADCAST','VERIFYING') AND (a.status<>'CREATED' OR a.created_at>clock_timestamp()-INTERVAL '15 minutes'))`, payer, chainID, rec.Invoice.TokenAddress, rec.Invoice.ReceiverAddress, int64(rec.Invoice.AmountBaseUnits)).Scan(&ambiguous); err != nil {
		return models.QuantumAttempt{}, err
	}
	if ambiguous {
		return models.QuantumAttempt{}, models.ErrQuantumAmbiguous
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM quantum_payment_attempts WHERE invoice_id=$1 AND status IN('CREATED','BROADCAST','VERIFYING')`, rec.Invoice.ID).Scan(&count); err != nil {
		return models.QuantumAttempt{}, err
	}
	if count >= 5 {
		return models.QuantumAttempt{}, models.ErrQuantumAttempts
	}
	a, err := scanQuantumAttempt(tx.QueryRow(ctx, "INSERT INTO quantum_payment_attempts(invoice_id,payer_address) VALUES($1,$2) RETURNING "+quantumAttemptColumns, rec.Invoice.ID, payer))
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}
func (r *QuantumRepository) Submit(ctx context.Context, token, id, hash string) (models.QuantumAttempt, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return models.QuantumAttempt{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	rec, err := scanQuantumInvoice(tx.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.checkout_token=$1 FOR UPDATE OF i", token))
	if err != nil {
		return models.QuantumAttempt{}, notFound(err, errQuantumCheckoutNotFound)
	}
	if rec.Invoice.Status != models.QuantumPending || !time.Now().Before(rec.Invoice.ExpiresAt) {
		return models.QuantumAttempt{}, models.ErrQuantumNotPending
	}
	a, err := scanQuantumAttempt(tx.QueryRow(ctx, "SELECT "+quantumAttemptColumns+" FROM quantum_payment_attempts WHERE id=$1 AND invoice_id=$2 FOR UPDATE", id, rec.Invoice.ID))
	if err != nil {
		return a, notFound(err, errQuantumCheckoutNotFound)
	}
	if a.TxHash != nil || a.Status != models.QuantumCreated {
		return models.QuantumAttempt{}, models.ErrQuantumTxSubmitted
	}
	if err := lockQuantumIntent(ctx, tx, rec.Invoice, a.PayerAddress); err != nil {
		return models.QuantumAttempt{}, err
	}
	a, err = scanQuantumAttempt(tx.QueryRow(ctx, "UPDATE quantum_payment_attempts SET tx_hash=$1,status='BROADCAST',updated_at=NOW(),next_verify_at=NOW() WHERE id=$2 AND invoice_id=$3 AND created_at>clock_timestamp()-INTERVAL '15 minutes' RETURNING "+quantumAttemptColumns, hash, id, rec.Invoice.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.QuantumAttempt{}, models.ErrQuantumAttemptExpired
	}
	if err != nil {
		return a, err
	}
	return a, tx.Commit(ctx)
}
