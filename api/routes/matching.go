package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterMatching registers the matching endpoints and their middleware.
func RegisterMatching(api *gin.RouterGroup, h *controllers.MatchingController, auth *middleware.Auth) {
	g := api.Group("/reconciliation/imports/:id", middleware.ErrorFallback(apperr.Unavailable("matching_unavailable", "Statement matching is unavailable.")), h.Ready)
	g.GET("/matches", auth.CanAll(models.PermReconciliationRead, models.PermLedgerRead), h.Report)
	g.POST("/matching", auth.CanAll(models.PermReconciliationMatch, models.PermReconciliationRead, models.PermLedgerRead), h.Configure)
	g.POST("/rows/:row/match", auth.CanAll(models.PermReconciliationMatch, models.PermReconciliationRead, models.PermLedgerRead), h.Review)
}
