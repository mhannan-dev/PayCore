package controllers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

// Shared controller helpers (the counterpart of Laravel's base Controller).
// Authentication and permission checks are route middleware: see
// internal/http/middleware.Auth (`auth` / `can:`).

// SessionVerifier resolves a bearer token to the signed-in user (implemented by
// services.AuthService).
type SessionVerifier interface {
	CurrentUser(ctx context.Context, token string) (models.UserDTO, error)
}

// errNotConfigured marks a controller built without its dependencies (route
// discovery, tests); the route group's ErrorFallback answers it, e.g. 503.
var errNotConfigured = apperr.ErrNotConfigured

const requestTimeout = 15 * time.Second

// fail hands err to the error middleware (internal/http/middleware.Errors) and stops
// the chain. Controllers never write error responses themselves.
func fail(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

// sessionUser is the user authenticated by the route's Auth middleware. Without it
// (a wiring bug) it returns the zero user, whose empty ID matches nothing: fail closed.
func sessionUser(c *gin.Context) models.UserDTO {
	u, _ := middleware.CurrentUser(c)
	return u
}

// callerOrAbort returns the authenticated user, or answers 500 when a controller is
// mounted without its Auth middleware (a programming error).
func callerOrAbort(c *gin.Context) (models.UserDTO, bool) {
	u, ok := middleware.CurrentUser(c)
	if !ok {
		c.Abort()
		responses.InternalError(c)
	}
	return u, ok
}
