package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/internal/service"
	"paycore/api/pkg/response"
)

// Invitations is what the public invitation endpoints need (implemented by
// service.InvitationService).
type Invitations interface {
	Inspect(ctx context.Context, token string) (model.InvitationInfo, error)
	Accept(ctx context.Context, token, password string) (string, error)
}

// InvitationHandler serves the public set-password-from-invitation endpoints.
type InvitationHandler struct{ invites Invitations }

// NewInvitationHandler: a nil service makes the routes answer 503 rbac_unavailable.
func NewInvitationHandler(invites Invitations) *InvitationHandler {
	return &InvitationHandler{invites: invites}
}

// Register adds (public, rate limited, never cached):
//
//	POST /auth/invitations/inspect  {token} → {email, first_name, expires_at}
//	POST /auth/invitations/accept   {token, password} → {email}
func (h *InvitationHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/auth/invitations", noStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("rbac_unavailable", "Access management is not available right now. Try again shortly.")))
	g.POST("/inspect", h.Inspect)
	g.POST("/accept", h.Accept)
}

func (h *InvitationHandler) Inspect(c *gin.Context) {
	if h.invites == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.InvitationTokenRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, service.ErrInvitation) // a malformed token is just an invalid link
		return
	}
	info, err := h.invites.Inspect(c.Request.Context(), req.Token)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, info)
}

func (h *InvitationHandler) Accept(c *gin.Context) {
	if h.invites == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.AcceptInvitationRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send the invitation token and a password."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	email, err := h.invites.Accept(ctx, req.Token, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"email": email})
}
