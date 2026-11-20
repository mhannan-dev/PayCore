package controllers

import (
	"context"
	"net/http"
	"paycore/api/internal/http/requests"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

type MFALogins interface {
	Verify(context.Context, models.MFAVerifyRequest) (models.MFALoginResult, error)
}
type MFAController struct{ mfa MFALogins }

func NewMFAController(mfa MFALogins) *MFAController { return &MFAController{mfa: mfa} }

func (h *MFAController) Verify(c *gin.Context) {
	if h.mfa == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req models.MFAVerifyRequest
	if err := requests.BindJSON(c, &req); err != nil {
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
	responses.OK(c, result)
}
