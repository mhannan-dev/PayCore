package routing_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/http/routing"
)

func TestGroupRegistersDeclarativeRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := routing.NewGroup(engine.Group("/api/v1"), "/example")
	group.Register(routing.Route{
		Method: http.MethodGet,
		Path:   "/items",
		Handler: func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/example/items", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("got %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
