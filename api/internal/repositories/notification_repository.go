package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/models"
)

var ErrNotificationNotFound = fmt.Errorf("%w: %w", models.ErrDeliveryNotFound, pgx.ErrNoRows)

type NotificationRepository struct{ pool *pgxpool.Pool }

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

const deliveryColumns = `d.id,d.event_id,e.event_type,d.journal_id,j.reference_id,j.currency,
 s.id,s.payout_reference,d.channel,d.destination,d.status,e.status,d.attempts,d.last_error,d.receipt,d.delivered_at,
 CASE WHEN d.status <> 'DELIVERED' AND e.status = 'PENDING' THEN e.available_at ELSE NULL END,
 (d.status = 'FAILED' AND e.status = 'FAILED'),d.created_at`
const deliveryFrom = ` FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id
 JOIN journal_entries j ON j.id=d.journal_id
 LEFT JOIN settlements s ON (s.reserve_journal_id=j.id OR s.capture_journal_id=j.id OR s.release_journal_id=j.id)`

func scanDelivery(row pgx.Row) (models.NotificationDelivery, error) {
	var d models.NotificationDelivery
	err := row.Scan(&d.ID, &d.EventID, &d.EventType, &d.JournalID, &d.JournalReference, &d.Currency,
		&d.SettlementID, &d.PayoutReference, &d.Channel, &d.Destination, &d.Status, &d.DispatchStatus, &d.Attempts,
		&d.LastError, &d.Receipt, &d.DeliveredAt, &d.NextAttemptAt, &d.CanRetry, &d.CreatedAt)
	return d, notFound(err, ErrNotificationNotFound)
}
func (r *NotificationRepository) List(ctx context.Context, q models.DeliveryQuery) ([]models.NotificationDelivery, int, error) {
	const where = ` WHERE ($1='' OR j.currency=$1) AND ($2='' OR d.status=$2) AND ($3='' OR d.channel=$3)`
	args := []any{string(q.Currency), string(q.Status), string(q.Channel)}
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*)"+deliveryFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+deliveryColumns+deliveryFrom+where+" ORDER BY d.created_at DESC,d.id DESC LIMIT $4 OFFSET $5", append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []models.NotificationDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, d)
	}
	return items, total, rows.Err()
}
func (r *NotificationRepository) Detail(ctx context.Context, id string) (models.DeliveryDetail, error) {
	d, err := scanDelivery(r.pool.QueryRow(ctx, "SELECT "+deliveryColumns+deliveryFrom+" WHERE d.id=$1", id))
	if err != nil {
		return models.DeliveryDetail{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT attempt,status,receipt,error,occurred_at FROM notification_attempts WHERE delivery_id=$1 ORDER BY attempt DESC`, id)
	if err != nil {
		return models.DeliveryDetail{}, err
	}
	defer rows.Close()
	attempts := []models.DeliveryAttempt{}
	for rows.Next() {
		var a models.DeliveryAttempt
		if err := rows.Scan(&a.Attempt, &a.Status, &a.Receipt, &a.Error, &a.OccurredAt); err != nil {
			return models.DeliveryDetail{}, err
		}
		attempts = append(attempts, a)
	}
	return models.DeliveryDetail{Delivery: d, Attempts: attempts}, rows.Err()
}
func (r *NotificationRepository) Metrics(ctx context.Context, currency models.Currency) (models.DeliveryMetrics, error) {
	var m models.DeliveryMetrics
	err := r.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE d.status='PENDING'),count(*) FILTER(WHERE d.status='DELIVERED'),count(*) FILTER(WHERE d.status='FAILED') FROM notification_deliveries d JOIN journal_entries j ON j.id=d.journal_id WHERE ($1='' OR j.currency=$1)`, string(currency)).Scan(&m.Pending, &m.Delivered, &m.Failed)
	return m, err
}

// Prepare uses a unique event/channel pair so replay never recreates successful deliveries.
func (r *NotificationRepository) Prepare(ctx context.Context, event models.OutboxEvent) ([]models.NotificationDelivery, error) {
	var live bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM outbox_events WHERE id=$1 AND lease_id=$2 AND status='SENDING' AND lease_until>NOW())", event.ID, event.LeaseID).Scan(&live); err != nil {
		return nil, err
	}
	if !live {
		return nil, ErrOutboxLeaseLost
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO notification_deliveries(event_id,journal_id,channel,destination)
 SELECT e.id,j.id,c.channel,c.destination FROM outbox_events e JOIN journal_entries j ON j.id::text=e.aggregate_id
 CROSS JOIN (VALUES ('EMAIL','sandbox-email-inbox'),('WEBHOOK','sandbox-webhook-receiver')) c(channel,destination)
 WHERE e.id=$1 AND e.aggregate_type='JOURNAL' AND e.event_type IN ('ledger.journal_posted.v1','ledger.journal_reversed.v1')
 AND e.status='SENDING' AND e.lease_id=$2 AND e.lease_until>NOW()
 ON CONFLICT(event_id,channel) DO NOTHING`, event.ID, event.LeaseID)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, "SELECT "+deliveryColumns+deliveryFrom+" WHERE d.event_id=$1 ORDER BY d.channel", event.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.NotificationDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, rows.Err()
}

// Record is fenced by the outbox lease; the attempt and current outcome commit together.
func (r *NotificationRepository) Record(ctx context.Context, event models.OutboxEvent, id string, receipt, lastError *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var eventID string
	if err := tx.QueryRow(ctx, `SELECT id FROM outbox_events WHERE id=$1 AND lease_id=$2 AND status='SENDING' AND lease_until>NOW() FOR UPDATE`, event.ID, event.LeaseID).Scan(&eventID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOutboxLeaseLost
		}
		return err
	}
	var attempt int
	err = tx.QueryRow(ctx, `UPDATE notification_deliveries SET attempts=attempts+1,
 status=CASE WHEN $3::text IS NULL THEN 'FAILED' ELSE 'DELIVERED' END,receipt=$3,last_error=$4,
 delivered_at=CASE WHEN $3::text IS NOT NULL THEN NOW() ELSE NULL END
 WHERE id=$1 AND event_id=$2 AND status<>'DELIVERED'
 AND NOT EXISTS(SELECT 1 FROM notification_attempts WHERE delivery_id=$1 AND lease_id=$5)
 RETURNING attempts`, id, event.ID, receipt, lastError, event.LeaseID).Scan(&attempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var delivered bool
			if err := tx.QueryRow(ctx, "SELECT status='DELIVERED' FROM notification_deliveries WHERE id=$1 AND event_id=$2", id, event.ID).Scan(&delivered); err != nil {
				return notFound(err, ErrNotificationNotFound)
			}
			if delivered {
				return tx.Commit(ctx)
			}
			return models.ErrDeliveryStale
		}
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notification_attempts(delivery_id,lease_id,attempt,status,receipt,error)
 VALUES($1,$2,$3,CASE WHEN $4::text IS NULL THEN 'FAILED' ELSE 'DELIVERED' END,$4,$5)`, id, event.LeaseID, attempt, receipt, lastError)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *NotificationRepository) Retry(ctx context.Context, id string, attempts int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	// Lock the event first, as Record does. Then read the attempt version in a
	// fresh statement so a worker completing while we waited cannot be retried
	// using a stale snapshot from before the lock was acquired.
	var eventID string
	err = tx.QueryRow(ctx, `SELECT e.id FROM outbox_events e JOIN notification_deliveries d ON d.event_id=e.id
 WHERE d.id=$1 AND e.status='FAILED' FOR UPDATE OF e`, id).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ErrDeliveryStale
	}
	if err != nil {
		return err
	}
	var current bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_deliveries WHERE id=$1 AND status='FAILED' AND attempts=$2)`, id, attempts).Scan(&current); err != nil {
		return err
	}
	if !current {
		return models.ErrDeliveryStale
	}
	if _, err := tx.Exec(ctx, `UPDATE outbox_events SET status='PENDING',retry_count=0,available_at=NOW(),last_error=NULL,lease_id=NULL,lease_until=NULL WHERE id=$1`, eventID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
