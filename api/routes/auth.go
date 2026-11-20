package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/http/routing"
)

// RegisterAuth registers the auth endpoints and their middleware.
func RegisterAuth(api *gin.RouterGroup, h *controllers.AuthController) {
	auth := routing.NewGroup(api, "/auth", middleware.NoStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("auth_unavailable", "The authentication service is unavailable. Try again shortly.")))
	auth.Register(
		routing.Route{Method: http.MethodGet, Path: "/captcha", Handler: h.Challenge},
		routing.Route{Method: http.MethodPost, Path: "/login", Handler: h.Login},
		routing.Route{Method: http.MethodGet, Path: "/me", Handler: h.Me},
		routing.Route{Method: http.MethodPatch, Path: "/me", Handler: h.UpdateMe},
		routing.Route{Method: http.MethodPost, Path: "/logout", Handler: h.Logout},
	)
}
