package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
)

// RegisterPasswordReset registers the passwordreset endpoints and their middleware.
func RegisterPasswordReset(api *gin.RouterGroup, h *controllers.PasswordResetController) {
	g := api.Group("/auth/password-reset", middleware.NoStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("reset_unavailable", "Password reset is not available right now. Try again shortly.")))
	g.POST("/request", h.Request)
	g.POST("/inspect", h.Inspect)
	g.POST("/confirm", h.Confirm)
}
