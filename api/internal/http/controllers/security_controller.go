package controllers

import (
	"context"
	"net/http"
	"paycore/api/internal/http/requests"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

type AccountSecurity interface {
	Overview(context.Context, string, string) (models.SecurityOverview, error)
	Regenerate(context.Context, string, string, models.SecurityProof) (models.RecoveryCodesResult, error)
	BeginReplacement(context.Context, models.UserDTO, string, models.SecurityProof) (models.ReplacementSetup, error)
	ConfirmReplacement(context.Context, string, string, models.SecurityProof) (models.ReplacementResult, error)
	Revoke(context.Context, string, string, string, bool, models.SecurityProof) (models.SecurityActionResult, error)
}
type SecurityController struct {
	security AccountSecurity
}

func NewSecurityController(security AccountSecurity) *SecurityController {
	return &SecurityController{security: security}
}

func (h *SecurityController) Ready(c *gin.Context) {
	if h.security == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func securityProof(c *gin.Context) (models.SecurityProof, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var proof models.SecurityProof
	if err := requests.BindJSON(c, &proof); err != nil {
		fail(c, apperr.BadRequest("invalid_security_proof", "Send your current password and a fresh MFA code when enrolled."))
		return proof, false
	}
	return proof, true
}
func (h *SecurityController) Overview(c *gin.Context) {
	u := sessionUser(c)
	result, err := h.security.Overview(c.Request.Context(), u.ID, middleware.BearerToken(c))
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *SecurityController) Regenerate(c *gin.Context) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	u := sessionUser(c)
	result, err := h.security.Regenerate(c.Request.Context(), u.ID, middleware.BearerToken(c), proof)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *SecurityController) BeginReplacement(c *gin.Context) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	result, err := h.security.BeginReplacement(c.Request.Context(), sessionUser(c), middleware.BearerToken(c), proof)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *SecurityController) ConfirmReplacement(c *gin.Context) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	u := sessionUser(c)
	result, err := h.security.ConfirmReplacement(c.Request.Context(), u.ID, middleware.BearerToken(c), proof)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}
func (h *SecurityController) RevokeOthers(c *gin.Context) { h.revoke(c, "", true) }
func (h *SecurityController) RevokeSession(c *gin.Context) {
	id := c.Param("id")
	if !validUUID(id) {
		fail(c, apperr.NotFound("session_not_found", "Session not found."))
		return
	}
	h.revoke(c, id, false)
}
func (h *SecurityController) revoke(c *gin.Context, id string, others bool) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	u := sessionUser(c)
	result, err := h.security.Revoke(c.Request.Context(), u.ID, middleware.BearerToken(c), id, others, proof)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, result)
}

// Guard returns the shared authentication and permission middleware provider.
