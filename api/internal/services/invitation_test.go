package services

import (
	"context"
	"errors"
	"paycore/api/internal/models"
	"strings"
	"testing"
	"time"
)

type invitationDeliveryStore struct {
	draft      models.EmailContent
	draftCount int
	fail       bool
}

func (s *invitationDeliveryStore) save(first, email string, expires time.Time, drafts []models.EmailDraft) (models.ManagedUser, error) {
	u := models.ManagedUser{ID: "user", FirstName: first, Email: email, InviteExpiresAt: &expires}
	if s.fail {
		return models.ManagedUser{}, errors.New("transaction rejected")
	}
	if len(drafts) > 0 && drafts[0] != nil {
		content, err := drafts[0](models.EmailRecipient{ID: u.ID, FirstName: first, Email: email})
		if err != nil {
			return models.ManagedUser{}, err
		}
		s.draft = content
		s.draftCount++
	}
	return u, nil
}
func (s *invitationDeliveryStore) CreateInvitedUser(_ context.Context, first string, _ *string, email string, _ []string, _ []byte, expires time.Time, _ string, draft ...models.EmailDraft) (models.ManagedUser, error) {
	return s.save(first, email, expires, draft)
}
func (s *invitationDeliveryStore) ReissueInvitation(_ context.Context, _ string, _ []byte, expires time.Time, _ string, draft ...models.EmailDraft) (models.ManagedUser, error) {
	return s.save("Member", "member@example.test", expires, draft)
}
func (*invitationDeliveryStore) FindInvitation(context.Context, []byte) (models.InvitationInfo, error) {
	return models.InvitationInfo{}, errors.New("unused")
}
func (*invitationDeliveryStore) AcceptInvitation(context.Context, []byte, string) (string, error) {
	return "", errors.New("unused")
}
func TestInvitationPersistsRenderedEmail(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		store := &invitationDeliveryStore{}
		svc := NewInvitationService(store, enabled, "http://web.test/")
		_, status, err := svc.Invite(context.Background(), models.CreateUserRequest{FirstName: "Mina", Email: "mina@example.test", Roles: []string{"OPERATOR"}}, "")
		if err != nil {
			t.Fatal(err)
		}
		if enabled {
			if status != EmailPending || store.draftCount != 1 || !strings.Contains(store.draft.Text, "http://web.test/set-password#token=") {
				t.Fatalf("missing durable draft: %q %+v", status, store.draft)
			}
		} else if status != EmailNotConfigured || store.draftCount != 0 {
			t.Fatal("disabled delivery created an email body")
		}
		_, status, err = svc.Resend(context.Background(), "user", "")
		if err != nil {
			t.Fatal(err)
		}
		if enabled && (store.draftCount != 2 || store.draft.To != "member@example.test" || status != EmailPending) {
			t.Fatal("resend did not store replacement email")
		}
	}
}
func TestInvitationPersistenceFailureIsReturned(t *testing.T) {
	store := &invitationDeliveryStore{fail: true}
	svc := NewInvitationService(store, true, "http://web.test")
	if _, _, err := svc.Invite(context.Background(), models.CreateUserRequest{}, ""); err == nil {
		t.Fatal("failed transaction reported success")
	}
	if store.draftCount != 0 {
		t.Fatal("email escaped failed transaction")
	}
}
