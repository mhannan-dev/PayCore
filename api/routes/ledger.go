package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterLedger registers the ledger endpoints and their middleware.
func RegisterLedger(api *gin.RouterGroup, h *controllers.LedgerController, auth *middleware.Auth) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("ledger_unavailable", "The ledger is not available right now. Try again shortly."))
	g := api.Group("", fallback, h.Ready)

	g.GET("/accounts", auth.Can(models.PermLedgerRead), h.Accounts)
	g.POST("/accounts", auth.Can(models.PermAccountsWrite), h.CreateAccount)
	g.GET("/accounts/:id", auth.Can(models.PermLedgerRead), h.Account)
	g.GET("/accounts/:id/postings", auth.Can(models.PermLedgerRead), h.AccountPostings)

	g.GET("/transfers", auth.Can(models.PermTransfersRead), h.Transfers)
	g.GET("/transfers/:id", auth.Can(models.PermTransfersRead), h.Transfer)
	g.POST("/transfers", auth.Can(models.PermTransfersCreate), h.CreateTransfer)
	g.POST("/transfers/:id/reverse", auth.Can(models.PermTransfersReverse), h.Reverse)

	g.GET("/audit/zero-sum", auth.Can(models.PermAuditVerify), h.ZeroSum)
}
