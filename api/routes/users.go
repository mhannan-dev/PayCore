package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterUsers registers the users endpoints and their middleware.
func RegisterUsers(api *gin.RouterGroup, h *controllers.UsersController, auth *middleware.Auth) {
	read, write := auth.Can(models.PermUsersRead), auth.Can(models.PermUsersWrite)
	fallback := middleware.ErrorFallback(apperr.Unavailable("rbac_unavailable", "Access management is not available right now. Try again shortly."))

	rbac := api.Group("/rbac", fallback)
	rbac.GET("/roles", read, h.Roles)
	rbac.GET("/permissions", read, h.Permissions)

	users := api.Group("/users", fallback)
	users.GET("", read, h.List)
	users.POST("", write, h.Invite)
	users.POST("/:id/invitation", write, h.ResendInvitation)
	users.PUT("/:id/roles", write, h.SetRoles)
	users.POST("/:id/deactivate", write, h.Deactivate)
	users.POST("/:id/activate", write, h.Activate)
	users.DELETE("/:id", write, h.Delete)
}
