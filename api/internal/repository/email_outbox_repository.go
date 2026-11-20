package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

var ErrNoEmailJob = model.ErrNoEmailJob
var ErrEmailLeaseLost = model.ErrEmailLeaseLost

type EmailOutboxRepository struct{ pool *pgxpool.Pool }

func NewEmailOutboxRepository(pool *pgxpool.Pool) *EmailOutboxRepository {
	return &EmailOutboxRepository{pool: pool}
}

// insertEmail is called inside the same transaction as user/token creation.
func insertEmail(ctx context.Context, tx pgx.Tx, kind model.EmailKind, recipient model.EmailRecipient, hash []byte, expires time.Time, drafts ...model.EmailDraft) error {
	content := model.EmailContent{To: recipient.Email}
	status := model.EmailDisabled
	if len(drafts) > 0 && drafts[0] != nil {
		var err error
		content, err = drafts[0](recipient)
		if err != nil {
			return err
		}
		status = model.EmailQueued
	}
	if _, err := tx.Exec(ctx, `INSERT INTO email_outbox
		(kind,user_id,token_hash,recipient,subject,text_body,html_body,status,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, kind, recipient.ID, hash, content.To, content.Subject, content.Text, content.HTML, status, expires); err != nil {
		return err
	}
	if kind == model.EmailInvitation {
		_, err := tx.Exec(ctx, `UPDATE user_invitations SET email_status=$2 WHERE token_hash=$1`, hash, status)
		return err
	}
	return nil
}

// Claim leases one job without holding a database transaction during SMTP I/O.
// A two-minute lease exceeds the worker's 45-second delivery deadline.
func (r *EmailOutboxRepository) Claim(ctx context.Context) (model.EmailJob, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.EmailJob{}, err
	}
	defer tx.Rollback(ctx)
	// Expired, superseded and exhausted jobs must never send stale reset links.
	rows, err := tx.Query(ctx, `UPDATE email_outbox e SET
		status=CASE WHEN attempts>=5 THEN 'failed' ELSE 'canceled' END,
		text_body='',html_body='',lease_id=NULL,lease_until=NULL,
		last_error=CASE WHEN attempts>=5 THEN 'attempts_exhausted' ELSE 'token_unavailable' END,updated_at=NOW()
		WHERE e.id IN (SELECT x.id FROM email_outbox x
		WHERE (x.status='pending' OR (x.status='sending' AND x.lease_until<=NOW())) AND
		(x.expires_at<=NOW() OR x.attempts>=5 OR NOT EXISTS (
		 SELECT 1 FROM users u WHERE u.id=x.user_id AND u.is_active AND
		 CASE x.kind WHEN 'invitation' THEN u.password_hash IS NULL AND EXISTS
		  (SELECT 1 FROM user_invitations i WHERE i.token_hash=x.token_hash AND i.used_at IS NULL AND i.expires_at>NOW())
		 ELSE EXISTS (SELECT 1 FROM password_reset_tokens p WHERE p.token_hash=x.token_hash AND p.used_at IS NULL AND p.expires_at>NOW()) END
		)) ORDER BY x.created_at LIMIT 200 FOR UPDATE SKIP LOCKED)
		RETURNING kind,token_hash,attempts,last_error`)
	if err != nil {
		return model.EmailJob{}, err
	}
	type stopped struct {
		kind     model.EmailKind
		hash     []byte
		attempts int
		code     string
	}
	var canceled []stopped
	for rows.Next() {
		var s stopped
		if err = rows.Scan(&s.kind, &s.hash, &s.attempts, &s.code); err != nil {
			rows.Close()
			return model.EmailJob{}, err
		}
		canceled = append(canceled, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return model.EmailJob{}, err
	}
	for _, s := range canceled {
		if s.kind == model.EmailInvitation {
			if err = recordOutboxInvitation(ctx, tx, s.hash, model.EmailDeliveryFailed, s.attempts, s.code); err != nil {
				return model.EmailJob{}, err
			}
		}
	}
	var job model.EmailJob
	err = tx.QueryRow(ctx, `UPDATE email_outbox e SET status='sending', attempts=e.attempts+1,
		lease_id=gen_random_uuid(),lease_until=NOW()+INTERVAL '2 minutes',updated_at=NOW()
		WHERE e.id=(SELECT x.id FROM email_outbox x JOIN users u ON u.id=x.user_id
		WHERE x.attempts<5 AND x.expires_at>NOW() AND u.is_active AND
		((x.status='pending' AND x.available_at<=NOW()) OR (x.status='sending' AND x.lease_until<=NOW())) AND
		CASE x.kind WHEN 'invitation' THEN u.password_hash IS NULL AND EXISTS
		 (SELECT 1 FROM user_invitations i WHERE i.token_hash=x.token_hash AND i.used_at IS NULL AND i.expires_at>NOW())
		ELSE EXISTS (SELECT 1 FROM password_reset_tokens p WHERE p.token_hash=x.token_hash AND p.used_at IS NULL AND p.expires_at>NOW()) END
		ORDER BY x.available_at,x.created_at LIMIT 1 FOR UPDATE OF x SKIP LOCKED)
		RETURNING e.id,e.lease_id,e.kind,e.recipient,e.subject,e.text_body,e.html_body,e.attempts,e.expires_at`).Scan(
		&job.ID, &job.LeaseID, &job.Kind, &job.Content.To, &job.Content.Subject, &job.Content.Text, &job.Content.HTML, &job.Attempts, &job.ExpiresAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.EmailJob{}, err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return model.EmailJob{}, commitErr
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmailJob{}, ErrNoEmailJob
	}
	return job, nil
}

// Complete uses the lease as a fencing token. Late workers cannot overwrite a
// newer claim. Terminal jobs discard one-time links in the same transaction.
func (r *EmailOutboxRepository) Complete(ctx context.Context, job model.EmailJob, result model.EmailOutcome) error {
	switch result.Status {
	case model.EmailQueued, model.EmailDelivered, model.EmailDeliveryFailed, model.EmailDisabled:
	default:
		return errors.New("invalid email outcome")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var kind model.EmailKind
	var hash []byte
	var attempts int
	err = tx.QueryRow(ctx, `UPDATE email_outbox SET status=$3,available_at=NOW()+make_interval(secs=>$4),
		text_body=CASE WHEN $3='pending' THEN text_body ELSE '' END,
		html_body=CASE WHEN $3='pending' THEN html_body ELSE '' END,
		last_error=NULLIF($5,''),sent_at=CASE WHEN $3='sent' THEN NOW() ELSE sent_at END,
		lease_id=NULL,lease_until=NULL,updated_at=NOW()
		WHERE id=$1 AND lease_id=$2 AND status='sending' AND lease_until>NOW()
		RETURNING kind,token_hash,attempts`, job.ID, job.LeaseID, result.Status, result.RetryAfter.Seconds(), result.ErrorCode).Scan(&kind, &hash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEmailLeaseLost
	}
	if err != nil {
		return err
	}
	if kind == model.EmailInvitation {
		if err = recordOutboxInvitation(ctx, tx, hash, result.Status, attempts, result.ErrorCode); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func recordOutboxInvitation(ctx context.Context, tx pgx.Tx, hash []byte, status model.EmailStatus, attempts int, code string) error {
	_, err := tx.Exec(ctx, `UPDATE user_invitations SET email_status=$2,email_attempts=$3,email_error=NULLIF($4,''),
		email_sent_at=CASE WHEN $2='sent' THEN NOW() ELSE email_sent_at END WHERE token_hash=$1`, hash, status, attempts, code)
	return err
}
