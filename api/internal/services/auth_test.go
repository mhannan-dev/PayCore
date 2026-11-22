package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
)

type fakeCaptcha struct{ valid bool }

func (c *fakeCaptcha) Verify(string, string) bool { was := c.valid; c.valid = false; return was }

type fakeUsers struct {
	user    models.User
	missing bool
	hash    []byte
	revoked bool
	lookups int
}

func TestAdminCannotLoginWithoutMFAService(t *testing.T) {
	hash, err := HashPassword("Admin#1234")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeUsers{user: models.User{ID: "admin", Email: "admin@example.test", PasswordHash: hash, Roles: []string{models.RoleAdmin}, IsActive: true}}
	auth, err := NewAuthService(store, &fakeCaptcha{valid: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Login(context.Background(), models.LoginRequest{Email: store.user.Email, Password: "Admin#1234"}); !errors.Is(err, models.ErrMFARequired) || len(store.hash) != 0 {
		t.Fatal("admin obtained session without configured MFA")
	}
}

func TestExplicitMFAPauseAllowsAdminPasswordLogin(t *testing.T) {
	hash, err := HashPassword("Admin#1234")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeUsers{user: models.User{ID: "admin", Email: "paused@example.test", PasswordHash: hash, Roles: []string{models.RoleAdmin}, IsActive: true}}
	auth, err := NewAuthServiceWithMFAPolicy(store, &fakeCaptcha{valid: true}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := auth.Login(context.Background(), models.LoginRequest{Email: store.user.Email, Password: "Admin#1234"})
	if err != nil || result.MFA != nil || len(result.Token) != 64 || len(store.hash) == 0 {
		t.Fatalf("paused MFA blocked login: %+v %v", result.MFA, err)
	}
	if _, err = auth.CurrentUser(context.Background(), result.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Login(context.Background(), models.LoginRequest{Email: store.user.Email, Password: "Admin#1234"}); !errors.Is(err, ErrCaptcha) {
		t.Fatal("MFA pause bypassed CAPTCHA")
	}
	auth.captcha = &fakeCaptcha{valid: true}
	if _, err = auth.Login(context.Background(), models.LoginRequest{Email: store.user.Email, Password: "wrong password"}); !errors.Is(err, ErrCredentials) {
		t.Fatal("MFA pause bypassed password verification")
	}
	paused := NewMFAServiceWithMFAPolicy(nil, nil, false)
	if _, required, err := paused.Begin(context.Background(), store.user); err != nil || required {
		t.Fatal("paused MFA issued challenge")
	}
	if _, err := paused.Verify(context.Background(), models.MFAVerifyRequest{}); !errors.Is(err, models.ErrMFADisabled) {
		t.Fatal("paused MFA accepted verification")
	}
}

func (s *fakeUsers) FindByEmail(_ context.Context, email string) (models.User, error) {
	s.lookups++
	if s.missing || email != s.user.Email {
		return models.User{}, pgx.ErrNoRows
	}
	return s.user, nil
}
func (s *fakeUsers) CreateSession(_ context.Context, _ string, expected string, hash []byte, _ time.Time) error {
	if expected != s.user.PasswordHash {
		return models.ErrCredentialsChanged
	}
	s.hash = append([]byte(nil), hash...)
	return nil
}
func (s *fakeUsers) FindSession(_ context.Context, hash []byte) (models.User, error) {
	if s.revoked || string(hash) != string(s.hash) {
		return models.User{}, pgx.ErrNoRows
	}
	return s.user, nil
}
func (s *fakeUsers) DeleteSession(context.Context, []byte) error { s.revoked = true; return nil }
func (s *fakeUsers) UpdateName(_ context.Context, id, first string, last *string) (models.User, error) {
	if id != s.user.ID {
		return models.User{}, pgx.ErrNoRows
	}
	s.user.FirstName, s.user.LastName = first, last
	return s.user, nil
}

func TestLoginAndRevocation(t *testing.T) {
	hash, err := HashPassword("Exampl#e01")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeUsers{user: models.User{ID: "user-1", FirstName: "Test accountant", Email: "accountant@example.test", PasswordHash: hash, Roles: []string{"ACCOUNTANT", "COMPLIANCE_AUDITOR"}, IsActive: true}}
	captcha := &fakeCaptcha{}
	s, err := NewAuthService(store, captcha)
	if err != nil {
		t.Fatal(err)
	}
	req := models.LoginRequest{Email: "ACCOUNTANT@example.test", Password: "Exampl#e01"}
	if _, err = s.Login(context.Background(), req); !errors.Is(err, ErrCaptcha) || store.lookups != 0 {
		t.Fatal("CAPTCHA must be verified before user lookup")
	}
	captcha.valid = true
	response, err := s.Login(context.Background(), req)
	if err != nil || len(response.Token) != 64 || (len(response.User.Roles) != 2 || response.User.Roles[0] != "ACCOUNTANT" || response.User.Roles[1] != "COMPLIANCE_AUDITOR") {
		t.Fatal("valid login failed")
	}
	digest := sha256.Sum256([]byte(response.Token))
	if string(store.hash) != string(digest[:]) {
		t.Fatal("session token must be hashed before storage")
	}
	body, _ := json.Marshal(store.user)
	if strings.Contains(string(body), hash) || strings.Contains(string(body), "password") {
		t.Fatal("User JSON exposes password hash")
	}
	if _, err = s.CurrentUser(context.Background(), response.Token); err != nil {
		t.Fatal("session rejected")
	}
	if _, err = s.Login(context.Background(), req); !errors.Is(err, ErrCaptcha) {
		t.Fatal("replayed CAPTCHA accepted")
	}
	if err = s.Logout(context.Background(), response.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CurrentUser(context.Background(), response.Token); !errors.Is(err, ErrSession) {
		t.Fatal("revoked session accepted")
	}
	for _, scenario := range []string{"wrong password", "unknown email", "disabled user"} {
		t.Run(scenario, func(t *testing.T) {
			store.missing = scenario == "unknown email"
			store.user.IsActive = scenario != "disabled user"
			attempt := req
			if scenario == "wrong password" {
				attempt.Password = "incorrect-test-password"
			}
			captcha.valid = true
			if _, err := s.Login(context.Background(), attempt); !errors.Is(err, ErrCredentials) {
				t.Fatal("must reject with generic credentials error")
			}
		})
	}
}

func TestUpdateProfile(t *testing.T) {
	hash, err := HashPassword("Exampl#e01")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeUsers{user: models.User{ID: "user-1", FirstName: "Old", Email: "accountant@example.test", PasswordHash: hash, Roles: []string{"ACCOUNTANT"}, IsActive: true}}
	s, err := NewAuthService(store, &fakeCaptcha{valid: true})
	if err != nil {
		t.Fatal(err)
	}
	login, err := s.Login(context.Background(), models.LoginRequest{Email: "accountant@example.test", Password: "Exampl#e01"})
	if err != nil {
		t.Fatal(err)
	}
	last := "  Sato "
	if _, err := s.UpdateProfile(context.Background(), strings.Repeat("0", 64), models.UpdateProfileRequest{FirstName: "Kenji"}); !errors.Is(err, ErrSession) {
		t.Fatal("unknown session must be rejected before validation")
	}
	u, err := s.UpdateProfile(context.Background(), login.Token, models.UpdateProfileRequest{FirstName: " Kenji ", LastName: &last})
	if err != nil || u.FirstName != "Kenji" || u.LastName == nil || *u.LastName != "Sato" || u.Email != "accountant@example.test" {
		t.Fatalf("update failed: %+v %v", u, err)
	}
	blank := "   "
	if u, err = s.UpdateProfile(context.Background(), login.Token, models.UpdateProfileRequest{FirstName: "Kenji", LastName: &blank}); err != nil || u.LastName != nil {
		t.Fatal("blank last name must be stored as NULL")
	}
	for _, bad := range []models.UpdateProfileRequest{{FirstName: "  "}, {FirstName: strings.Repeat("a", 81)}, {FirstName: "ok", LastName: func() *string { v := strings.Repeat("b", 81); return &v }()}} {
		if _, err := s.UpdateProfile(context.Background(), login.Token, bad); !errors.Is(err, models.ErrInvalidProfile) {
			t.Fatalf("invalid profile accepted: %+v", bad)
		}
	}
}

// An invited user (no password yet) cannot log in with any password, including the
// internal dummy one, and the attempt is answered like any wrong password.
func TestLoginRejectsInvitedUserWithoutPassword(t *testing.T) {
	store := &fakeUsers{user: models.User{ID: "user-2", FirstName: "Invited", Email: "invited@example.test", Roles: []string{"ACCOUNTANT"}, IsActive: true}}
	captcha := &fakeCaptcha{}
	auth, err := NewAuthService(store, captcha)
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"", "unused-dummy-password-for-unknown-users", "anything-at-all-123"} {
		captcha.valid = true
		_, err := auth.Login(context.Background(), models.LoginRequest{Email: "invited@example.test", Password: password, CaptchaID: "id", CaptchaAnswer: "1"})
		if !errors.Is(err, ErrCredentials) {
			t.Fatalf("password %q: got %v", password, err)
		}
	}
	if store.hash != nil {
		t.Fatal("no session may be created")
	}
}

func TestHashPasswordLength(t *testing.T) {
	for _, tc := range []struct {
		password string
		ok       bool
	}{
		{"Test@12345", true},   // 10
		{"Pässwörd!1", true},   // 10 characters, 12 bytes
		{"Test@1234", false},   // 9
		{"Test@123456", false}, // 11
		{"", false},
		{"Test@1234\xff", false}, // invalid UTF-8
	} {
		_, err := HashPassword(tc.password)
		if (err == nil) != tc.ok {
			t.Errorf("%q: got %v", tc.password, err)
		}
	}
}

type memAudit struct{ events []audit.Event }

func (m *memAudit) Record(_ context.Context, e audit.Event) error {
	m.events = append(m.events, e)
	return nil
}

// Sign-ins are audited with a reason code, but every failure answers the same.
func TestLoginAudit(t *testing.T) {
	hash, err := HashPassword("Audit#1234")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeUsers{user: models.User{ID: "user-9", FirstName: "A", Email: "a@example.test", PasswordHash: hash, Roles: []string{"ACCOUNTANT"}, IsActive: true}}
	captcha := &fakeCaptcha{}
	auth, err := NewAuthService(store, captcha)
	if err != nil {
		t.Fatal(err)
	}
	rec := &memAudit{}
	auth.Audit = rec
	login := func(email, password string) error {
		captcha.valid = true
		_, err := auth.Login(context.Background(), models.LoginRequest{Email: email, Password: password, CaptchaID: "id", CaptchaAnswer: "1"})
		return err
	}
	if err := login("nobody@example.test", "Audit#1234"); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	if err := login("a@example.test", "Wrong#1234"); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	store.user.IsActive = false
	if err := login("a@example.test", "Audit#1234"); !errors.Is(err, ErrCredentials) {
		t.Fatal(err)
	}
	store.user.IsActive = true
	if err := login(" A@Example.TEST ", "Audit#1234"); err != nil {
		t.Fatal(err)
	}
	want := []string{"login_failed/unknown_email", "login_failed/wrong_password", "login_failed/account_deactivated", "login_succeeded/"}
	if len(rec.events) != len(want) {
		t.Fatalf("events: %+v", rec.events)
	}
	for i, e := range rec.events {
		if got := string(e.Action) + "/" + e.Details.Reason; got != want[i] {
			t.Errorf("event %d: %s, want %s", i, got, want[i])
		}
	}
	if last := rec.events[3]; last.ActorID == nil || *last.ActorID != "user-9" {
		t.Fatalf("success actor: %+v", last)
	}
}
