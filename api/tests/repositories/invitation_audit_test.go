package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

func TestInvitationAuditAtomicity(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	admin, err := users.Create(ctx, "Admin", nil, "admin@example.test", "fixture hash", []string{models.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	ctx = audit.WithMeta(audit.WithActor(ctx, audit.Actor{ID: admin.ID, Email: admin.Email}), audit.Meta{IP: "203.0.113.7", UserAgent: "audit-test"})
	repo := repositories.NewInvitationRepository(pool)
	first := sha256.Sum256([]byte("first invitation"))
	second := sha256.Sum256([]byte("replacement invitation"))
	expires := time.Now().Add(time.Hour)
	block := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `ALTER TABLE audit_events ADD CONSTRAINT reject_invitation_audit CHECK(action NOT IN ('user_invited','invitation_resent','invitation_accepted')) NOT VALID`); err != nil {
			t.Fatal(err)
		}
	}
	unblock := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, "ALTER TABLE audit_events DROP CONSTRAINT reject_invitation_audit"); err != nil {
			t.Fatal(err)
		}
	}
	block()
	if _, err := repo.CreateInvitedUser(ctx, "Member", nil, "member@example.test", []string{models.RoleAccountant}, first[:], expires, admin.ID, emailDraft); err == nil {
		t.Fatal("invitation committed without its audit event")
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE email='member@example.test')+(SELECT count(*) FROM user_invitations)+(SELECT count(*) FROM email_outbox)+(SELECT count(*) FROM audit_events)`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("failed invitation left records: %d %v", rows, err)
	}
	unblock()
	member, err := repo.CreateInvitedUser(ctx, "Member", nil, "member@example.test", []string{models.RoleAccountant}, first[:], expires, admin.ID, emailDraft)
	if err != nil {
		t.Fatal(err)
	}
	block()
	if _, err := repo.ReissueInvitation(ctx, member.ID, second[:], expires, admin.ID, emailDraft); err == nil {
		t.Fatal("replacement committed without its audit event")
	}
	if _, err := repo.FindInvitation(ctx, first[:]); err != nil {
		t.Fatalf("failed replacement invalidated original invitation: %v", err)
	}
	if _, err := repo.FindInvitation(ctx, second[:]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("failed replacement left a usable token: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM email_outbox").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("failed replacement left email intent: %d %v", rows, err)
	}
	unblock()
	if _, err := repo.ReissueInvitation(ctx, member.ID, second[:], expires, admin.ID, emailDraft); err != nil {
		t.Fatal(err)
	}
	block()
	if _, err := repo.AcceptInvitation(ctx, second[:], "fixture password hash"); err == nil {
		t.Fatal("password committed without acceptance audit")
	}
	if _, err := repo.FindInvitation(ctx, second[:]); err != nil {
		t.Fatalf("failed acceptance changed password or consumed token: %v", err)
	}
	unblock()
	if _, err := repo.AcceptInvitation(ctx, second[:], "fixture password hash"); err != nil {
		t.Fatal(err)
	}
	events, total, err := repositories.NewAuditRepository(pool).List(ctx, repositories.AuditQuery{Limit: 10})
	if err != nil || total != 3 || len(events) != 3 {
		t.Fatalf("expected exactly one audit per committed action: %d %v", total, err)
	}
	for _, event := range events {
		actor := admin.ID
		if event.Action == audit.InvitationAccepted {
			actor = member.ID
		}
		if event.ActorID == nil || *event.ActorID != actor || event.TargetID == nil || *event.TargetID != member.ID || event.IP == nil || *event.IP != "203.0.113.7" {
			t.Fatalf("incomplete invitation audit: %+v", event)
		}
	}
}

func TestMerchantOwnerDeletionRequiresDeactivation(t *testing.T) {
	pool := isolatedPool(t)
	ctx := context.Background()
	users := repositories.NewUserRepository(pool)
	if _, err := users.Create(ctx, "Admin", nil, "admin@example.test", "fixture hash", []string{models.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	member, err := users.Create(ctx, "Merchant", nil, "merchant@example.test", "fixture hash", []string{models.RoleAccountant})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repositories.NewQuantumRepository(pool).CreateMerchant(ctx, member.ID, models.QuantumMerchantRequest{DisplayName: "Shop", ReceiverAddress: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}); err != nil {
		t.Fatal(err)
	}
	rbac := repositories.NewRBACRepository(pool)
	if err := rbac.DeleteUser(ctx, member.ID); !errors.Is(err, repositories.ErrUserHasMerchant) {
		t.Fatalf("merchant deletion must return a domain conflict: %v", err)
	}
	if _, err := users.FindByEmail(ctx, member.Email); err != nil {
		t.Fatal("failed deletion removed the owner")
	}
	disabled, err := rbac.SetUserActive(ctx, member.ID, false)
	if err != nil || disabled.IsActive {
		t.Fatalf("merchant deactivation: %v", err)
	}
	if _, err := repositories.NewQuantumRepository(pool).Merchant(ctx, member.ID); err != nil {
		t.Fatalf("deactivation removed merchant history: %v", err)
	}
}
