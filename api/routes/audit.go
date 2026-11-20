package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterAudit registers the audit endpoints and their middleware.
func RegisterAudit(api *gin.RouterGroup, h *controllers.AuditController, auth *middleware.Auth) {
	g := api.Group("/audit", middleware.ErrorFallback(apperr.Unavailable("audit_unavailable", "The audit log is not available right now.")))
	g.GET("/events", auth.Can(models.PermAuditRead), h.Events)
}
