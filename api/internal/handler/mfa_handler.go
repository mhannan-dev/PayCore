package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

type MFALogins interface {
	Verify(context.Context, model.MFAVerifyRequest) (model.MFALoginResult, error)
}
type MFAHandler struct{ mfa MFALogins }

func NewMFAHandler(mfa MFALogins) *MFAHandler { return &MFAHandler{mfa: mfa} }
func (h *MFAHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/auth/mfa", noStore, middleware.AuthRateLimit(), middleware.ErrorFallback(apperr.Unavailable("mfa_unavailable", "MFA is unavailable.")))
	g.POST("/verify", h.Verify)
}
func (h *MFAHandler) Verify(c *gin.Context) {
	if h.mfa == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.MFAVerifyRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send the MFA challenge and authenticator or recovery code."))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()
	result, err := h.mfa.Verify(ctx, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
