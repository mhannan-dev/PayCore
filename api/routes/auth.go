package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
)

// RegisterAuth registers the auth endpoints and their middleware.
func RegisterAuth(api *gin.RouterGroup, h *controllers.AuthController) {
	auth := api.Group("/auth", middleware.NoStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("auth_unavailable", "The authentication service is unavailable. Try again shortly.")))
	auth.GET("/captcha", h.Challenge)
	auth.POST("/login", h.Login)
	auth.GET("/me", h.Me)
	auth.PATCH("/me", h.UpdateMe)
	auth.POST("/logout", h.Logout)
}
