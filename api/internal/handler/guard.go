package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/audit"
	"paycore/api/internal/model"
)

// SessionVerifier resolves a bearer token to the signed-in user (implemented by
// service.AuthService).
type SessionVerifier interface {
	CurrentUser(ctx context.Context, token string) (model.UserDTO, error)
}

// errNotConfigured marks a handler built without its dependencies (route
// discovery, tests); the route group's ErrorFallback answers it, e.g. 503.
var errNotConfigured = apperr.ErrNotConfigured

const requestTimeout = 15 * time.Second

// fail hands err to the error middleware (internal/middleware.Errors) and stops
// the chain. Handlers never write error responses themselves.
func fail(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

// bearer returns the token from "Authorization: Bearer <token>", or "".
func bearer(c *gin.Context) string {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// Guard authenticates requests and checks permissions. One Guard is shared by
// every handler that needs a signed-in user.
type Guard struct{ sessions SessionVerifier }

func NewGuard(sessions SessionVerifier) *Guard { return &Guard{sessions: sessions} }

// Session requires a valid bearer session. It bounds the request with a
// timeout, stores the user for callerOrAbort, and names them as the audit actor.
func (g *Guard) Session() gin.HandlerFunc { return g.check("") }

// Require requires a valid session whose roles grant permission. Permissions
// are read on every request, so role changes apply immediately.
func (g *Guard) Require(permission string) gin.HandlerFunc { return g.check(permission) }

func (g *Guard) check(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if g == nil || g.sessions == nil {
			fail(c, errNotConfigured)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
		defer cancel()
		u, err := g.sessions.CurrentUser(ctx, bearer(c))
		if err != nil {
			fail(c, err)
			return
		}
		if permission != "" && !model.HasPermission(u.Permissions, permission) {
			fail(c, apperr.Forbidden("permission_denied", "Your roles do not grant the "+permission+" permission."))
			return
		}
		authenticatedUser.set(c, u)
		c.Request = c.Request.WithContext(audit.WithActor(ctx, audit.Actor{ID: u.ID, Email: u.Email}))
		c.Next()
	}
}

// noStore marks responses as private and uncacheable (auth and token endpoints).
func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
}
