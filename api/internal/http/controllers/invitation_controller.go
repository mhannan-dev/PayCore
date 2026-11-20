package controllers

import (
	"context"
	"net/http"
	"paycore/api/internal/http/requests"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

// Invitations is what the public invitation endpoints need (implemented by
// services.InvitationService).
type Invitations interface {
	Inspect(ctx context.Context, token string) (models.InvitationInfo, error)
	Accept(ctx context.Context, token, password string) (string, error)
}

// InvitationController serves the public set-password-from-invitation endpoints.
type InvitationController struct{ invites Invitations }

// NewInvitationController: a nil service makes the routes answer 503 rbac_unavailable.
func NewInvitationController(invites Invitations) *InvitationController {
	return &InvitationController{invites: invites}
}

func (h *InvitationController) Inspect(c *gin.Context) {
	if h.invites == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req models.InvitationTokenRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, services.ErrInvitation) // a malformed token is just an invalid link
		return
	}
	info, err := h.invites.Inspect(c.Request.Context(), req.Token)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, info)
}

func (h *InvitationController) Accept(c *gin.Context) {
	if h.invites == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req models.AcceptInvitationRequest
	if err := requests.BindJSON(c, &req); err != nil {
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
	responses.OK(c, gin.H{"email": email})
}
