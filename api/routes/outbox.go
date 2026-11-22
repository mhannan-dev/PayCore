package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterOutbox registers the outbox metrics and replay endpoints.
func RegisterOutbox(api *gin.RouterGroup, h *controllers.OutboxController, auth *middleware.Auth) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("outbox_unavailable", "The outbox service is not available right now. Try again shortly."))
	g := api.Group("/outbox", fallback, h.Ready)

	g.GET("/metrics", auth.Can(models.PermSystemRead), h.Metrics)
	g.POST("/replay", auth.Can(models.PermSystemWrite), h.Replay)
}
