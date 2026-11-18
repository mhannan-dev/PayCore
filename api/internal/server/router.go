package server

import (
	"context"
	"paycore/api/internal/handler"
	"paycore/api/internal/middleware"
	"paycore/api/pkg/response"

	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

// SetupRouter accepts nil for offline route discovery; readiness then returns 503.
func SetupRouter(db DatabasePinger, authHandlers ...*handler.AuthHandler) *gin.Engine {
	var auth *handler.AuthHandler
	if len(authHandlers) > 0 {
		auth = authHandlers[0]
	}
	return SetupRouterWithAttachments(db, auth, nil)
}

// RouteRegistrar is a handler that adds its own routes under /api/v1.
type RouteRegistrar interface{ Register(api *gin.RouterGroup) }

// SetupRouterWithAttachments wires the API. extra may hold a *handler.RBACHandler and a
// *handler.PasswordResetHandler; any that are omitted (or nil) register routes that answer 503.
func SetupRouterWithAttachments(db DatabasePinger, authHandler *handler.AuthHandler, attachmentHandler *handler.AttachmentHandler, extra ...RouteRegistrar) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)

	// Default Gin middlewares
	r.Use(gin.Logger())
	// Panics are logged by gin and answered with the standard JSON error envelope.
	r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.Abort()
		response.InternalError(c)
	}))
	// Unknown routes and wrong methods get JSON too, not gin's plain-text defaults.
	r.HandleMethodNotAllowed = true
	r.NoRoute(response.RouteNotFound)
	r.NoMethod(response.MethodNotAllowed)

	r.Use(corsMiddleware())

	registerHealthRoutes(r, db)

	// Handlers
	if authHandler == nil {
		authHandler = handler.NewAuthHandler(nil, nil)
	}
	if attachmentHandler == nil {
		attachmentHandler = handler.NewAttachmentHandler(nil, nil, nil)
	}
	var rbacHandler RouteRegistrar = handler.NewRBACHandler(nil, nil, nil)
	var resetHandler RouteRegistrar = handler.NewPasswordResetHandler(nil)
	for _, registrar := range extra {
		switch h := registrar.(type) {
		case *handler.RBACHandler:
			if h != nil {
				rbacHandler = h
			}
		case *handler.PasswordResetHandler:
			if h != nil {
				resetHandler = h
			}
		}
	}
	// API v1 Routes
	v1 := r.Group("/api/v1")
	attachmentHandler.Register(v1)
	rbacHandler.Register(v1)
	resetHandler.Register(v1)
	{
		auth := v1.Group("/auth")
		auth.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }, middleware.AuthRateLimit())
		{
			auth.GET("/captcha", authHandler.Challenge)
			auth.POST("/login", authHandler.Login)
			auth.GET("/me", authHandler.Me)
			auth.PATCH("/me", authHandler.UpdateMe)
			auth.POST("/logout", authHandler.Logout)
		}
	}

	return r
}
