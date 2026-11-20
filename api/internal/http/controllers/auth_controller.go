package controllers

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/http/requests"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
	"paycore/api/internal/services"
)

// AuthService is what the sign-in endpoints need (implemented by services.AuthService).
type AuthService interface {
	SessionVerifier
	Login(ctx context.Context, req models.LoginRequest) (models.LoginResponse, error)
	UpdateProfile(ctx context.Context, token string, req models.UpdateProfileRequest) (models.UserDTO, error)
	Logout(ctx context.Context, token string) error
}

// CaptchaIssuer creates login challenges (implemented by services.CaptchaStore).
type CaptchaIssuer interface {
	Issue() (services.CaptchaChallenge, error)
}

// AuthController serves sign-in, the current user and sign-out.
type AuthController struct {
	auth    AuthService
	captcha CaptchaIssuer
}

// NewAuthController: nil dependencies make the routes answer 503 auth_unavailable.
func NewAuthController(auth AuthService, captcha CaptchaIssuer) *AuthController {
	return &AuthController{auth: auth, captcha: captcha}
}

func (h *AuthController) Challenge(c *gin.Context) {
	if h.captcha == nil {
		fail(c, errNotConfigured)
		return
	}
	challenge, err := h.captcha.Issue()
	if err != nil {
		fail(c, apperr.Unavailable("captcha_unavailable", "Could not create a security check. Try again shortly.").WithCause(err))
		return
	}
	responses.OK(c, challenge)
}

func (h *AuthController) Login(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req models.LoginRequest
	if err := requests.BindJSON(c, &req); err != nil || len(req.Password) > 72 {
		fail(c, apperr.BadRequest("invalid_request", "Email, password, captcha_id and the numeric captcha_answer are required."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	result, err := h.auth.Login(ctx, req)
	if err != nil {
		fail(c, err)
		return
	}
	if result.MFA != nil {
		responses.OK(c, result.MFA)
		return
	}
	responses.OK(c, result)
}

func (h *AuthController) Me(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	u, err := h.auth.CurrentUser(ctx, middleware.BearerToken(c))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, u)
}

// UpdateMe handles PATCH /auth/me: {"first_name": "...", "last_name": "..." | null}.
func (h *AuthController) UpdateMe(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req models.UpdateProfileRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send JSON with first_name and optional last_name."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	u, err := h.auth.UpdateProfile(ctx, middleware.BearerToken(c), req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, u)
}

func (h *AuthController) Logout(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	if err := h.auth.Logout(ctx, middleware.BearerToken(c)); err != nil {
		fail(c, err)
		return
	}
	responses.NoContent(c)
}
