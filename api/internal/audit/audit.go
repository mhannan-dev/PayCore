// Package audit defines PayCore's append-only audit trail: who did what, to whom,
// when and from where. Events are stored in audit_events (migration 013), which the
// database refuses to update, delete or truncate.
package audit

import (
	"context"
	"log"
	"strings"
	"time"
)

// Action is one kind of audited event. The set is closed and mirrored by a CHECK
// constraint and by the web's AuditAction union.
type Action string

const (
	LoginSucceeded         Action = "login_succeeded"
	LoginFailed            Action = "login_failed"
	Logout                 Action = "logout"
	UserInvited            Action = "user_invited"
	InvitationResent       Action = "invitation_resent"
	InvitationAccepted     Action = "invitation_accepted"
	RolesChanged           Action = "roles_changed"
	UserDeactivated        Action = "user_deactivated"
	UserActivated          Action = "user_activated"
	UserDeleted            Action = "user_deleted"
	PasswordResetRequested Action = "password_reset_requested"
	PasswordResetCompleted Action = "password_reset_completed"
)

// Actions lists every Action, for validation and filters.
var Actions = []Action{
	LoginSucceeded, LoginFailed, Logout, UserInvited, InvitationResent, InvitationAccepted,
	RolesChanged, UserDeactivated, UserActivated, UserDeleted, PasswordResetRequested, PasswordResetCompleted,
}

// Valid reports whether a is a known action.
func (a Action) Valid() bool {
	for _, known := range Actions {
		if a == known {
			return true
		}
	}
	return false
}

// Details holds action-specific facts. Fields are optional; never put secrets here.
type Details struct {
	RolesBefore []string `json:"roles_before,omitempty"`
	RolesAfter  []string `json:"roles_after,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}

// Event is one audit record. ID and OccurredAt are set by the database.
type Event struct {
	ID          int64     `json:"id"`
	OccurredAt  time.Time `json:"occurred_at"`
	Action      Action    `json:"action"`
	ActorID     *string   `json:"actor_user_id"`
	ActorEmail  *string   `json:"actor_email"`
	TargetID    *string   `json:"target_user_id"`
	TargetEmail *string   `json:"target_email"`
	Details     Details   `json:"details"`
	IP          *string   `json:"ip"`
	UserAgent   *string   `json:"user_agent"`
}

// Recorder stores events (implemented by repository.AuditRepository).
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// Record stores e if rec is set, logging (not returning) failures: used after an
// action has already succeeded, where failing the request would mislead the user.
// Admin changes are recorded inside their own database transaction instead.
func Record(ctx context.Context, rec Recorder, e Event) {
	if rec == nil {
		return
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := rec.Record(auditCtx, e); err != nil {
		log.Printf("audit: %s event not recorded: %v", e.Action, err)
	}
}

// Ptr returns a pointer to s, or nil for "" (absent).
func Ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Request context -------------------------------------------------------------

type metaKey struct{}
type actorKey struct{}

// Meta is where a request came from.
type Meta struct{ IP, UserAgent string }

// Actor is the authenticated user performing an action.
type Actor struct{ ID, Email string }

// WithMeta attaches request metadata (set by router middleware for every request).
func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

// MetaFrom returns the request metadata, if any.
func MetaFrom(ctx context.Context) Meta {
	m, _ := ctx.Value(metaKey{}).(Meta)
	return m
}

// WithActor attaches the authenticated user (set by authentication middleware).
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom returns the authenticated user, if any.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok && a.ID != ""
}

// Complete fills the actor (when the event has none) and request metadata from ctx.
func Complete(ctx context.Context, e Event) Event {
	if e.ActorID == nil && e.ActorEmail == nil {
		if a, ok := ActorFrom(ctx); ok {
			e.ActorID, e.ActorEmail = Ptr(a.ID), Ptr(a.Email)
		}
	}
	m := MetaFrom(ctx)
	if e.IP == nil {
		e.IP = Ptr(m.IP)
	}
	if e.UserAgent == nil {
		characters := []rune(strings.ToValidUTF8(m.UserAgent, ""))
		if len(characters) > 300 {
			characters = characters[:300]
		}
		ua := string(characters)
		e.UserAgent = Ptr(ua)
	}
	return e
}
