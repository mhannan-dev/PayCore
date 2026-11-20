package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
)

// RegisterMFA registers the mfa endpoints and their middleware.
func RegisterMFA(api *gin.RouterGroup, h *controllers.MFAController) {
	g := api.Group("/auth/mfa", middleware.NoStore, middleware.AuthRateLimit(), middleware.ErrorFallback(apperr.Unavailable("mfa_unavailable", "MFA is unavailable.")))
	g.POST("/verify", h.Verify)
}
