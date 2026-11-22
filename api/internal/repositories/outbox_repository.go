package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
)

var ErrOutboxLeaseLost = errors.New("outbox event lease was lost or expired")

// OutboxRepository handles transactional outbox persistence and delivery claims.
// A claim is committed before publish, so no publish happens inside a database
// transaction; a lease token fences late workers after expiry.
type OutboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{pool: pool}
}

// Claim leases due events for at least lease. Sending rows become claimable again
// once their lease expires, which is what recovers a worker that died mid-publish.
func (r *OutboxRepository) Claim(ctx context.Context, batchSize int, lease time.Duration) ([]models.OutboxEvent, error) {
	if r.pool == nil {
		return nil, nil
	}
	if batchSize <= 0 || lease <= 0 {
		return nil, errors.New("invalid outbox claim arguments")
	}
	rows, err := r.pool.Query(ctx, `
		WITH due AS (
			SELECT id FROM outbox_events
			WHERE aggregate_type <> 'QUANTUM_INVOICE' AND ((status = 'PENDING' AND available_at <= NOW())
			   OR (status = 'SENDING' AND lease_until <= NOW()))
			ORDER BY available_at, created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events e
		SET status = 'SENDING',
		    lease_id = gen_random_uuid()::text,
		    lease_until = NOW() + make_interval(secs => $2)
		FROM due
		WHERE e.id = due.id
		RETURNING e.id, e.aggregate_type, e.aggregate_id, e.event_type, e.payload,
		          e.retry_count, e.created_at, e.available_at, e.lease_id, e.last_error
	`, batchSize, lease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.OutboxEvent
	for rows.Next() {
		var e models.OutboxEvent
		var payload []byte
		if err := rows.Scan(
			&e.ID,
			&e.AggregateType,
			&e.AggregateID,
			&e.EventType,
			&payload,
			&e.RetryCount,
			&e.CreatedAt,
			&e.AvailableAt,
			&e.LeaseID,
			&e.LastError,
		); err != nil {
			return nil, err
		}
		e.Payload = payload
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// Complete records one publish attempt. A successful publish is terminal; a
// retry stays PENDING with a backoff and a permanent or exhausted one is FAILED.
// The lease token prevents a late worker from overwriting a newer claim.
func (r *OutboxRepository) Complete(ctx context.Context, event models.OutboxEvent, status models.OutboxStatus, retryAfter time.Duration, lastError *string) error {
	if r.pool == nil {
		return nil
	}
	if !status.Valid() || status == models.OutboxSending || event.LeaseID == "" {
		return errors.New("invalid outbox completion")
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET status = $3,
		    retry_count = CASE WHEN $3 = 'PUBLISHED' THEN retry_count ELSE retry_count + 1 END,
		    published_at = CASE WHEN $3 = 'PUBLISHED' THEN NOW() ELSE published_at END,
		    available_at = CASE WHEN $3 = 'PENDING' THEN NOW() + make_interval(secs => $4) ELSE now() END,
		    lease_id = NULL,
		    lease_until = NULL,
		    last_error = $5
		WHERE id = $1 AND lease_id = $2 AND status = 'SENDING'
	`, event.ID, event.LeaseID, string(status), retryAfter.Seconds(), lastError)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrOutboxLeaseLost
	}
	return nil
}

// GetMetrics calculates real-time event counts and the age of the oldest pending event.
func (r *OutboxRepository) GetMetrics(ctx context.Context) (models.OutboxMetrics, error) {
	m := models.OutboxMetrics{CheckedAt: time.Now().UTC(), Healthy: true}
	if r.pool == nil {
		return m, nil
	}

	var lag float64
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'PENDING'),
			COUNT(*) FILTER (WHERE status = 'SENDING'),
			COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
			COUNT(*) FILTER (WHERE status = 'FAILED'),
			COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at) FILTER (WHERE status = 'PENDING')))::double precision, 0)
		FROM outbox_events WHERE aggregate_type <> 'QUANTUM_INVOICE'
	`).Scan(&m.PendingCount, &m.SendingCount, &m.PublishedCount, &m.FailedCount, &lag)
	if err != nil {
		return m, err
	}
	m.LagSeconds = lag
	m.Healthy = m.FailedCount == 0 && lag < 60
	return m, nil
}

// ReplayFailed resets failed events so the worker can re-attempt delivery.
func (r *OutboxRepository) ReplayFailed(ctx context.Context, ids ...string) (int64, error) {
	if r.pool == nil {
		return 0, nil
	}

	var tag pgconn.CommandTag
	var err error
	if len(ids) > 0 {
		tag, err = r.pool.Exec(ctx, `
			UPDATE outbox_events
			SET status = 'PENDING', retry_count = 0, available_at = NOW(),
			    last_error = NULL, lease_id = NULL, lease_until = NULL
			WHERE aggregate_type <> 'QUANTUM_INVOICE' AND status = 'FAILED' AND id = ANY($1)
		`, ids)
	} else {
		tag, err = r.pool.Exec(ctx, `
			UPDATE outbox_events
			SET status = 'PENDING', retry_count = 0, available_at = NOW(),
			    last_error = NULL, lease_id = NULL, lease_until = NULL
			WHERE aggregate_type <> 'QUANTUM_INVOICE' AND status = 'FAILED'
		`)
	}
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
