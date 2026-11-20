package server

import (
	"context"
	"paycore/api/internal/audit"
	"paycore/api/internal/handler"
	"paycore/api/internal/middleware"
	"paycore/api/pkg/response"

	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

// Handlers are the route handlers wired by cmd/server (the composition root).
// A nil handler is replaced by one without dependencies, whose routes answer 503,
// so the full route table exists for discovery (`-routes`) and tests.
type Handlers struct {
	Matching       *handler.MatchingHandler
	Statements     *handler.StatementHandler
	Security       *handler.SecurityHandler
	MFA            *handler.MFAHandler
	Auth           *handler.AuthHandler
	Attachments    *handler.AttachmentHandler
	Users          *handler.UsersHandler
	Invitations    *handler.InvitationHandler
	PasswordResets *handler.PasswordResetHandler
	Audit          *handler.AuditHandler
	Ledger         *handler.LedgerHandler
}

// SetupRouter builds a router with only the auth handler (tests, route discovery).
func SetupRouter(db DatabasePinger, authHandlers ...*handler.AuthHandler) *gin.Engine {
	var h Handlers
	if len(authHandlers) > 0 {
		h.Auth = authHandlers[0]
	}
	return New(db, h)
}

// New builds the API router. db may be nil; readiness then answers 503.
func New(db DatabasePinger, h Handlers) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)

	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog())
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
	// Audit events record where each request came from.
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(audit.WithMeta(c.Request.Context(), audit.Meta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}))
		c.Next()
	})
	// Every error passed to c.Error becomes a response here (coding rule 26).
	r.Use(middleware.Errors(ErrorTable))

	registerHealthRoutes(r, db)

	if h.Auth == nil {
		h.Auth = handler.NewAuthHandler(nil, nil)
	}
	if h.Attachments == nil {
		h.Attachments = handler.NewAttachmentHandler(nil, nil, nil)
	}
	if h.Users == nil {
		h.Users = handler.NewUsersHandler(nil, nil)
	}
	if h.Invitations == nil {
		h.Invitations = handler.NewInvitationHandler(nil)
	}
	if h.PasswordResets == nil {
		h.PasswordResets = handler.NewPasswordResetHandler(nil)
	}
	if h.Audit == nil {
		h.Audit = handler.NewAuditHandler(nil, nil)
	}
	if h.Ledger == nil {
		h.Ledger = handler.NewLedgerHandler(nil, nil)
	}

	v1 := r.Group("/api/v1")
	if h.Security == nil {
		h.Security = handler.NewSecurityHandler(nil, nil)
	}
	h.Security.Register(v1)
	if h.MFA == nil {
		h.MFA = handler.NewMFAHandler(nil)
	}
	h.MFA.Register(v1)
	h.Auth.Register(v1)
	h.Attachments.Register(v1)
	h.Users.Register(v1)
	h.Invitations.Register(v1)
	h.PasswordResets.Register(v1)
	h.Audit.Register(v1)
	h.Ledger.Register(v1)
	if h.Statements == nil {
		h.Statements = handler.NewStatementHandler(nil, nil)
	}
	h.Statements.Register(v1)
	if h.Matching == nil {
		h.Matching = handler.NewMatchingHandler(nil, nil)
	}
	h.Matching.Register(v1)
	return r
}
