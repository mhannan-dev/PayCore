package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/internal/service"
	"paycore/api/pkg/response"
)

// PasswordResetHandler serves the public "forgot password" endpoints.
type PasswordResetHandler struct{ resets *service.PasswordResetService }

func NewPasswordResetHandler(resets *service.PasswordResetService) *PasswordResetHandler {
	return &PasswordResetHandler{resets: resets}
}

const (
	resetUnavailableMessage = "Password reset is not available right now. Try again shortly."
	resetInvalidMessage     = "This reset link is invalid, has expired or was already used. Request a new one."
	resetRequestedMessage   = "If an account exists for that email, a reset link is on its way. It expires in 30 minutes."
)

// Register adds (all public, rate limited, never cached):
//
//	POST /auth/password-reset/request  {email, captcha_id, captcha_answer} → 202 (same answer for any email)
//	POST /auth/password-reset/inspect  {token} → {email, first_name, expires_at}
//	POST /auth/password-reset/confirm  {token, password} → {email}; signs the user out everywhere
func (h *PasswordResetHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/auth/password-reset", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }, middleware.AuthRateLimit())
	g.POST("/request", h.Request)
	g.POST("/inspect", h.Inspect)
	g.POST("/confirm", h.Confirm)
}

func (h *PasswordResetHandler) Request(c *gin.Context) {
	if h.resets == nil {
		response.ServiceUnavailable(c, "reset_unavailable", resetUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.PasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request", "A valid email, captcha_id and the numeric captcha_answer are required.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	switch err := h.resets.Request(ctx, req); {
	case errors.Is(err, service.ErrCaptcha):
		response.BadRequest(c, "invalid_captcha", "CAPTCHA is invalid, expired or already used. Request a new challenge.")
	case err != nil:
		response.ServiceUnavailable(c, "reset_unavailable", resetUnavailableMessage)
	default:
		response.JSON(c, http.StatusAccepted, gin.H{"message": resetRequestedMessage})
	}
}

func (h *PasswordResetHandler) Inspect(c *gin.Context) {
	if h.resets == nil {
		response.ServiceUnavailable(c, "reset_unavailable", resetUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.InvitationTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.NotFound(c, "reset_invalid", resetInvalidMessage)
		return
	}
	info, err := h.resets.Inspect(c.Request.Context(), req.Token)
	switch {
	case errors.Is(err, service.ErrResetToken):
		response.NotFound(c, "reset_invalid", resetInvalidMessage)
	case err != nil:
		response.ServiceUnavailable(c, "reset_unavailable", resetUnavailableMessage)
	default:
		response.OK(c, info)
	}
}

func (h *PasswordResetHandler) Confirm(c *gin.Context) {
	if h.resets == nil {
		response.ServiceUnavailable(c, "reset_unavailable", resetUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.AcceptInvitationRequest // same shape: {token, password}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request", "Send the reset token and a password.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	email, err := h.resets.Confirm(ctx, req.Token, req.Password)
	switch {
	case errors.Is(err, service.ErrWeakPassword):
		response.BadRequest(c, "invalid_password", "The password must be exactly 10 characters.")
	case errors.Is(err, service.ErrResetToken):
		response.NotFound(c, "reset_invalid", resetInvalidMessage)
	case err != nil:
		response.ServiceUnavailable(c, "reset_unavailable", resetUnavailableMessage)
	default:
		response.OK(c, gin.H{"email": email})
	}
}
