package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

func RegisterQuantum(api *gin.RouterGroup, h *controllers.QuantumController, auth *middleware.Auth) {
	public := api.Group("/quantum", middleware.NoStore)
	public.GET("/config", h.Config)
	g := public.Group("", middleware.ErrorFallback(apperr.Unavailable("quantum_unavailable", "Quantum testnet payments are unavailable. Check the server configuration.")), h.Ready)
	g.POST("/merchants", auth.Can(models.PermQuantumWrite), h.CreateMerchant)
	g.GET("/merchants/me", auth.Can(models.PermQuantumRead), h.Merchant)
	g.POST("/invoices", auth.Can(models.PermQuantumWrite), h.CreateInvoice)
	g.GET("/invoices", auth.Can(models.PermQuantumRead), h.Invoices)
	g.GET("/invoices/:id", auth.Can(models.PermQuantumRead), h.Invoice)
	g.POST("/invoices/:id/cancel", auth.Can(models.PermQuantumWrite), h.Cancel)
	checkout := g.Group("/checkout", middleware.QuantumRateLimit())
	checkout.GET("/:token", h.Checkout)
	checkout.GET("/:token/status", h.Status)
	checkout.POST("/:token/attempts", h.Attempt)
	checkout.POST("/:token/attempts/:id/tx", h.Submit)
}
