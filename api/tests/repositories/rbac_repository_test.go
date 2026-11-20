package repositories_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

func TestInvitationLifecycleAndRoles(t *testing.T) {
	pool := isolatedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	invites := repositories.NewInvitationRepository(pool)
	rbac := repositories.NewRBACRepository(pool)
	users := repositories.NewUserRepository(pool)
	// Keep paging and last-admin checks independent of existing database fixtures.
	adminEmail := fmt.Sprintf("fixture-admin-%d@example.test", time.Now().UnixNano())
	admin, err := users.Create(ctx, "Fixture Admin", nil, adminEmail, "$2a$12$fixture", []string{"ADMIN"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", admin.ID) })
	email := fmt.Sprintf("invite-%d@example.test", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", email) })
	digest := func(s string) []byte { d := sha256.Sum256([]byte(s)); return d[:] }
	expires := time.Now().Add(time.Hour)

	u, err := invites.CreateInvitedUser(ctx, "Invitee", nil, email, []string{"OPERATOR", "COMPLIANCE_AUDITOR"}, digest(email+"1"), expires, "", emailDraft)
	if err != nil || u.PasswordSet || u.InviteExpiresAt == nil || !u.IsActive {
		t.Fatalf("create: %+v %v", u, err)
	}
	var mirrored int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM user_roles WHERE user_id = $1", u.ID).Scan(&mirrored); err != nil || mirrored != 2 {
		t.Fatalf("user_roles mirror: %d %v", mirrored, err)
	}
	if _, err := invites.CreateInvitedUser(ctx, "Again", nil, email, []string{"OPERATOR"}, digest(email+"x"), expires, ""); !errors.Is(err, repositories.ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	// An invited user has no password: login lookup sees an empty hash.
	if found, err := users.FindByEmail(ctx, email); err != nil || found.PasswordHash != "" {
		t.Fatalf("invited user hash: %q %v", found.PasswordHash, err)
	}

	// Delivery outcome is recorded on the invitation and shown on the user.

	jobs := repositories.NewEmailOutboxRepository(pool)
	job, err := jobs.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobs.Complete(ctx, job, models.EmailOutcome{Status: models.EmailQueued, ErrorCode: "smtp_temporary_failure"}); err != nil {
		t.Fatal(err)
	}
	job, err = jobs.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobs.Complete(ctx, job, models.EmailOutcome{Status: models.EmailDelivered}); err != nil {
		t.Fatal(err)
	}
	if listed, _, err := rbac.ListUsers(ctx, models.UserQuery{Search: email, Limit: 1}); err != nil || len(listed) != 1 ||
		listed[0].InviteEmailStatus == nil || *listed[0].InviteEmailStatus != "sent" || listed[0].InviteEmailSentAt == nil || listed[0].InviteEmailError != nil {
		t.Fatalf("email status on user: %+v %v", listed, err)
	}
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT email_attempts FROM user_invitations WHERE token_hash = $1", digest(email+"1")).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("attempts: %d %v", attempts, err)
	}

	// Search, role filter and paging.
	page, total, err := rbac.ListUsers(ctx, models.UserQuery{Search: strings.ToUpper(email), Role: "COMPLIANCE_AUDITOR", Limit: 10})
	if err != nil || total != 1 || len(page) != 1 || page[0].ID != u.ID {
		t.Fatalf("search: %d %+v %v", total, page, err)
	}
	if _, total, err := rbac.ListUsers(ctx, models.UserQuery{Search: email, Role: "STAFF_ENGINEER", Limit: 10}); err != nil || total != 0 {
		t.Fatalf("role filter: %d %v", total, err)
	}
	if _, total, err := rbac.ListUsers(ctx, models.UserQuery{Search: "invite_%", Limit: 10}); err != nil || total != 0 {
		t.Fatalf("wildcards must match literally: %d %v", total, err)
	}
	all, total, err := rbac.ListUsers(ctx, models.UserQuery{Limit: 1, Offset: 0})
	if err != nil || len(all) != 1 || total < 2 {
		t.Fatalf("paging: %d %d %v", len(all), total, err)
	}
	if rest, _, err := rbac.ListUsers(ctx, models.UserQuery{Limit: 1, Offset: total}); err != nil || len(rest) != 0 {
		t.Fatalf("past the end: %d %v", len(rest), err)
	}

	// Re-sending replaces the first link.
	if _, err := invites.ReissueInvitation(ctx, u.ID, digest(email+"2"), expires, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := invites.FindInvitation(ctx, digest(email+"1")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("old link should be revoked: %v", err)
	}
	if info, err := invites.FindInvitation(ctx, digest(email+"2")); err != nil || info.Email != email || info.FirstName != "Invitee" {
		t.Fatalf("inspect: %+v %v", info, err)
	}

	if got, err := invites.AcceptInvitation(ctx, digest(email+"2"), "$2a$12$hash"); err != nil || got != email {
		t.Fatalf("accept: %q %v", got, err)
	}
	if _, err := invites.AcceptInvitation(ctx, digest(email+"2"), "$2a$12$other"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second accept must fail: %v", err)
	}
	if _, err := invites.ReissueInvitation(ctx, u.ID, digest(email+"3"), expires, ""); !errors.Is(err, repositories.ErrAlreadyActivated) {
		t.Fatalf("reissue after activation: %v", err)
	}

	u, err = rbac.SetUserRoles(ctx, u.ID, []string{"STAFF_ENGINEER"})
	if err != nil || strings.Join(u.Roles, ",") != "STAFF_ENGINEER" || !u.PasswordSet || u.InviteExpiresAt != nil {
		t.Fatalf("set roles: %+v %v", u, err)
	}

	if err := rbac.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM users WHERE id = $1) + (SELECT count(*) FROM user_roles WHERE user_id = $1) + (SELECT count(*) FROM user_invitations WHERE user_id = $1)", u.ID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("delete should cascade, %d rows left (%v)", left, err)
	}
	if err := rbac.DeleteUser(ctx, u.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second delete: %v", err)
	}

	// Expired links do not work.
	expired := fmt.Sprintf("expired-%d@example.test", time.Now().UnixNano())
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE email = $1", expired) })
	if _, err := invites.CreateInvitedUser(ctx, "Late", nil, expired, []string{"OPERATOR"}, digest(expired), time.Now().Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE user_invitations SET created_at = NOW() - interval '2 hours', expires_at = NOW() - interval '1 second' WHERE token_hash = $1", digest(expired)); err != nil {
		t.Fatal(err)
	}
	if _, err := invites.AcceptInvitation(ctx, digest(expired), "$2a$12$hash"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired accept must fail: %v", err)
	}
}
