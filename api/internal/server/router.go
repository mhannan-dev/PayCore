package server

import (
	"context"
	"paycore/api/internal/audit"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/http/responses"
	"paycore/api/routes"

	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

// Controllers holds the endpoint dependencies passed to the API routes.
type Controllers = routes.Controllers

// SetupRouter builds a router with only the auth controller (tests, route discovery).
func SetupRouter(db DatabasePinger, authControllers ...*controllers.AuthController) *gin.Engine {
	var h Controllers
	if len(authControllers) > 0 {
		h.Auth = authControllers[0]
	}
	return New(db, h)
}

// New builds the API router. db may be nil; readiness then answers 503.
func New(db DatabasePinger, h Controllers) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)

	stack := NewMiddlewareStack(
		middleware.RequestID(),
		middleware.AccessLog(),
	)
	// Panics are logged by gin and answered with the standard JSON error envelope.
	stack.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.Abort()
		responses.InternalError(c)
	}))
	stack.Use(corsMiddleware())
	// Audit events record where each request came from.
	stack.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(audit.WithMeta(c.Request.Context(), audit.Meta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}))
		c.Next()
	})
	// Every error passed to c.Error becomes a response here (coding rule 26).
	stack.Use(middleware.Errors(ErrorTable))
	stack.Apply(r)

	// Unknown routes and wrong methods get JSON too, not gin's plain-text defaults.
	r.HandleMethodNotAllowed = true
	r.NoRoute(responses.RouteNotFound)
	r.NoMethod(responses.MethodNotAllowed)

	registerHealthRoutes(r, db)

	if h.Auth == nil {
		h.Auth = controllers.NewAuthController(nil, nil)
	}
	if h.Attachments == nil {
		h.Attachments = controllers.NewAttachmentController(nil, nil)
	}
	if h.Users == nil {
		h.Users = controllers.NewUsersController(nil)
	}
	if h.Invitations == nil {
		h.Invitations = controllers.NewInvitationController(nil)
	}
	if h.PasswordResets == nil {
		h.PasswordResets = controllers.NewPasswordResetController(nil)
	}
	if h.Audit == nil {
		h.Audit = controllers.NewAuditController(nil)
	}
	if h.Ledger == nil {
		h.Ledger = controllers.NewLedgerController(nil)
	}

	v1 := r.Group("/api/v1")
	if h.Security == nil {
		h.Security = controllers.NewSecurityController(nil)
	}
	if h.MFA == nil {
		h.MFA = controllers.NewMFAController(nil)
	}
	if h.Statements == nil {
		h.Statements = controllers.NewStatementController(nil)
	}
	if h.Matching == nil {
		h.Matching = controllers.NewMatchingController(nil)
	}
	if h.AuthMiddleware == nil {
		h.AuthMiddleware = middleware.NewAuth(nil)
	}
	routes.Register(v1, h)
	return r
}
