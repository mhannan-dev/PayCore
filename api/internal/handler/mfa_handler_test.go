package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/handler"
	"paycore/api/internal/model"
	"paycore/api/internal/server"
)

type mfaTransportStub struct{ calls int }

func (s *mfaTransportStub) Verify(ctx context.Context, req model.MFAVerifyRequest) (model.MFALoginResult, error) {
	s.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("MFA request lacks deadline")
	}
	return model.MFALoginResult{}, model.ErrMFACode
}

type passwordMFAStage struct{ handler.AuthService }

func (s *passwordMFAStage) Login(context.Context, model.LoginRequest) (model.LoginResponse, error) {
	return model.LoginResponse{MFA: &model.MFAChallengeResponse{State: model.MFAVerificationRequired, ChallengeToken: strings.Repeat("c", 64), ExpiresIn: 300}}, nil
}
func TestPasswordStageReturnsOnlyMFAChallenge(t *testing.T) {
	router := server.New(nil, server.Handlers{Auth: handler.NewAuthHandler(&passwordMFAStage{}, nil)})
	body := `{"email":"admin@example.test","password":"Admin#1234","captcha_id":"` + strings.Repeat("a", 64) + `","captcha_answer":"1"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	var result map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || result["challenge_token"] == nil || result["token"] != nil || result["user"] != nil {
		t.Fatalf("password stage exposed a session: %d %s", w.Code, w.Body)
	}
}

func TestMFATransportValidationAndNoStore(t *testing.T) {
	stub := &mfaTransportStub{}
	router := server.New(nil, server.Handlers{MFA: handler.NewMFAHandler(stub)})
	for _, body := range []string{`{}`, `{"challenge_token":"` + strings.Repeat("a", 64) + `","code":"123456","roles":["ADMIN"]}`, `{"challenge_token":"` + strings.Repeat("a", 64) + `","code":"123456"}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("MFA response was cacheable")
		}
		want := 400
		if strings.HasSuffix(body, `"code":"123456"}`) {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body)
		}
		var payload map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
	}
	if stub.calls != 1 {
		t.Fatal("invalid requests reached MFA service")
	}
}
