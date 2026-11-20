package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
)

// Controllers are the route controllers wired by cmd/server (the composition root).
// A nil controller is replaced by one without dependencies, whose routes answer 503,
// so the full route table exists for discovery (`-routes`) and tests.
type Controllers struct {
	Matching       *controllers.MatchingController
	Statements     *controllers.StatementController
	Security       *controllers.SecurityController
	MFA            *controllers.MFAController
	Auth           *controllers.AuthController
	Attachments    *controllers.AttachmentController
	Users          *controllers.UsersController
	Invitations    *controllers.InvitationController
	PasswordResets *controllers.PasswordResetController
	Audit          *controllers.AuditController
	Ledger         *controllers.LedgerController

	// AuthMiddleware provides `auth` / `can:` checks for protected routes.
	AuthMiddleware *middleware.Auth
}

// Register mounts all API resource routes on the supplied version group.
func Register(api *gin.RouterGroup, h Controllers) {
	RegisterSecurity(api, h.Security, h.AuthMiddleware)
	RegisterMFA(api, h.MFA)
	RegisterAuth(api, h.Auth)
	RegisterAttachment(api, h.Attachments, h.AuthMiddleware)
	RegisterUsers(api, h.Users, h.AuthMiddleware)
	RegisterInvitation(api, h.Invitations)
	RegisterPasswordReset(api, h.PasswordResets)
	RegisterAudit(api, h.Audit, h.AuthMiddleware)
	RegisterLedger(api, h.Ledger, h.AuthMiddleware)
	RegisterStatement(api, h.Statements, h.AuthMiddleware)
	RegisterMatching(api, h.Matching, h.AuthMiddleware)
}
