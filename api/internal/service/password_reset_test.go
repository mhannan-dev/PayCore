package service

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/mailer"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
)

type resetStore struct {
	users     map[string]model.ResetUser // by email
	throttled bool
	tokens    map[string]string // digest hex -> email
	reset     map[string]string // email -> password hash
}

func (s *resetStore) IssueResetToken(_ context.Context, email string, digest []byte, _ time.Time, _ time.Duration) (model.ResetUser, error) {
	u, ok := s.users[email]
	if !ok {
		return model.ResetUser{}, pgx.ErrNoRows
	}
	if s.throttled {
		return model.ResetUser{}, repository.ErrResetThrottled
	}
	s.tokens = map[string]string{hex.EncodeToString(digest): email}
	return u, nil
}
func (s *resetStore) FindResetToken(_ context.Context, digest []byte) (model.PasswordResetInfo, error) {
	email, ok := s.tokens[hex.EncodeToString(digest)]
	if !ok {
		return model.PasswordResetInfo{}, pgx.ErrNoRows
	}
	return model.PasswordResetInfo{Email: email}, nil
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

type memSender struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (m *memSender) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

var resetLink = regexp.MustCompile(`http://web\.test/reset-password#token=([0-9a-f]{64})`)

func newResetService(captchaOK bool) (*PasswordResetService, *resetStore, *memSender) {
	store := &resetStore{users: map[string]model.ResetUser{"rasel@example.test": {ID: "u1", Email: "rasel@example.test", FirstName: "Rasel"}}}
	sender := &memSender{}
	return NewPasswordResetService(store, okCaptcha{captchaOK}, sender, "http://web.test/"), store, sender
}

func settle(svc *PasswordResetService) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc.Wait(ctx)
}

func TestPasswordResetFlow(t *testing.T) {
	svc, store, sender := newResetService(true)
	ctx := context.Background()
	if err := svc.Request(ctx, model.PasswordResetRequest{Email: " Rasel@Example.TEST ", CaptchaID: "id", CaptchaAnswer: "1"}); err != nil {
		t.Fatal(err)
	}
	settle(svc)
	if len(sender.sent) != 1 || sender.sent[0].To != "rasel@example.test" || sender.sent[0].Subject != "Reset your PayCore password" {
		t.Fatalf("email: %+v", sender.sent)
	}
	m := resetLink.FindStringSubmatch(sender.sent[0].Text)
	if m == nil || !strings.Contains(sender.sent[0].HTML, m[0]) {
		t.Fatalf("email lacks the reset link:\n%s", sender.sent[0].Text)
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
	svc, store, sender := newResetService(true)
	ctx := context.Background()
	// Unknown email and a throttled request both succeed silently, without email.
	if err := svc.Request(ctx, model.PasswordResetRequest{Email: "nobody@example.test", CaptchaID: "id", CaptchaAnswer: "1"}); err != nil {
		t.Fatalf("unknown email: %v", err)
	}
	store.throttled = true
	if err := svc.Request(ctx, model.PasswordResetRequest{Email: "rasel@example.test", CaptchaID: "id", CaptchaAnswer: "1"}); err != nil {
		t.Fatalf("throttled: %v", err)
	}
	settle(svc)
	if len(sender.sent) != 0 {
		t.Fatalf("no email expected, got %d", len(sender.sent))
	}
}

func TestPasswordResetNeedsCaptcha(t *testing.T) {
	svc, store, _ := newResetService(false)
	if err := svc.Request(context.Background(), model.PasswordResetRequest{Email: "rasel@example.test"}); err != ErrCaptcha {
		t.Fatalf("captcha: %v", err)
	}
	if store.tokens != nil {
		t.Fatal("no token may be issued without a valid captcha")
	}
}
