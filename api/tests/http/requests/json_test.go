package requests_test

import (
	"net/http"
	"net/http/httptest"
	"paycore/api/internal/http/requests"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindJSONRejectsAmbiguousPayloads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", `{"email":"user@example.test"}`, true},
		{"unknown field", `{"email":"user@example.test","roles":["ADMIN"]}`, false},
		{"second document", `{"email":"user@example.test"} {}`, false},
		{"trailing garbage", `{"email":"user@example.test"} x`, false},
		{"required field", `{}`, false},
		{"invalid email", `{"email":"invalid"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var request struct {
				Email string `json:"email" binding:"required,email"`
			}
			if err := requests.BindJSON(c, &request); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
