package controllers_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/models"
	"paycore/api/internal/server"
	"paycore/api/internal/services"
)

type testCaptcha struct{ valid bool }

func (v *testCaptcha) Verify(string, string) bool { old := v.valid; v.valid = false; return old }

type testStore struct {
	user models.User
	hash []byte
}

func (s *testStore) FindByEmail(context.Context, string) (models.User, error) { return s.user, nil }
func (s *testStore) CreateSession(_ context.Context, _ string, _ string, hash []byte, _ time.Time) error {
	s.hash = append([]byte(nil), hash...)
	return nil
}
func (s *testStore) FindSession(_ context.Context, hash []byte) (models.User, error) {
	if string(hash) != string(s.hash) {
		return models.User{}, pgx.ErrNoRows
	}
	return s.user, nil
}
func (s *testStore) DeleteSession(context.Context, []byte) error { s.hash = nil; return nil }
func (s *testStore) UpdateName(_ context.Context, _ string, first string, last *string) (models.User, error) {
	s.user.FirstName, s.user.LastName = first, last
	return s.user, nil
}

func TestLoginHTTPContract(t *testing.T) {
	hash, err := services.HashPassword("Sample#h01")
	if err != nil {
		t.Fatal(err)
	}
	store := &testStore{user: models.User{ID: "test-user", Email: "accountant@example.test", FirstName: "Accountant", PasswordHash: hash, Roles: []string{"ACCOUNTANT", "COMPLIANCE_AUDITOR"}, IsActive: true}}
	verifier := &testCaptcha{}
	auth, err := services.NewAuthService(store, verifier)
	if err != nil {
		t.Fatal(err)
	}
	router := server.SetupRouter(nil, controllers.NewAuthController(auth, nil))
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(w, r)
		return w
	}
	body := `{"email":"accountant@example.test","password":"Sample#h01","captcha_id":"` + strings.Repeat("a", 64) + `","captcha_answer":"12"}`
	if w := request("POST", "/api/v1/auth/login", strings.TrimSuffix(body, "}")+`,"roles":["ADMIN"]}`, ""); w.Code != http.StatusBadRequest {
		t.Fatal("unknown login fields accepted")
	}
	if w := request("POST", "/api/v1/auth/login", `{"email":"accountant@example.test","password":"anything"}`, ""); w.Code != http.StatusBadRequest {
		t.Fatal("login without CAPTCHA accepted")
	}
	verifier.valid = true
	if w := request("POST", "/api/v1/auth/login", strings.ReplaceAll(body, "Sample#h01", "incorrect-password"), ""); w.Code != http.StatusUnauthorized {
		t.Fatal("wrong password accepted")
	}
	verifier.valid = true
	w := request("POST", "/api/v1/auth/login", body, "")
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("valid login rejected or cacheable")
	}
	var response models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if (len(response.User.Roles) != 2 || response.User.Roles[0] != "ACCOUNTANT" || response.User.Roles[1] != "COMPLIANCE_AUDITOR") || strings.Contains(w.Body.String(), hash) {
		t.Fatal("role escalation or password hash exposure")
	}
	if response.User.FirstName != "Accountant" || response.User.LastName != nil || strings.Contains(w.Body.String(), `"name":`) {
		t.Fatal("user response must use first_name and nullable last_name, without name")
	}
	digest := sha256.Sum256([]byte(response.Token))
	if string(store.hash) != string(digest[:]) {
		t.Fatal("raw session token stored")
	}
	if w := request("GET", "/api/v1/auth/me", "", response.Token); w.Code != http.StatusOK {
		t.Fatal("valid session rejected")
	}
	if w := request("POST", "/api/v1/auth/logout", "", response.Token); w.Code != http.StatusNoContent {
		t.Fatal("logout failed")
	}
	if w := request("GET", "/api/v1/auth/me", "", response.Token); w.Code != http.StatusUnauthorized {
		t.Fatal("revoked token accepted")
	}
	if w := request("POST", "/api/v1/auth/login", body, ""); w.Code != http.StatusBadRequest {
		t.Fatal("CAPTCHA replay accepted")
	}
}

func TestUpdateMeHTTPContract(t *testing.T) {
	hash, err := services.HashPassword("Sample#h01")
	if err != nil {
		t.Fatal(err)
	}
	store := &testStore{user: models.User{ID: "test-user", Email: "accountant@example.test", FirstName: "Accountant", PasswordHash: hash, Roles: []string{"ACCOUNTANT"}, IsActive: true}}
	auth, err := services.NewAuthService(store, &testCaptcha{valid: true})
	if err != nil {
		t.Fatal(err)
	}
	router := server.SetupRouter(nil, controllers.NewAuthController(auth, nil))
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(w, r)
		return w
	}
	w := request("POST", "/api/v1/auth/login", `{"email":"accountant@example.test","password":"Sample#h01","captcha_id":"`+strings.Repeat("a", 64)+`","captcha_answer":"12"}`, "")
	var login models.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil || login.Token == "" {
		t.Fatal("login failed:", w.Body.String())
	}
	if w := request("PATCH", "/api/v1/auth/me", `{"first_name":"Kenji"}`, ""); w.Code != http.StatusUnauthorized {
		t.Fatal("unauthenticated profile update accepted:", w.Code)
	}
	if w := request("PATCH", "/api/v1/auth/me", `{"first_name":"  "}`, login.Token); w.Code != http.StatusBadRequest {
		t.Fatal("blank first name accepted:", w.Code)
	}
	if w := request("PATCH", "/api/v1/auth/me", `not json`, login.Token); w.Code != http.StatusBadRequest {
		t.Fatal("malformed body accepted:", w.Code)
	}
	w = request("PATCH", "/api/v1/auth/me", `{"first_name":"Kenji","last_name":"Sato","email":"evil@example.test","roles":["ADMIN"]}`, login.Token)
	if w.Code != http.StatusBadRequest {
		t.Fatal("unknown profile fields accepted")
	}
	w = request("PATCH", "/api/v1/auth/me", `{"first_name":"Kenji","last_name":"Sato"}`, login.Token)
	var u models.UserDTO
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &u) != nil {
		t.Fatal("valid profile update rejected:", w.Code, w.Body.String())
	}
	if u.FirstName != "Kenji" || u.LastName == nil || *u.LastName != "Sato" || u.Email != "accountant@example.test" || len(u.Roles) != 1 || u.Roles[0] != "ACCOUNTANT" {
		t.Fatalf("profile update changed the wrong fields: %+v", u)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("profile response is cacheable")
	}
}
