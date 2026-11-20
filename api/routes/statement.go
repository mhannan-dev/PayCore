package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterStatement registers the statement endpoints and their middleware.
func RegisterStatement(api *gin.RouterGroup, h *controllers.StatementController, auth *middleware.Auth) {
	g := api.Group("/reconciliation", middleware.ErrorFallback(apperr.Unavailable("reconciliation_unavailable", "Statement reconciliation is unavailable. Try again shortly.")), h.Ready)
	g.POST("/preview", auth.CanAll(models.PermReconciliationImport, models.PermAttachmentsRead, models.PermLedgerRead), h.Preview)
	g.POST("/imports", auth.CanAll(models.PermReconciliationImport, models.PermAttachmentsRead, models.PermLedgerRead), h.Import)
	g.GET("/imports", auth.CanAll(models.PermReconciliationRead, models.PermLedgerRead), h.List)
	g.GET("/imports/:id", auth.CanAll(models.PermReconciliationRead, models.PermLedgerRead), h.Detail)
}
