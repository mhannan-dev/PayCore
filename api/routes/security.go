package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
)

// RegisterSecurity registers the security endpoints and their middleware.
func RegisterSecurity(api *gin.RouterGroup, h *controllers.SecurityController, auth *middleware.Auth) {
	g := api.Group("/auth/security", middleware.NoStore, middleware.AuthRateLimit(), middleware.ErrorFallback(apperr.Unavailable("security_unavailable", "Account security is unavailable.")), h.Ready, auth.Authenticate())
	g.GET("", h.Overview)
	g.POST("/recovery-codes", h.Regenerate)
	g.POST("/authenticator", h.BeginReplacement)
	g.POST("/authenticator/confirm", h.ConfirmReplacement)
	g.POST("/sessions/revoke-others", h.RevokeOthers)
	g.POST("/sessions/:id/revoke", h.RevokeSession)
}
