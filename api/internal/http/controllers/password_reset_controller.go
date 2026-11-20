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

// PasswordResets is what the forgot-password endpoints need (implemented by
// services.PasswordResetService).
type PasswordResets interface {
	Request(ctx context.Context, req models.PasswordResetRequest) error
	Inspect(ctx context.Context, token string) (models.PasswordResetInfo, error)
	Confirm(ctx context.Context, token, password string) (string, error)
}

// PasswordResetController serves the public "forgot password" endpoints.
type PasswordResetController struct{ resets PasswordResets }

// NewPasswordResetController: a nil service makes the routes answer 503 reset_unavailable.
func NewPasswordResetController(resets PasswordResets) *PasswordResetController {
	return &PasswordResetController{resets: resets}
}

func (h *PasswordResetController) Request(c *gin.Context) {
	if h.resets == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req models.PasswordResetRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "A valid email, captcha_id and the numeric captcha_answer are required."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	if err := h.resets.Request(ctx, req); err != nil {
		fail(c, err)
		return
	}
	responses.JSON(c, http.StatusAccepted, gin.H{"message": "If an account exists for that email, a reset link is on its way. It expires in 30 minutes."})
}

func (h *PasswordResetController) Inspect(c *gin.Context) {
	if h.resets == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req models.InvitationTokenRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, services.ErrResetToken) // a malformed token is just an invalid link
		return
	}
	info, err := h.resets.Inspect(c.Request.Context(), req.Token)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, info)
}

func (h *PasswordResetController) Confirm(c *gin.Context) {
	if h.resets == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req models.AcceptInvitationRequest // same shape: {token, password}
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send the reset token and a password."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	email, err := h.resets.Confirm(ctx, req.Token, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, gin.H{"email": email})
}
