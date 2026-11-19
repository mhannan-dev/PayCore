package repository

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/audit"
)

// AuditRepository stores and lists audit events (append-only; see migration 013).
type AuditRepository struct{ pool *pgxpool.Pool }

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository { return &AuditRepository{pool: pool} }

// execer is satisfied by *pgxpool.Pool and pgx.Tx, so events can be written inside
// the transaction that makes the audited change.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// insertAudit writes e, filling the actor and request metadata from ctx.
func insertAudit(ctx context.Context, db execer, e audit.Event) error {
	e = audit.Complete(ctx, e)
	details, err := json.Marshal(e.Details)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO audit_events
		(action, actor_user_id, actor_email, target_user_id, target_email, details, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		string(e.Action), e.ActorID, e.ActorEmail, e.TargetID, e.TargetEmail, details, e.IP, e.UserAgent)
	return err
}

// Record implements audit.Recorder.
func (r *AuditRepository) Record(ctx context.Context, e audit.Event) error {
	return insertAudit(ctx, r.pool, e)
}

// AuditQuery filters and pages the audit log.
type AuditQuery struct {
	Action audit.Action // "" for any
	Search string       // matched against actor and target email, case-insensitive
	Limit  int
	Offset int
}

// List returns one page of events, newest first, plus the number of matches.
func (r *AuditRepository) List(ctx context.Context, q AuditQuery) ([]audit.Event, int, error) {
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.Search) + "%"
	const where = ` WHERE ($1 = '' OR action = $1)
		AND ($2 = '%%' OR COALESCE(actor_email, '') ILIKE $2 OR COALESCE(target_email, '') ILIKE $2)`
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events"+where, string(q.Action), pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id, occurred_at, action, actor_user_id, actor_email, target_user_id, target_email, details, ip, user_agent
		FROM audit_events`+where+` ORDER BY occurred_at DESC, id DESC LIMIT $3 OFFSET $4`, string(q.Action), pattern, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := []audit.Event{}
	for rows.Next() {
		var e audit.Event
		var action string
		var details []byte
		if err := rows.Scan(&e.ID, &e.OccurredAt, &action, &e.ActorID, &e.ActorEmail, &e.TargetID, &e.TargetEmail, &details, &e.IP, &e.UserAgent); err != nil {
			return nil, 0, err
		}
		e.Action = audit.Action(action)
		if err := json.Unmarshal(details, &e.Details); err != nil {
			return nil, 0, err
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}
