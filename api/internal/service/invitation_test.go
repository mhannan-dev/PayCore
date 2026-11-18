package service

import (
	"context"
	"errors"
	"net/textproto"
	"os"
	"sync"
	"testing"
	"time"

	"paycore/api/internal/mailer"
	"paycore/api/internal/model"
)

type invitationDeliveryStore struct {
	mu       sync.Mutex
	created  bool
	statuses []string // every RecordEmail status, in order
	lastErr  string
}

func (s *invitationDeliveryStore) CreateInvitedUser(_ context.Context, first string, last *string, email string, roles []string, digest []byte, expires time.Time, _ string) (model.ManagedUser, error) {
	s.created = true
	return model.ManagedUser{FirstName: first, Email: email, Roles: roles, InviteExpiresAt: &expires}, nil
}
func (*invitationDeliveryStore) ReissueInvitation(context.Context, string, []byte, time.Time, string) (model.ManagedUser, error) {
	return model.ManagedUser{}, errors.New("unused in delivery test")
}
func (*invitationDeliveryStore) FindInvitation(context.Context, []byte) (model.InvitationInfo, error) {
	return model.InvitationInfo{}, errors.New("unused in delivery test")
}
func (*invitationDeliveryStore) AcceptInvitation(context.Context, []byte, string) (string, error) {
	return "", errors.New("unused in delivery test")
}
func (s *invitationDeliveryStore) RecordEmail(_ context.Context, _ []byte, status, emailError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, status)
	s.lastErr = emailError
	return nil
}
func (s *invitationDeliveryStore) final() (string, []string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.statuses) == 0 {
		return "", nil, ""
	}
	return s.statuses[len(s.statuses)-1], append([]string(nil), s.statuses...), s.lastErr
}

// scriptedSender returns the queued errors in order, then success.
type scriptedSender struct {
	mu    sync.Mutex
	errs  []error
	calls int
}

func (s *scriptedSender) Send(context.Context, mailer.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if len(s.errs) == 0 {
		return nil
	}
	err := s.errs[0]
	s.errs = s.errs[1:]
	return err
}

func invite(t *testing.T, svc *InvitationService) string {
	t.Helper()
	_, status, err := svc.Invite(context.Background(), model.CreateUserRequest{FirstName: "Delivery Check", Email: "invitation-delivery-check@example.test", Roles: []string{"OPERATOR"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc.Wait(ctx)
	return status
}

func TestInvitationLocalSMTP(t *testing.T) {
	if os.Getenv("SMTP_TEST_HOST") != "127.0.0.1" {
		t.Skip("requires local Mailpit SMTP on 127.0.0.1")
	}
	port := os.Getenv("SMTP_TEST_PORT")
	if port == "" {
		port = "1025"
	}
	store := &invitationDeliveryStore{}
	sender := mailer.SMTP{Host: "127.0.0.1", Port: port, From: "PayCore <no-reply@paycore.local>"}
	svc := NewInvitationService(store, sender, "http://localhost:3000")
	if status := invite(t, svc); status != EmailPending || !store.created {
		t.Fatalf("queued status %q, created %t", status, store.created)
	}
	if final, all, errText := store.final(); final != EmailSent {
		t.Fatalf("Mailpit delivery: %v (%s)", all, errText)
	}
}

func TestInvitationLogIsNotDelivery(t *testing.T) {
	store := &invitationDeliveryStore{}
	svc := NewInvitationService(store, mailer.Log{}, "http://localhost:3000")
	invite(t, svc)
	if final, _, _ := store.final(); final != EmailNotConfigured || !store.created {
		t.Fatalf("log preview must keep the account and record not_configured, got %q", final)
	}
}

func TestInvitationWithoutMailer(t *testing.T) {
	store := &invitationDeliveryStore{}
	svc := NewInvitationService(store, nil, "http://localhost:3000")
	if status := invite(t, svc); status != EmailNotConfigured {
		t.Fatalf("status %q", status)
	}
	if final, _, _ := store.final(); final != EmailNotConfigured {
		t.Fatalf("recorded %q", final)
	}
}

func TestInvitationRetriesTemporaryFailures(t *testing.T) {
	store := &invitationDeliveryStore{}
	sender := &scriptedSender{errs: []error{errors.New("dial tcp: i/o timeout"), &textproto.Error{Code: 421, Msg: "try later"}}}
	svc := NewInvitationService(store, sender, "http://localhost:3000")
	svc.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	invite(t, svc)
	final, all, _ := store.final()
	if final != EmailSent || sender.calls != 3 || len(all) != 3 || all[0] != EmailPending {
		t.Fatalf("retry: calls=%d statuses=%v", sender.calls, all)
	}
}

func TestInvitationStopsOnPermanentFailure(t *testing.T) {
	store := &invitationDeliveryStore{}
	sender := &scriptedSender{errs: []error{&textproto.Error{Code: 535, Msg: "5.7.139 Authentication unsuccessful"}}}
	svc := NewInvitationService(store, sender, "http://localhost:3000")
	svc.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	invite(t, svc)
	final, all, errText := store.final()
	if final != EmailFailed || sender.calls != 1 || errText == "" {
		t.Fatalf("permanent: calls=%d statuses=%v err=%q", sender.calls, all, errText)
	}
}

func TestInvitationGivesUpAfterRetries(t *testing.T) {
	store := &invitationDeliveryStore{}
	down := errors.New("connection refused")
	sender := &scriptedSender{errs: []error{down, down, down, down}}
	svc := NewInvitationService(store, sender, "http://localhost:3000")
	svc.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	invite(t, svc)
	if final, all, _ := store.final(); final != EmailFailed || sender.calls != 3 {
		t.Fatalf("give up: calls=%d statuses=%v", sender.calls, all)
	}
}
