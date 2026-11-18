package handler_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/handler"
	"paycore/api/internal/mailer"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/internal/server"
	"paycore/api/internal/service"
)

type rbacStore struct {
	lastQuery model.UserQuery
	users     map[string]model.ManagedUser
	saved     []string
	lastErr   error
}

func (s *rbacStore) ListRoleDetails(context.Context) ([]model.RoleDetail, error) {
	return []model.RoleDetail{{Role: model.Role{Code: model.RoleAdmin}, Permissions: []string{model.PermUsersRead}, UserCount: 1}}, nil
}
func (s *rbacStore) ListPermissions(context.Context) ([]model.Permission, error) { return nil, nil }
func (s *rbacStore) ListUsers(_ context.Context, q model.UserQuery) ([]model.ManagedUser, int, error) {
	s.lastQuery = q
	out := []model.ManagedUser{}
	for _, u := range s.users {
		out = append(out, u)
	}
	return out, len(out), nil
}
func (s *rbacStore) DeleteUser(_ context.Context, id string) error {
	if s.lastErr != nil {
		return s.lastErr
	}
	if _, ok := s.users[id]; !ok {
		return pgx.ErrNoRows
	}
	delete(s.users, id)
	return nil
}
func (s *rbacStore) SetUserRoles(_ context.Context, id string, roles []string) (model.ManagedUser, error) {
	if s.lastErr != nil {
		return model.ManagedUser{}, s.lastErr
	}
	u, ok := s.users[id]
	if !ok {
		return model.ManagedUser{}, pgx.ErrNoRows
	}
	s.saved = roles
	u.Roles = roles
	return u, nil
}

const (
	adminID  = "11111111-1111-4111-8111-111111111111"
	memberID = "22222222-2222-4222-8222-222222222222"
)

func rbacRouter(t *testing.T, caller model.User) (*testRouter, *rbacStore, string) {
	r, repo, _, _, token := rbacRouterWithInvites(t, caller)
	return r, repo, token
}

func rbacRouterWithInvites(t *testing.T, caller model.User) (*testRouter, *rbacStore, *inviteStore, *captureMail, string) {
	t.Helper()
	token := strings.Repeat("a", 64)
	digest := sha256.Sum256([]byte(token))
	store := &testStore{user: caller, hash: digest[:]}
	auth, err := service.NewAuthService(store, &testCaptcha{})
	if err != nil {
		t.Fatal(err)
	}
	repo := &rbacStore{users: map[string]model.ManagedUser{
		adminID:  {ID: adminID, Email: "admin@example.test", Roles: []string{"ADMIN", "OPERATOR"}, IsActive: true},
		memberID: {ID: memberID, Email: "member@example.test", Roles: []string{"OPERATOR"}, IsActive: true},
	}}
	invites := &inviteStore{taken: map[string]bool{"member@example.test": true}}
	mail := &captureMail{}
	svc := service.NewInvitationService(invites, mail, "http://web.test/")
	svc.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	invites.svc = svc
	r := server.SetupRouterWithAttachments(nil, handler.NewAuthHandler(auth, nil), nil, handler.NewRBACHandler(auth, repo, svc))
	return &testRouter{r}, repo, invites, mail, token
}

type testRouter struct{ h http.Handler }

func (g *testRouter) do(method, path, body, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	g.h.ServeHTTP(w, r)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return body.Error
}

var adminCaller = model.User{ID: adminID, FirstName: "Admin", Email: "admin@example.test", Roles: []string{"ADMIN"}, IsActive: true,
	Permissions: []string{model.PermUsersRead, model.PermUsersWrite}}

func TestRBACRequiresSessionAndPermission(t *testing.T) {
	r, _, token := rbacRouter(t, model.User{ID: memberID, Roles: []string{"OPERATOR"}, IsActive: true, Permissions: []string{model.PermLedgerRead}})
	if w := r.do(http.MethodGet, "/api/v1/users", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d", w.Code)
	}
	w := r.do(http.MethodGet, "/api/v1/users", "", token)
	if w.Code != http.StatusForbidden || errorCode(t, w) != "permission_denied" {
		t.Fatalf("missing permission: got %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodGet, "/api/v1/rbac/roles", "", token); w.Code != http.StatusForbidden {
		t.Fatalf("roles without users:read: got %d", w.Code)
	}
}

func TestRBACReadOnlyCannotChangeRoles(t *testing.T) {
	r, repo, token := rbacRouter(t, model.User{ID: memberID, Roles: []string{"LEAD_OPERATOR"}, IsActive: true, Permissions: []string{model.PermUsersRead}})
	if w := r.do(http.MethodGet, "/api/v1/users", "", token); w.Code != http.StatusOK {
		t.Fatalf("users:read list: got %d", w.Code)
	}
	w := r.do(http.MethodPut, "/api/v1/users/"+memberID+"/roles", `{"roles":["ADMIN"]}`, token)
	if w.Code != http.StatusForbidden || repo.saved != nil {
		t.Fatalf("users:read must not write: got %d, saved %v", w.Code, repo.saved)
	}
}

func TestRBACSetRoles(t *testing.T) {
	r, repo, token := rbacRouter(t, adminCaller)
	w := r.do(http.MethodPut, "/api/v1/users/"+memberID+"/roles", `{"roles":["operator"," COMPLIANCE_AUDITOR ","OPERATOR"]}`, token)
	if w.Code != http.StatusOK || strings.Join(repo.saved, ",") != "OPERATOR,COMPLIANCE_AUDITOR" {
		t.Fatalf("set roles: got %d %s, saved %v", w.Code, w.Body, repo.saved)
	}

	cases := []struct {
		name, id, body, code string
		status               int
	}{
		{"empty roles", memberID, `{"roles":[]}`, "invalid_roles", http.StatusBadRequest},
		{"unknown role", memberID, `{"roles":["ROOT"]}`, "invalid_roles", http.StatusBadRequest},
		{"bad id", "not-a-uuid", `{"roles":["OPERATOR"]}`, "user_not_found", http.StatusNotFound},
		{"unknown user", "33333333-3333-4333-8333-333333333333", `{"roles":["OPERATOR"]}`, "user_not_found", http.StatusNotFound},
		{"own admin", adminID, `{"roles":["OPERATOR"]}`, "cannot_remove_own_admin", http.StatusConflict},
	}
	for _, tc := range cases {
		w := r.do(http.MethodPut, "/api/v1/users/"+tc.id+"/roles", tc.body, token)
		if w.Code != tc.status || errorCode(t, w) != tc.code {
			t.Errorf("%s: got %d %s", tc.name, w.Code, w.Body)
		}
	}

	repo.lastErr = repository.ErrLastAdmin
	w = r.do(http.MethodPut, "/api/v1/users/"+memberID+"/roles", `{"roles":["OPERATOR"]}`, token)
	if w.Code != http.StatusConflict || errorCode(t, w) != "last_admin" {
		t.Fatalf("last admin: got %d %s", w.Code, w.Body)
	}
}

func TestMeIncludesPermissions(t *testing.T) {
	r, _, token := rbacRouter(t, adminCaller)
	w := r.do(http.MethodGet, "/api/v1/auth/me", "", token)
	var body model.UserDTO
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusOK {
		t.Fatalf("me: %d %s", w.Code, w.Body)
	}
	if strings.Join(body.Permissions, ",") != "users:read,users:write" {
		t.Fatalf("permissions: %v", body.Permissions)
	}
}

// inviteStore is an in-memory InvitationStore.
type inviteStore struct {
	svc       *service.InvitationService
	emailMu   sync.Mutex
	emails    []string
	taken     map[string]bool
	created   []model.ManagedUser
	tokens    map[string]string // sha256 hex -> email
	activated map[string]bool
}

func (s *inviteStore) CreateInvitedUser(_ context.Context, first string, last *string, email string, roles []string, digest []byte, expires time.Time, by string) (model.ManagedUser, error) {
	if s.taken[email] {
		return model.ManagedUser{}, repository.ErrEmailTaken
	}
	if s.tokens == nil {
		s.tokens = map[string]string{}
	}
	s.tokens[hex.EncodeToString(digest)] = email
	u := model.ManagedUser{ID: "44444444-4444-4444-8444-444444444444", FirstName: first, LastName: last, Email: email, Roles: roles, IsActive: true, InviteExpiresAt: &expires}
	s.created = append(s.created, u)
	return u, nil
}
func (s *inviteStore) ReissueInvitation(_ context.Context, id string, digest []byte, expires time.Time, by string) (model.ManagedUser, error) {
	if id == adminID {
		return model.ManagedUser{}, repository.ErrAlreadyActivated
	}
	if id != memberID {
		return model.ManagedUser{}, pgx.ErrNoRows
	}
	s.tokens = map[string]string{hex.EncodeToString(digest): "member@example.test"}
	return model.ManagedUser{ID: id, FirstName: "Member", Email: "member@example.test", InviteExpiresAt: &expires}, nil
}
func (s *inviteStore) FindInvitation(_ context.Context, digest []byte) (model.InvitationInfo, error) {
	email, ok := s.tokens[hex.EncodeToString(digest)]
	if !ok || s.activated[email] {
		return model.InvitationInfo{}, pgx.ErrNoRows
	}
	return model.InvitationInfo{Email: email, FirstName: "Mina"}, nil
}
func (s *inviteStore) AcceptInvitation(ctx context.Context, digest []byte, hash string) (string, error) {
	info, err := s.FindInvitation(ctx, digest)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(hash, "$2") {
		return "", errors.New("password was not hashed")
	}
	if s.activated == nil {
		s.activated = map[string]bool{}
	}
	s.activated[info.Email] = true
	return info.Email, nil
}

func (s *inviteStore) RecordEmail(_ context.Context, _ []byte, status, _ string) error {
	s.emailMu.Lock()
	defer s.emailMu.Unlock()
	s.emails = append(s.emails, status)
	return nil
}

// settle waits for background invitation emails and returns the recorded statuses.
func (s *inviteStore) settle(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.svc.Wait(ctx)
	s.emailMu.Lock()
	defer s.emailMu.Unlock()
	return append([]string(nil), s.emails...)
}

// captureMail records sent messages; fail makes Send return an error.
type captureMail struct {
	mu   sync.Mutex
	sent []mailer.Message
	fail bool
}

func (m *captureMail) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("smtp down")
	}
	m.sent = append(m.sent, msg)
	return nil
}

var linkToken = regexp.MustCompile(`http://web\.test/set-password#token=([0-9a-f]{64})`)

func TestRBACInviteUser(t *testing.T) {
	r, _, invites, mail, token := rbacRouterWithInvites(t, adminCaller)
	body := `{"first_name":" Mina ","last_name":"  ","email":"Mina@Example.TEST","roles":["auditor_typo"]}`
	if w := r.do(http.MethodPost, "/api/v1/users", body, token); w.Code != http.StatusBadRequest || errorCode(t, w) != "invalid_user" {
		t.Fatalf("bad role: got %d %s", w.Code, w.Body)
	}
	body = strings.Replace(body, "auditor_typo", "compliance_auditor", 1)
	w := r.do(http.MethodPost, "/api/v1/users", body, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("invite: got %d %s", w.Code, w.Body)
	}
	var created struct {
		User        model.ManagedUser `json:"user"`
		EmailStatus string            `json:"email_status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	u := created.User
	if created.EmailStatus != "pending" || u.FirstName != "Mina" || u.LastName != nil || u.Email != "mina@example.test" || strings.Join(u.Roles, ",") != "COMPLIANCE_AUDITOR" {
		t.Fatalf("invited user: %+v", created)
	}
	if got := invites.settle(t); len(got) != 1 || got[0] != "sent" {
		t.Fatalf("recorded delivery: %v", got)
	}
	if len(mail.sent) != 1 || mail.sent[0].To != "mina@example.test" {
		t.Fatalf("email: %+v", mail.sent)
	}
	m := linkToken.FindStringSubmatch(mail.sent[0].Text)
	if m == nil || !strings.Contains(mail.sent[0].HTML, m[0]) {
		t.Fatalf("email lacks a set-password link:\n%s", mail.sent[0].Text)
	}
	if strings.Contains(w.Body.String(), m[1]) {
		t.Fatal("API response must not contain the invitation token")
	}

	// The emailed token works once on the public endpoints.
	link := m[1]
	if w := r.do(http.MethodPost, "/api/v1/auth/invitations/inspect", `{"token":"`+link+`"}`, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "mina@example.test") {
		t.Fatalf("inspect: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodPost, "/api/v1/auth/invitations/accept", `{"token":"`+link+`","password":"short"}`, ""); w.Code != http.StatusBadRequest || errorCode(t, w) != "invalid_password" {
		t.Fatalf("weak password: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodPost, "/api/v1/auth/invitations/accept", `{"token":"`+link+`","password":"Horse#4242"}`, ""); w.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodPost, "/api/v1/auth/invitations/accept", `{"token":"`+link+`","password":"Horse#4242"}`, ""); w.Code != http.StatusNotFound || errorCode(t, w) != "invitation_invalid" {
		t.Fatalf("reuse must fail: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodPost, "/api/v1/auth/invitations/inspect", `{"token":"nope"}`, ""); w.Code != http.StatusNotFound {
		t.Fatalf("malformed token: %d", w.Code)
	}

	cases := []struct {
		name, body, code string
		status           int
	}{
		{"duplicate email", `{"first_name":"A","email":"member@example.test","roles":["OPERATOR"]}`, "email_taken", http.StatusConflict},
		{"bad email", `{"first_name":"A","email":"not-an-email","roles":["OPERATOR"]}`, "invalid_user", http.StatusBadRequest},
		{"no first name", `{"first_name":"  ","email":"a@example.test","roles":["OPERATOR"]}`, "invalid_user", http.StatusBadRequest},
		{"no roles", `{"first_name":"A","email":"a@example.test","roles":[]}`, "invalid_user", http.StatusBadRequest},
		{"not json", `nope`, "invalid_request", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if w := r.do(http.MethodPost, "/api/v1/users", tc.body, token); w.Code != tc.status || errorCode(t, w) != tc.code {
			t.Errorf("%s: got %d %s", tc.name, w.Code, w.Body)
		}
	}
	if len(invites.created) != 1 {
		t.Fatalf("only the valid invite should be stored, got %d", len(invites.created))
	}
}

func TestRBACInviteReportsEmailFailure(t *testing.T) {
	r, _, invites, mail, token := rbacRouterWithInvites(t, adminCaller)
	mail.fail = true
	w := r.do(http.MethodPost, "/api/v1/users", `{"first_name":"A","email":"a@example.test","roles":["OPERATOR"]}`, token)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"email_status":"pending"`) || len(invites.created) != 1 {
		t.Fatalf("send failure: %d %s", w.Code, w.Body)
	}
	// Three attempts (two retries), then recorded as failed; the account still exists.
	if got := invites.settle(t); strings.Join(got, ",") != "pending,pending,failed" {
		t.Fatalf("recorded delivery: %v", got)
	}
}

func TestRBACResendInvitation(t *testing.T) {
	r, _, invites, mail, token := rbacRouterWithInvites(t, adminCaller)
	if w := r.do(http.MethodPost, "/api/v1/users/"+memberID+"/invitation", "", token); w.Code != http.StatusOK || len(invites.settle(t)) != 1 || len(mail.sent) != 1 {
		t.Fatalf("resend: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodPost, "/api/v1/users/"+adminID+"/invitation", "", token); w.Code != http.StatusConflict || errorCode(t, w) != "already_activated" {
		t.Fatalf("activated: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodPost, "/api/v1/users/33333333-3333-4333-8333-333333333333/invitation", "", token); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
}

func TestRBACInviteNeedsWrite(t *testing.T) {
	r, _, invites, _, token := rbacRouterWithInvites(t, model.User{ID: memberID, Roles: []string{"LEAD_OPERATOR"}, IsActive: true, Permissions: []string{model.PermUsersRead}})
	w := r.do(http.MethodPost, "/api/v1/users", `{"first_name":"A","email":"a@example.test","roles":["ADMIN"]}`, token)
	if w.Code != http.StatusForbidden || len(invites.created) != 0 {
		t.Fatalf("users:read must not invite: got %d", w.Code)
	}
	if w := r.do(http.MethodPost, "/api/v1/users/"+memberID+"/invitation", "", token); w.Code != http.StatusForbidden {
		t.Fatalf("users:read must not resend: got %d", w.Code)
	}
}

func TestRBACDeleteUser(t *testing.T) {
	r, repo, token := rbacRouter(t, adminCaller)
	if w := r.do(http.MethodDelete, "/api/v1/users/"+adminID, "", token); w.Code != http.StatusConflict || errorCode(t, w) != "cannot_delete_self" {
		t.Fatalf("self delete: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodDelete, "/api/v1/users/not-a-uuid", "", token); w.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := r.do(http.MethodDelete, "/api/v1/users/33333333-3333-4333-8333-333333333333", "", token); w.Code != http.StatusNotFound || errorCode(t, w) != "user_not_found" {
		t.Fatalf("unknown: %d %s", w.Code, w.Body)
	}
	if w := r.do(http.MethodDelete, "/api/v1/users/"+memberID, "", token); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, ok := repo.users[memberID]; ok {
		t.Fatal("user should be removed")
	}
	repo.users[memberID] = model.ManagedUser{ID: memberID}
	repo.lastErr = repository.ErrLastAdmin
	if w := r.do(http.MethodDelete, "/api/v1/users/"+memberID, "", token); w.Code != http.StatusConflict || errorCode(t, w) != "last_admin" {
		t.Fatalf("last admin: %d %s", w.Code, w.Body)
	}
}

func TestRBACDeleteUserNeedsWrite(t *testing.T) {
	r, repo, token := rbacRouter(t, model.User{ID: memberID, Roles: []string{"LEAD_OPERATOR"}, IsActive: true, Permissions: []string{model.PermUsersRead}})
	if w := r.do(http.MethodDelete, "/api/v1/users/"+adminID, "", token); w.Code != http.StatusForbidden || len(repo.users) != 2 {
		t.Fatalf("users:read must not delete: %d", w.Code)
	}
}

func TestRBACUsersPaging(t *testing.T) {
	r, repo, token := rbacRouter(t, adminCaller)
	w := r.do(http.MethodGet, "/api/v1/users?q=%20Mina%20&role=compliance_auditor&limit=5&offset=10", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if q := repo.lastQuery; q.Search != "Mina" || q.Role != "COMPLIANCE_AUDITOR" || q.Limit != 5 || q.Offset != 10 {
		t.Fatalf("query: %+v", q)
	}
	var body struct {
		Items                []model.ManagedUser
		Limit, Offset, Total int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Total != 2 || body.Limit != 5 || body.Offset != 10 || len(body.Items) != 2 {
		t.Fatalf("envelope: %+v %v", body, err)
	}
	if w := r.do(http.MethodGet, "/api/v1/users", "", token); w.Code != http.StatusOK || repo.lastQuery.Limit != 20 || repo.lastQuery.Offset != 0 {
		t.Fatalf("defaults: %d %+v", w.Code, repo.lastQuery)
	}
	for _, bad := range []string{"limit=0", "limit=101", "limit=x", "offset=-1", "role=ROOT", "q=" + strings.Repeat("a", 101)} {
		if w := r.do(http.MethodGet, "/api/v1/users?"+bad, "", token); w.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d", bad, w.Code)
		}
	}
}
