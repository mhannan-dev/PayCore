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

type AccountSecurity interface {
	Overview(context.Context, string, string) (model.SecurityOverview, error)
	Regenerate(context.Context, string, string, model.SecurityProof) (model.RecoveryCodesResult, error)
	BeginReplacement(context.Context, model.UserDTO, string, model.SecurityProof) (model.ReplacementSetup, error)
	ConfirmReplacement(context.Context, string, string, model.SecurityProof) (model.ReplacementResult, error)
	Revoke(context.Context, string, string, string, bool, model.SecurityProof) (model.SecurityActionResult, error)
}
type SecurityHandler struct {
	security AccountSecurity
	guard    *Guard
}

func NewSecurityHandler(security AccountSecurity, guard *Guard) *SecurityHandler {
	return &SecurityHandler{security: security, guard: guard}
}
func (h *SecurityHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/auth/security", noStore, middleware.AuthRateLimit(), middleware.ErrorFallback(apperr.Unavailable("security_unavailable", "Account security is unavailable.")), h.ready, h.guard.Session())
	g.GET("", h.Overview)
	g.POST("/recovery-codes", h.Regenerate)
	g.POST("/authenticator", h.BeginReplacement)
	g.POST("/authenticator/confirm", h.ConfirmReplacement)
	g.POST("/sessions/revoke-others", h.RevokeOthers)
	g.POST("/sessions/:id/revoke", h.RevokeSession)
}
func (h *SecurityHandler) ready(c *gin.Context) {
	if h.security == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}
func securityProof(c *gin.Context) (model.SecurityProof, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var proof model.SecurityProof
	if err := bindJSON(c, &proof); err != nil {
		fail(c, apperr.BadRequest("invalid_security_proof", "Send your current password and a fresh MFA code when enrolled."))
		return proof, false
	}
	return proof, true
}
func (h *SecurityHandler) Overview(c *gin.Context) {
	u := sessionUser(c)
	result, err := h.security.Overview(c.Request.Context(), u.ID, bearer(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
func (h *SecurityHandler) Regenerate(c *gin.Context) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	u := sessionUser(c)
	result, err := h.security.Regenerate(c.Request.Context(), u.ID, bearer(c), proof)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
func (h *SecurityHandler) BeginReplacement(c *gin.Context) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	result, err := h.security.BeginReplacement(c.Request.Context(), sessionUser(c), bearer(c), proof)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
func (h *SecurityHandler) ConfirmReplacement(c *gin.Context) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	u := sessionUser(c)
	result, err := h.security.ConfirmReplacement(c.Request.Context(), u.ID, bearer(c), proof)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
func (h *SecurityHandler) RevokeOthers(c *gin.Context) { h.revoke(c, "", true) }
func (h *SecurityHandler) RevokeSession(c *gin.Context) {
	id := c.Param("id")
	if !validUUID(id) {
		fail(c, apperr.NotFound("session_not_found", "Session not found."))
		return
	}
	h.revoke(c, id, false)
}
func (h *SecurityHandler) revoke(c *gin.Context, id string, others bool) {
	proof, ok := securityProof(c)
	if !ok {
		return
	}
	u := sessionUser(c)
	result, err := h.security.Revoke(c.Request.Context(), u.ID, bearer(c), id, others, proof)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
