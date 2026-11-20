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

// AuthService is what the sign-in endpoints need (implemented by service.AuthService).
type AuthService interface {
	SessionVerifier
	Login(ctx context.Context, req model.LoginRequest) (model.LoginResponse, error)
	UpdateProfile(ctx context.Context, token string, req model.UpdateProfileRequest) (model.UserDTO, error)
	Logout(ctx context.Context, token string) error
}

// CaptchaIssuer creates login challenges (implemented by service.CaptchaStore).
type CaptchaIssuer interface {
	Issue() (service.CaptchaChallenge, error)
}

// AuthHandler serves sign-in, the current user and sign-out.
type AuthHandler struct {
	auth    AuthService
	captcha CaptchaIssuer
}

// NewAuthHandler: nil dependencies make the routes answer 503 auth_unavailable.
func NewAuthHandler(auth AuthService, captcha CaptchaIssuer) *AuthHandler {
	return &AuthHandler{auth: auth, captcha: captcha}
}

const authUnavailableMessage = "The authentication service is unavailable. Try again shortly."

// Register adds GET /auth/captcha, POST /auth/login, GET|PATCH /auth/me and POST /auth/logout.
func (h *AuthHandler) Register(api *gin.RouterGroup) {
	auth := api.Group("/auth", noStore, middleware.AuthRateLimit(),
		middleware.ErrorFallback(apperr.Unavailable("auth_unavailable", authUnavailableMessage)))
	auth.GET("/captcha", h.Challenge)
	auth.POST("/login", h.Login)
	auth.GET("/me", h.Me)
	auth.PATCH("/me", h.UpdateMe)
	auth.POST("/logout", h.Logout)
}

func (h *AuthHandler) Challenge(c *gin.Context) {
	if h.captcha == nil {
		fail(c, errNotConfigured)
		return
	}
	challenge, err := h.captcha.Issue()
	if err != nil {
		fail(c, apperr.Unavailable("captcha_unavailable", "Could not create a security check. Try again shortly.").WithCause(err))
		return
	}
	response.OK(c, challenge)
}

func (h *AuthHandler) Login(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req model.LoginRequest
	if err := bindJSON(c, &req); err != nil || len(req.Password) > 72 {
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
		response.OK(c, result.MFA)
		return
	}
	response.OK(c, result)
}

func (h *AuthHandler) Me(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	u, err := h.auth.CurrentUser(ctx, bearer(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, u)
}

// UpdateMe handles PATCH /auth/me: {"first_name": "...", "last_name": "..." | null}.
func (h *AuthHandler) UpdateMe(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.UpdateProfileRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send JSON with first_name and optional last_name."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	u, err := h.auth.UpdateProfile(ctx, bearer(c), req)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, u)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if h.auth == nil {
		fail(c, errNotConfigured)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	if err := h.auth.Logout(ctx, bearer(c)); err != nil {
		fail(c, err)
		return
	}
	response.NoContent(c)
}
