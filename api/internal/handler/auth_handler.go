package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/model"
	"paycore/api/internal/service"
	"paycore/api/pkg/response"
)

type AuthHandler struct {
	auth    *service.AuthService
	captcha *service.CaptchaStore
}

func NewAuthHandler(auth *service.AuthService, captcha *service.CaptchaStore) *AuthHandler {
	return &AuthHandler{auth: auth, captcha: captcha}
}
func (h *AuthHandler) Challenge(c *gin.Context) {
	if h.captcha == nil {
		response.ServiceUnavailable(c, "auth_unavailable", authUnavailableMessage)
		return
	}
	challenge, err := h.captcha.Issue()
	if err != nil {
		response.ServiceUnavailable(c, "captcha_unavailable", "Could not create a security check. Try again shortly.")
		return
	}
	response.OK(c, challenge)
}
func (h *AuthHandler) Login(c *gin.Context) {
	if h.auth == nil {
		response.ServiceUnavailable(c, "auth_unavailable", authUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Password) > 72 {
		response.BadRequest(c, "invalid_request", "Email, password, captcha_id and the numeric captcha_answer are required.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	result, err := h.auth.Login(ctx, req)
	if err != nil {
		authError(c, err)
		return
	}
	response.OK(c, result)
}
func bearer(c *gin.Context) string {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
func (h *AuthHandler) Me(c *gin.Context) {
	if h.auth == nil {
		authError(c, service.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	u, err := h.auth.CurrentUser(ctx, bearer(c))
	if err != nil {
		authError(c, err)
		return
	}
	response.OK(c, u)
}

// UpdateMe handles PATCH /auth/me: {"first_name": "...", "last_name": "..." | null}.
func (h *AuthHandler) UpdateMe(c *gin.Context) {
	if h.auth == nil {
		authError(c, service.ErrUnavailable)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request", "Send JSON with first_name and optional last_name.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	u, err := h.auth.UpdateProfile(ctx, bearer(c), req)
	if err != nil {
		authError(c, err)
		return
	}
	response.OK(c, u)
}
func (h *AuthHandler) Logout(c *gin.Context) {
	if h.auth == nil {
		authError(c, service.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.auth.Logout(ctx, bearer(c)); err != nil {
		authError(c, err)
		return
	}
	response.NoContent(c)
}

const authUnavailableMessage = "The authentication service is unavailable. Try again shortly."

func authError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrCredentials), errors.Is(err, service.ErrSession):
		response.Unauthorized(c, "invalid_credentials", "Invalid credentials or session.")
	case errors.Is(err, model.ErrInvalidProfile):
		response.BadRequest(c, "invalid_profile", "First name is required; first and last name must be at most 80 characters.")
	case errors.Is(err, service.ErrCaptcha):
		response.BadRequest(c, "invalid_captcha", "CAPTCHA is invalid, expired or already used. Request a new challenge.")
	default:
		response.ServiceUnavailable(c, "auth_unavailable", authUnavailableMessage)
	}
}
