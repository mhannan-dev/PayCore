package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

func RegisterNotification(api *gin.RouterGroup, h *controllers.NotificationController, auth *middleware.Auth) {
	g := api.Group("/notifications", middleware.ErrorFallback(apperr.Unavailable("notifications_unavailable", "The notification service is not available right now.")), h.Ready)
	g.GET("", auth.Can(models.PermNotificationsRead), h.List)
	g.GET("/metrics", auth.Can(models.PermNotificationsRead), h.Metrics)
	g.GET("/:id", auth.Can(models.PermNotificationsRead), h.Detail)
	g.POST("/:id/retry", auth.Can(models.PermNotificationsRetry), h.Retry)
}
