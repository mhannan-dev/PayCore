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

// PasswordResets is what the forgot-password endpoints need (implemented by
// service.PasswordResetService).
type PasswordResets interface {
	Request(ctx context.Context, req model.PasswordResetRequest) error
	Inspect(ctx context.Context, token string) (model.PasswordResetInfo, error)
	Confirm(ctx context.Context, token, password string) (string, error)
}

// PasswordResetHandler serves the public "forgot password" endpoints.
type PasswordResetHandler struct{ resets PasswordResets }

// NewPasswordResetHandler: a nil service makes the routes answer 503 reset_unavailable.
func NewPasswordResetHandler(resets PasswordResets) *PasswordResetHandler {
	return &PasswordResetHandler{resets: resets}
}

// Register adds (all public, rate limited, never cached):
//
//	POST /auth/password-reset/request  {email, captcha_id, captcha_answer} → 202 (same answer for any email)
//	POST /auth/password-reset/inspect  {token} → {email, first_name, expires_at}
//	POST /auth/password-reset/confirm  {token, password} → {email}; signs the user out everywhere
func (h *PasswordResetHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/auth/password-reset", noStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("reset_unavailable", "Password reset is not available right now. Try again shortly.")))
	g.POST("/request", h.Request)
	g.POST("/inspect", h.Inspect)
	g.POST("/confirm", h.Confirm)
}

func (h *PasswordResetHandler) Request(c *gin.Context) {
	if h.resets == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.PasswordResetRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "A valid email, captcha_id and the numeric captcha_answer are required."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	if err := h.resets.Request(ctx, req); err != nil {
		fail(c, err)
		return
	}
	response.JSON(c, http.StatusAccepted, gin.H{"message": "If an account exists for that email, a reset link is on its way. It expires in 30 minutes."})
}

func (h *PasswordResetHandler) Inspect(c *gin.Context) {
	if h.resets == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.InvitationTokenRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, service.ErrResetToken) // a malformed token is just an invalid link
		return
	}
	info, err := h.resets.Inspect(c.Request.Context(), req.Token)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, info)
}

func (h *PasswordResetHandler) Confirm(c *gin.Context) {
	if h.resets == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.AcceptInvitationRequest // same shape: {token, password}
	if err := bindJSON(c, &req); err != nil {
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
	response.OK(c, gin.H{"email": email})
}
