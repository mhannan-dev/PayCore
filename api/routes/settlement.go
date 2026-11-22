package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterSettlement registers the settlement endpoints.
func RegisterSettlement(api *gin.RouterGroup, h *controllers.SettlementController, auth *middleware.Auth) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("settlements_unavailable", "The settlement service is not available right now. Try again shortly."))
	g := api.Group("/settlements", fallback, h.Ready)

	g.GET("", auth.Can(models.PermTransfersRead), h.List)
	g.GET("/:id", auth.Can(models.PermTransfersRead), h.Detail)
	g.POST("", auth.Can(models.PermTransfersCreate), h.Create)
	g.POST("/:id/capture", auth.Can(models.PermTransfersReverse), h.Capture)
	g.POST("/:id/release", auth.Can(models.PermTransfersReverse), h.Release)
}
