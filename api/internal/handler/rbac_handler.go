package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/internal/service"
	"paycore/api/pkg/response"
)

// RBACStore is the data access the RBAC endpoints need (implemented by repository.RBACRepository).
type RBACStore interface {
	ListRoleDetails(context.Context) ([]model.RoleDetail, error)
	ListPermissions(context.Context) ([]model.Permission, error)
	ListUsers(context.Context, model.UserQuery) ([]model.ManagedUser, int, error)
	SetUserRoles(ctx context.Context, userID string, roles []string) (model.ManagedUser, error)
	DeleteUser(ctx context.Context, userID string) error
}

// RBACHandler serves role, permission and user-access administration.
type RBACHandler struct {
	auth    *service.AuthService
	repo    RBACStore
	invites *service.InvitationService
}

func NewRBACHandler(auth *service.AuthService, repo RBACStore, invites *service.InvitationService) *RBACHandler {
	return &RBACHandler{auth: auth, repo: repo, invites: invites}
}

const rbacUnavailableMessage = "Access management is not available right now. Try again shortly."

// Register adds:
//
//	GET /rbac/roles            users:read
//	GET /rbac/permissions      users:read
//	GET /users                 users:read   (?q=&role=&limit=20&offset=0 → {items, limit, offset, total})
//	POST /users                users:write  (invites by email)
//	POST /users/:id/invitation users:write  (re-sends the invitation)
//	PUT /users/:id/roles       users:write
//	DELETE /users/:id          users:write
//	POST /auth/invitations/inspect  public, rate limited
//	POST /auth/invitations/accept   public, rate limited
func (h *RBACHandler) Register(api *gin.RouterGroup) {
	rbac := api.Group("/rbac")
	rbac.GET("/roles", h.require(model.PermUsersRead), h.Roles)
	rbac.GET("/permissions", h.require(model.PermUsersRead), h.Permissions)
	users := api.Group("/users")
	users.GET("", h.require(model.PermUsersRead), h.Users)
	users.POST("", h.require(model.PermUsersWrite), h.CreateUser)
	users.POST("/:id/invitation", h.require(model.PermUsersWrite), h.ResendInvitation)
	invitations := api.Group("/auth/invitations", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() }, middleware.AuthRateLimit())
	invitations.POST("/inspect", h.InspectInvitation)
	invitations.POST("/accept", h.AcceptInvitation)
	users.PUT("/:id/roles", h.require(model.PermUsersWrite), h.SetRoles)
	users.DELETE("/:id", h.require(model.PermUsersWrite), h.DeleteUser)
}

// require authenticates the bearer session and checks that its roles grant permission.
// Permissions are read from the database on every request, so role changes apply immediately.
func (h *RBACHandler) require(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if h.auth == nil || h.repo == nil {
			response.Abort(c, http.StatusServiceUnavailable, "rbac_unavailable", rbacUnavailableMessage)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		u, err := h.auth.CurrentUser(ctx, bearer(c))
		if err != nil {
			authError(c, err)
			c.Abort()
			return
		}
		if !model.HasPermission(u.Permissions, permission) {
			response.Abort(c, http.StatusForbidden, "permission_denied", "Your roles do not grant the "+permission+" permission.")
			return
		}
		authenticatedUser.set(c, u)
		c.Next()
	}
}

func (h *RBACHandler) Roles(c *gin.Context) {
	roles, err := h.repo.ListRoleDetails(c.Request.Context())
	if err != nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	response.OK(c, gin.H{"items": roles})
}

func (h *RBACHandler) Permissions(c *gin.Context) {
	permissions, err := h.repo.ListPermissions(c.Request.Context())
	if err != nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	if permissions == nil {
		permissions = []model.Permission{}
	}
	response.OK(c, gin.H{"items": permissions})
}

func (h *RBACHandler) Users(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		response.BadRequest(c, "invalid_limit", "limit must be a whole number from 1 to 100.")
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 100000 {
		response.BadRequest(c, "invalid_offset", "offset must be a whole number from 0 to 100000.")
		return
	}
	search := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(search) > 100 {
		response.BadRequest(c, "invalid_query", "q must be at most 100 characters.")
		return
	}
	role := strings.ToUpper(strings.TrimSpace(c.Query("role")))
	if role != "" {
		if _, err := model.NormalizeRoles([]string{role}); err != nil {
			response.BadRequest(c, "invalid_role", "role must be one of ADMIN, OPERATOR, LEAD_OPERATOR, COMPLIANCE_AUDITOR and STAFF_ENGINEER.")
			return
		}
	}
	users, total, err := h.repo.ListUsers(c.Request.Context(), model.UserQuery{Search: search, Role: role, Limit: limit, Offset: offset})
	if err != nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	response.Page(c, users, total, limit, offset)
}

func (h *RBACHandler) SetRoles(c *gin.Context) {
	id := c.Param("id")
	if !validAttachmentID(id) {
		response.NotFound(c, "user_not_found", "No user has that ID.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.UpdateUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request", "Send {\"roles\": [...]} with at least one role.")
		return
	}
	roles, err := model.NormalizeRoles(req.Roles)
	if err != nil {
		response.BadRequest(c, "invalid_roles", "Choose at least one role from ADMIN, OPERATOR, LEAD_OPERATOR, COMPLIANCE_AUDITOR and STAFF_ENGINEER.")
		return
	}
	caller, ok := callerOrAbort(c)
	if !ok {
		return
	}
	if caller.ID == id && !hasRole(roles, model.RoleAdmin) && hasRole(caller.Roles, model.RoleAdmin) {
		response.Conflict(c, "cannot_remove_own_admin", "You cannot remove your own Admin role. Ask another administrator.")
		return
	}
	user, err := h.repo.SetUserRoles(c.Request.Context(), id, roles)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(c, "user_not_found", "No user has that ID.")
	case errors.Is(err, repository.ErrLastAdmin):
		response.Conflict(c, "last_admin", "At least one active user must keep the Admin role.")
	case err != nil:
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
	default:
		response.OK(c, user)
	}
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// CreateUser invites a user: the account is created without a password and the user
// receives an email link to set one. email_status is "pending" (sending in the
// background; the outcome appears on the user as invite_email_status) or "not_configured".
func (h *RBACHandler) CreateUser(c *gin.Context) {
	if h.invites == nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request", "Send first_name, email and roles as JSON.")
		return
	}
	if err := req.Normalize(); err != nil {
		response.BadRequest(c, "invalid_user", err.Error())
		return
	}
	caller, ok := callerOrAbort(c)
	if !ok {
		return
	}
	user, status, err := h.invites.Invite(c.Request.Context(), req, caller.ID)
	switch {
	case errors.Is(err, repository.ErrEmailTaken):
		response.Conflict(c, "email_taken", "A user with this email already exists.")
	case err != nil:
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
	default:
		response.Created(c, gin.H{"user": user, "email_status": status})
	}
}

// ResendInvitation issues a fresh link (the previous one stops working) and emails it.
func (h *RBACHandler) ResendInvitation(c *gin.Context) {
	id := c.Param("id")
	if h.invites == nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	if !validAttachmentID(id) {
		response.NotFound(c, "user_not_found", "No user has that ID.")
		return
	}
	caller, ok := callerOrAbort(c)
	if !ok {
		return
	}
	user, status, err := h.invites.Resend(c.Request.Context(), id, caller.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(c, "user_not_found", "No user has that ID.")
	case errors.Is(err, repository.ErrAlreadyActivated):
		response.Conflict(c, "already_activated", "This user has already set a password.")
	case err != nil:
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
	default:
		response.OK(c, gin.H{"user": user, "email_status": status})
	}
}

const invitationInvalidMessage = "This link is invalid, has expired or was already used. Ask your administrator to send a new invitation."

// InspectInvitation tells the set-password page whose invitation a token is.
func (h *RBACHandler) InspectInvitation(c *gin.Context) {
	if h.invites == nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.InvitationTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.NotFound(c, "invitation_invalid", invitationInvalidMessage)
		return
	}
	info, err := h.invites.Inspect(c.Request.Context(), req.Token)
	switch {
	case errors.Is(err, service.ErrInvitation):
		response.NotFound(c, "invitation_invalid", invitationInvalidMessage)
	case err != nil:
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
	default:
		response.OK(c, info)
	}
}

// AcceptInvitation sets the invited user's password; the link then stops working.
func (h *RBACHandler) AcceptInvitation(c *gin.Context) {
	if h.invites == nil {
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.AcceptInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request", "Send the invitation token and a password.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	email, err := h.invites.Accept(ctx, req.Token, req.Password)
	switch {
	case errors.Is(err, service.ErrWeakPassword):
		response.BadRequest(c, "invalid_password", "The password must be exactly 10 characters.")
	case errors.Is(err, service.ErrInvitation):
		response.NotFound(c, "invitation_invalid", invitationInvalidMessage)
	case err != nil:
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
	default:
		response.OK(c, gin.H{"email": email})
	}
}

// DeleteUser permanently removes another user's account.
func (h *RBACHandler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	if !validAttachmentID(id) {
		response.NotFound(c, "user_not_found", "No user has that ID.")
		return
	}
	caller, ok := callerOrAbort(c)
	if !ok {
		return
	}
	if caller.ID == id {
		response.Conflict(c, "cannot_delete_self", "You cannot delete your own account. Ask another administrator.")
		return
	}
	err := h.repo.DeleteUser(c.Request.Context(), id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		response.NotFound(c, "user_not_found", "No user has that ID.")
	case errors.Is(err, repository.ErrLastAdmin):
		response.Conflict(c, "last_admin", "At least one active user must keep the Admin role.")
	case err != nil:
		response.ServiceUnavailable(c, "rbac_unavailable", rbacUnavailableMessage)
	default:
		response.NoContent(c)
	}
}
