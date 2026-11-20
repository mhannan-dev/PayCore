package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/audit"
	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

// SessionVerifier resolves a bearer token to the signed-in user (implemented by
// services.AuthService).
type SessionVerifier interface {
	CurrentUser(ctx context.Context, token string) (models.UserDTO, error)
}

const (
	currentUserKey = "paycore.authenticated_user"
	authTimeout    = 15 * time.Second
)

// Auth is the authentication and authorization middleware, the Gin counterpart of
// Laravel's `auth` and `can:` middleware. Routes use it like:
//
//	g.GET("/users", auth.Can(models.PermUsersRead), users.List)       // ->middleware('can:users:read')
//	g := api.Group("/auth/security", auth.Authenticate())             // ->middleware('auth')
//
// Permissions are read from the database on every request, so role changes apply
// immediately. A nil Auth (or one without a SessionVerifier) fails every request
// with apperr.ErrNotConfigured, which the route group's ErrorFallback answers.
type Auth struct{ sessions SessionVerifier }

func NewAuth(sessions SessionVerifier) *Auth { return &Auth{sessions: sessions} }

// Authenticate requires a valid bearer session (Laravel: `auth`).
func (a *Auth) Authenticate() gin.HandlerFunc { return a.check(nil, false) }

// Can requires a valid session whose roles grant permission (Laravel: `can:`).
func (a *Auth) Can(permission models.PermissionCode) gin.HandlerFunc {
	return a.CanAll(permission)
}

// CanAll requires every listed permission. An empty list fails closed.
func (a *Auth) CanAll(permissions ...models.PermissionCode) gin.HandlerFunc {
	return a.check(permissions, true)
}

func (a *Auth) check(permissions []models.PermissionCode, required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if a == nil || a.sessions == nil {
			abort(c, apperr.ErrNotConfigured)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), authTimeout)
		defer cancel()
		u, err := a.sessions.CurrentUser(ctx, BearerToken(c))
		if err != nil {
			abort(c, err)
			return
		}
		if u.ID == "" {
			abort(c, services.ErrSession)
			return
		}
		allowed := !required || len(permissions) > 0
		for _, permission := range permissions {
			if !permission.Valid() || !models.HasPermission(u.Permissions, string(permission)) {
				allowed = false
			}
		}
		if !allowed {
			abort(c, apperr.Forbidden("permission_denied", "Your roles do not grant the required permission."))
			return
		}
		c.Set(currentUserKey, u)
		// Audit events recorded during this request name the signed-in user as actor.
		c.Request = c.Request.WithContext(audit.WithActor(ctx, audit.Actor{ID: u.ID, Email: u.Email}))
		c.Next()
	}
}

// CurrentUser returns the user authenticated by Auth for this request
// (Laravel: $request->user()). ok is false when the route has no Auth middleware.
func CurrentUser(c *gin.Context) (models.UserDTO, bool) {
	raw, exists := c.Get(currentUserKey)
	u, ok := raw.(models.UserDTO)
	return u, exists && ok
}

// BearerToken returns the token from "Authorization: Bearer <token>", or ""
// (Laravel: $request->bearerToken()).
func BearerToken(c *gin.Context) string {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// abort hands err to the Errors middleware and stops the chain.
func abort(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}
