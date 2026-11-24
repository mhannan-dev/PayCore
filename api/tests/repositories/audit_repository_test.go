package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/audit"
	"paycore/api/internal/repositories"
)

func TestAuditTrail(t *testing.T) {
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	invites := repositories.NewInvitationRepository(pool)
	rbac := repositories.NewRBACRepository(pool)
	log := repositories.NewAuditRepository(pool)
	digest := func(s string) []byte { d := sha256.Sum256([]byte(s)); return d[:] }

	admin, err := invites.CreateInvitedUser(ctx, "Admin", nil, "admin@example.test", []string{"ADMIN"}, digest("a"), time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	member, err := invites.CreateInvitedUser(ctx, "Member", nil, "member@example.test", []string{"ACCOUNTANT"}, digest("m"), time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	// The request context carries the acting admin and where the request came from.
	actx := audit.WithMeta(audit.WithActor(ctx, audit.Actor{ID: admin.ID, Email: admin.Email}), audit.Meta{IP: "203.0.113.7", UserAgent: "test-agent"})

	if _, err := rbac.SetUserRoles(actx, member.ID, []string{"ACCOUNTANT", "COMPLIANCE_AUDITOR"}); err != nil {
		t.Fatal(err)
	}
	events, total, err := log.List(ctx, repositories.AuditQuery{Action: audit.RolesChanged, Limit: 10})
	if err != nil || total != 1 || len(events) != 1 {
		t.Fatalf("roles_changed: %d %+v %v", total, events, err)
	}
	e := events[0]
	if e.ActorID == nil || *e.ActorID != admin.ID || e.TargetEmail == nil || *e.TargetEmail != "member@example.test" ||
		e.IP == nil || *e.IP != "203.0.113.7" || !slices.Equal(e.Details.RolesBefore, []string{"ACCOUNTANT"}) ||
		!slices.Equal(e.Details.RolesAfter, []string{"ACCOUNTANT", "COMPLIANCE_AUDITOR"}) {
		t.Fatalf("roles_changed event: %+v", e)
	}

	// Deactivate: signs out, audits once; repeating it is a no-op.
	if _, err := pool.Exec(ctx, "INSERT INTO auth_sessions (token_hash, user_id, expires_at) VALUES ($1, $2, NOW() + interval '1 hour')", digest("s"), member.ID); err != nil {
		t.Fatal(err)
	}
	u, err := rbac.SetUserActive(actx, member.ID, false)
	if err != nil || u.IsActive {
		t.Fatalf("deactivate: %+v %v", u, err)
	}
	var sessions int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM auth_sessions WHERE user_id = $1", member.ID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions after deactivate: %d %v", sessions, err)
	}
	if _, err := rbac.SetUserActive(actx, member.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := log.List(ctx, repositories.AuditQuery{Action: audit.UserDeactivated, Limit: 10}); n != 1 {
		t.Fatalf("deactivate should be audited once, got %d", n)
	}
	if u, err := rbac.SetUserActive(actx, member.ID, true); err != nil || !u.IsActive {
		t.Fatalf("activate: %+v %v", u, err)
	}

	// The last active admin cannot be deactivated; nothing is recorded for the failed attempt.
	if _, err := rbac.SetUserActive(actx, admin.ID, false); !errors.Is(err, repositories.ErrLastAdmin) {
		t.Fatalf("last admin: %v", err)
	}
	if _, err := rbac.SetUserActive(actx, "33333333-3333-4333-8333-333333333333", false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown user: %v", err)
	}

	// Deleting a user keeps their history.
	if err := rbac.DeleteUser(actx, member.ID); err != nil {
		t.Fatal(err)
	}
	history, n, err := log.List(ctx, repositories.AuditQuery{Search: "MEMBER@", Limit: 10})
	if err != nil || n != 5 { // invitation, roles_changed, deactivated, activated, deleted
		t.Fatalf("history after delete: %d %v", n, err)
	}
	if history[0].Action != audit.UserDeleted || !slices.Equal(history[0].Details.RolesBefore, []string{"ACCOUNTANT", "COMPLIANCE_AUDITOR"}) {
		t.Fatalf("newest event should be the delete: %+v", history[0])
	}

	// Best-effort recording outside a transaction works too.
	if err := log.Record(ctx, audit.Event{Action: audit.LoginFailed, TargetEmail: audit.Ptr("nobody@example.test"), Details: audit.Details{Reason: "unknown_email"}}); err != nil {
		t.Fatal(err)
	}

	// The table is append-only, even for the application's own connection.
	for _, stmt := range []string{"UPDATE audit_events SET action = 'logout'", "DELETE FROM audit_events", "TRUNCATE audit_events"} {
		if _, err := pool.Exec(ctx, stmt); err == nil {
			t.Fatalf("%q must be rejected", stmt)
		}
	}
	if _, total, _ := log.List(ctx, repositories.AuditQuery{Limit: 1}); total != 7 { // 2 invitations + 4 user events + 1 login failure
		t.Fatalf("expected 7 events to survive, got %d", total)
	}
}
