package services

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
)

type resetStore struct {
	users     map[string]models.ResetUser // by email
	throttled bool
	queued    []models.EmailContent
	tokens    map[string]string // digest hex -> email
	reset     map[string]string // email -> password hash
}

func (s *resetStore) IssueResetToken(_ context.Context, email string, digest []byte, _ time.Time, _ time.Duration, drafts ...models.EmailDraft) (models.ResetUser, error) {
	u, ok := s.users[email]
	if !ok {
		return models.ResetUser{}, pgx.ErrNoRows
	}
	if s.throttled {
		return models.ResetUser{}, repositories.ErrResetThrottled
	}
	if len(drafts) > 0 && drafts[0] != nil {
		content, err := drafts[0](models.EmailRecipient{ID: u.ID, Email: u.Email, FirstName: u.FirstName})
		if err != nil {
			return models.ResetUser{}, err
		}
		s.queued = append(s.queued, content)
	}
	s.tokens = map[string]string{hex.EncodeToString(digest): email}
	return u, nil
}
func (s *resetStore) FindResetToken(_ context.Context, digest []byte) (models.PasswordResetInfo, error) {
	email, ok := s.tokens[hex.EncodeToString(digest)]
	if !ok {
		return models.PasswordResetInfo{}, pgx.ErrNoRows
	}
	return models.PasswordResetInfo{Email: email}, nil
}
func (s *resetStore) ResetPassword(ctx context.Context, digest []byte, hash string) (string, error) {
	info, err := s.FindResetToken(ctx, digest)
	if err != nil {
		return "", err
	}
	delete(s.tokens, hex.EncodeToString(digest))
	if s.reset == nil {
		s.reset = map[string]string{}
	}
	s.reset[info.Email] = hash
	return info.Email, nil
}

type okCaptcha struct{ ok bool }

func (c okCaptcha) Verify(string, string) bool { return c.ok }

var resetLink = regexp.MustCompile(`http://web\.test/reset-password#token=([0-9a-f]{64})`)

func newResetService(captchaOK bool) (*PasswordResetService, *resetStore) {
	store := &resetStore{users: map[string]models.ResetUser{"rasel@example.test": {ID: "u1", Email: "rasel@example.test", FirstName: "Rasel"}}}
	return NewPasswordResetService(store, okCaptcha{captchaOK}, true, "http://web.test/"), store
}

func TestPasswordResetFlow(t *testing.T) {
	svc, store := newResetService(true)
	ctx := context.Background()
	if err := svc.Request(ctx, models.PasswordResetRequest{Email: " Rasel@Example.TEST ", CaptchaID: "id", CaptchaAnswer: "1"}); err != nil {
		t.Fatal(err)
	}
	if len(store.queued) != 1 || store.queued[0].To != "rasel@example.test" || store.queued[0].Subject != "Reset your PayCore password" {
		t.Fatalf("email: %+v", store.queued)
	}
	m := resetLink.FindStringSubmatch(store.queued[0].Text)
	if m == nil || !strings.Contains(store.queued[0].HTML, m[0]) {
		t.Fatalf("email lacks the reset link:\n%s", store.queued[0].Text)
	}
	token := m[1]
	if info, err := svc.Inspect(ctx, token); err != nil || info.Email != "rasel@example.test" {
		t.Fatalf("inspect: %+v %v", info, err)
	}
	if _, err := svc.Confirm(ctx, token, "too-short"); err != ErrWeakPassword {
		t.Fatalf("weak password: %v", err)
	}
	if email, err := svc.Confirm(ctx, token, "Reset#1234"); err != nil || email != "rasel@example.test" || !strings.HasPrefix(store.reset[email], "$2") {
		t.Fatalf("confirm: %q %v", email, err)
	}
	if _, err := svc.Confirm(ctx, token, "Reset#1234"); err != ErrResetToken {
		t.Fatalf("reuse must fail: %v", err)
	}
}

func TestPasswordResetDoesNotRevealAccounts(t *testing.T) {
	svc, store := newResetService(true)
	ctx := context.Background()
	// Unknown email and a throttled request both succeed silently, without email.
	if err := svc.Request(ctx, models.PasswordResetRequest{Email: "nobody@example.test", CaptchaID: "id", CaptchaAnswer: "1"}); err != nil {
		t.Fatalf("unknown email: %v", err)
	}
	store.throttled = true
	if err := svc.Request(ctx, models.PasswordResetRequest{Email: "rasel@example.test", CaptchaID: "id", CaptchaAnswer: "1"}); err != nil {
		t.Fatalf("throttled: %v", err)
	}
	if len(store.queued) != 0 {
		t.Fatalf("no email expected, got %d", len(store.queued))
	}
}

func TestPasswordResetNeedsCaptcha(t *testing.T) {
	svc, store := newResetService(false)
	if err := svc.Request(context.Background(), models.PasswordResetRequest{Email: "rasel@example.test"}); err != ErrCaptcha {
		t.Fatalf("captcha: %v", err)
	}
	if store.tokens != nil {
		t.Fatal("no token may be issued without a valid captcha")
	}
}

func TestPasswordResetWithoutMailer(t *testing.T) {
	svc, store := newResetService(true)
	svc.emailEnabled = false
	if err := svc.Request(context.Background(), models.PasswordResetRequest{Email: "rasel@example.test"}); err != nil {
		t.Fatal(err)
	}
	if len(store.queued) != 0 || len(store.tokens) != 1 {
		t.Fatal("unconfigured delivery should persist token without email content")
	}
}
