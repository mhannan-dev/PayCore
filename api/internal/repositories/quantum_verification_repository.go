package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

func (r *QuantumRepository) Work(ctx context.Context) ([]models.QuantumWork, error) {
	rows, err := r.pool.Query(ctx, `WITH due AS (SELECT id FROM quantum_payment_attempts WHERE tx_hash IS NOT NULL AND
 (status IN('BROADCAST','VERIFYING') OR (status IN('CONFIRMED','REORGED') AND updated_at>NOW()-INTERVAL '24 hours'))
 AND next_verify_at<=NOW() AND (lease_until IS NULL OR lease_until<=NOW()) ORDER BY next_verify_at LIMIT 10 FOR UPDATE SKIP LOCKED)
 UPDATE quantum_payment_attempts a SET lease_id=gen_random_uuid()::text,lease_until=NOW()+INTERVAL '3 minutes'
 FROM due WHERE a.id=due.id RETURNING a.id,a.invoice_id,a.lease_id,a.observed_hash,a.rpc_failures`)
	if err != nil {
		return nil, err
	}
	type claim struct {
		id, invoice, lease string
		hash               *string
		failures           int
	}
	claims := []claim{}
	for rows.Next() {
		var c claim
		if err := rows.Scan(&c.id, &c.invoice, &c.lease, &c.hash, &c.failures); err != nil {
			rows.Close()
			return nil, err
		}
		claims = append(claims, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	work := []models.QuantumWork{}
	for _, c := range claims {
		inv, err := scanQuantumInvoice(r.pool.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.id=$1", c.invoice))
		if err != nil {
			return nil, err
		}
		a, err := scanQuantumAttempt(r.pool.QueryRow(ctx, "SELECT "+quantumAttemptColumns+" FROM quantum_payment_attempts WHERE id=$1", c.id))
		if err != nil {
			return nil, err
		}
		work = append(work, models.QuantumWork{Invoice: inv, Attempt: a, LeaseID: c.lease, ObservedHash: c.hash, Failures: c.failures})
	}
	return work, nil
}
func (r *QuantumRepository) RetryWork(ctx context.Context, w models.QuantumWork, delay time.Duration) error {
	tag, err := r.pool.Exec(ctx, `UPDATE quantum_payment_attempts SET next_verify_at=NOW()+make_interval(secs=>$3),rpc_failures=rpc_failures+1,lease_id=NULL,lease_until=NULL WHERE id=$1 AND lease_id=$2`, w.Attempt.ID, w.LeaseID, delay.Seconds())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return models.ErrQuantumStale
	}
	return nil
}
func quantumUpdateAttempt(ctx context.Context, tx pgx.Tx, w models.QuantumWork, d models.QuantumDecision) error {
	var block *int64
	var hash *string
	if d.Receipt.Found {
		n, err := quantumInt64(d.Receipt.BlockNumber)
		if err != nil {
			return err
		}
		block = &n
		hash = &d.Receipt.BlockHash
	}
	_, err := tx.Exec(ctx, `UPDATE quantum_payment_attempts SET status=$2,failure_code=$3,confirmations=$4,observed_block=COALESCE($5,observed_block),observed_hash=COALESCE($6,observed_hash),
 next_verify_at=NOW()+make_interval(secs=>$7),lease_id=NULL,lease_until=NULL,rpc_failures=0,updated_at=CASE WHEN status=$2 THEN updated_at ELSE NOW() END WHERE id=$1`, w.Attempt.ID, string(d.Status), d.Failure, d.Receipt.Confirmations, block, hash, d.RetryAfter.Seconds())
	return err
}
func quantumEvent(ctx context.Context, tx pgx.Tx, rec models.QuantumInvoiceRecord, hash, eventType string) error {
	event := struct {
		InvoiceID  string        `json:"invoice_id"`
		MerchantID string        `json:"merchant_id"`
		Amount     models.Amount `json:"amount_base_units"`
		ChainID    uint64        `json:"chain_id"`
		TxHash     string        `json:"tx_hash"`
	}{rec.Invoice.ID, rec.MerchantID, rec.Invoice.AmountBaseUnits, rec.Invoice.ChainID, hash}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(aggregate_type,aggregate_id,event_type,payload) VALUES('QUANTUM_INVOICE',$1,$2,$3)`, rec.Invoice.ID, eventType, body)
	return err
}
func (r *QuantumRepository) Apply(ctx context.Context, w models.QuantumWork, d models.QuantumDecision) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	rec, err := scanQuantumInvoice(tx.QueryRow(ctx, "SELECT "+quantumInvoiceColumns+quantumInvoiceFrom+" WHERE i.id=$1 FOR UPDATE OF i", w.Invoice.Invoice.ID))
	if err != nil {
		return err
	}
	var live bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(lease_id=$2 AND lease_until>NOW(),false) FROM quantum_payment_attempts WHERE id=$1 AND invoice_id=$3 FOR UPDATE`, w.Attempt.ID, w.LeaseID, rec.Invoice.ID).Scan(&live); err != nil {
		return err
	}
	if !live {
		return models.ErrQuantumStale
	}
	if d.Reorg {
		if _, err := tx.Exec(ctx, `UPDATE quantum_transfer_claims SET canonical=false WHERE invoice_id=$1 AND attempt_id=$2`, rec.Invoice.ID, w.Attempt.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE quantum_invoices SET status='PENDING',paid_at=NULL,paid_tx_hash=NULL,updated_at=NOW(),version=version+1
 WHERE id=$1 AND status='PAID' AND NOT EXISTS(SELECT 1 FROM quantum_transfer_claims WHERE invoice_id=$1 AND canonical)`, rec.Invoice.ID); err != nil {
			return err
		}
		if err := quantumUpdateAttempt(ctx, tx, w, d); err != nil {
			return err
		}
		details, err := json.Marshal(audit.Details{Reason: "Quantum invoice " + rec.Invoice.ID + " payment lost canonical confirmation"})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events(action,details) VALUES($1,$2)`, string(audit.QuantumPaymentReorg), details); err != nil {
			return err
		}
		if w.Attempt.TxHash != nil {
			if err := quantumEvent(ctx, tx, rec, *w.Attempt.TxHash, "quantum.invoice.reorged"); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
	if d.Status != models.QuantumConfirmed || w.Attempt.Status == models.QuantumConfirmed {
		if err := quantumUpdateAttempt(ctx, tx, w, d); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var domainErr error
	if rec.Invoice.Status != models.QuantumPending {
		failure := models.QuantumLate
		if rec.Invoice.Status == models.QuantumPaid {
			failure = models.QuantumDuplicateTransfer
			domainErr = models.ErrQuantumDuplicate
		}
		d.Status = models.QuantumReview
		d.Failure = &failure
	} else if d.Transfer == nil || w.Attempt.TxHash == nil {
		return models.ErrQuantumStale
	} else {
		t := d.Transfer
		chainID, err := quantumInt64(rec.Invoice.ChainID)
		if err != nil {
			return err
		}
		blockNumber, err := quantumInt64(d.Receipt.BlockNumber)
		if err != nil {
			return err
		}
		logIndex, err := quantumInt64(t.LogIndex)
		if err != nil {
			return err
		}
		var claimID string
		err = tx.QueryRow(ctx, `INSERT INTO quantum_transfer_claims(invoice_id,attempt_id,chain_id,tx_hash,log_index,token_address,from_address,to_address,amount_base_units,block_number,block_hash)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(chain_id,tx_hash,log_index) DO NOTHING RETURNING id`, rec.Invoice.ID, w.Attempt.ID, chainID, *w.Attempt.TxHash, logIndex, t.Token, t.From, t.To, t.Amount, blockNumber, d.Receipt.BlockHash).Scan(&claimID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
			var own bool
			if err := tx.QueryRow(ctx, `SELECT invoice_id=$1 AND attempt_id=$2 AND NOT canonical FROM quantum_transfer_claims WHERE chain_id=$3 AND tx_hash=$4 AND log_index=$5`, rec.Invoice.ID, w.Attempt.ID, chainID, *w.Attempt.TxHash, logIndex).Scan(&own); err != nil {
				return err
			}
			if own {
				_, err = tx.Exec(ctx, `UPDATE quantum_transfer_claims SET canonical=true,block_number=$3,block_hash=$4 WHERE invoice_id=$1 AND attempt_id=$2`, rec.Invoice.ID, w.Attempt.ID, blockNumber, d.Receipt.BlockHash)
			} else {
				failure := models.QuantumDuplicateTransfer
				d.Status = models.QuantumReview
				d.Failure = &failure
				domainErr = models.ErrQuantumDuplicate
			}
		} else if err != nil {
			return err
		}
		if err != nil {
			return err
		}
		if d.Status == models.QuantumConfirmed {
			if _, err := tx.Exec(ctx, `UPDATE quantum_invoices SET status='PAID',paid_at=NOW(),paid_tx_hash=$2,updated_at=NOW(),version=version+1 WHERE id=$1`, rec.Invoice.ID, *w.Attempt.TxHash); err != nil {
				return err
			}
			if err := quantumEvent(ctx, tx, rec, *w.Attempt.TxHash, "quantum.invoice.paid"); err != nil {
				return err
			}
		}
	}
	if err := quantumUpdateAttempt(ctx, tx, w, d); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return domainErr
}
