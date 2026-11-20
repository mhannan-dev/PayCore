package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
)

// RegisterInvitation registers the invitation endpoints and their middleware.
func RegisterInvitation(api *gin.RouterGroup, h *controllers.InvitationController) {
	g := api.Group("/auth/invitations", middleware.NoStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("rbac_unavailable", "Access management is not available right now. Try again shortly.")))
	g.POST("/inspect", h.Inspect)
	g.POST("/accept", h.Accept)
}
