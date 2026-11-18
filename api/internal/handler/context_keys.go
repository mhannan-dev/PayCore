package handler

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

// contextKey is a typed key for values that middleware hands to handlers through
// gin.Context. Reads are checked at compile time; a missing value is reported with
// ok=false instead of a panicking type assertion (see docs/coding_rules.md).
type contextKey[T any] struct{ name string }

func (k contextKey[T]) set(c *gin.Context, v T) { c.Set(k.name, v) }

func (k contextKey[T]) get(c *gin.Context) (T, bool) {
	raw, exists := c.Get(k.name)
	v, ok := raw.(T)
	return v, exists && ok
}

// authenticatedUser is set by the authentication middleware of each route group.
var authenticatedUser = contextKey[model.UserDTO]{name: "paycore.authenticated_user"}

// callerOrAbort returns the authenticated user, or answers 500 when a handler is
// wired without its authentication middleware (a programming error).
func callerOrAbort(c *gin.Context) (model.UserDTO, bool) {
	u, ok := authenticatedUser.get(c)
	if !ok {
		c.Abort()
		response.InternalError(c)
	}
	return u, ok
}
