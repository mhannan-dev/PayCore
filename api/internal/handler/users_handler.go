package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

// UserAdmin is what user and role administration needs (implemented by
// service.UserAdminService).
type UserAdmin interface {
	Roles(ctx context.Context) ([]model.RoleDetail, error)
	Permissions(ctx context.Context) ([]model.Permission, error)
	Users(ctx context.Context, q model.UserQuery) ([]model.ManagedUser, int, error)
	Invite(ctx context.Context, actor model.UserDTO, req model.CreateUserRequest) (model.ManagedUser, string, error)
	ResendInvitation(ctx context.Context, actor model.UserDTO, userID string) (model.ManagedUser, string, error)
	SetRoles(ctx context.Context, actor model.UserDTO, userID string, roles []string) (model.ManagedUser, error)
	Delete(ctx context.Context, actor model.UserDTO, userID string) error
	SetActive(ctx context.Context, actor model.UserDTO, userID string, active bool) (model.ManagedUser, error)
}

// UsersHandler serves user, role and permission administration.
type UsersHandler struct {
	users UserAdmin
	guard *Guard
}

// NewUsersHandler: nil dependencies make the routes answer 503 rbac_unavailable.
func NewUsersHandler(users UserAdmin, guard *Guard) *UsersHandler {
	return &UsersHandler{users: users, guard: guard}
}

// Register adds:
//
//	GET    /rbac/roles               users:read
//	GET    /rbac/permissions         users:read
//	GET    /users                    users:read   ?q=&role=&limit=20&offset=0 → {items, limit, offset, total}
//	POST   /users                    users:write  invite by email
//	POST   /users/:id/invitation     users:write  re-send the invitation
//	PUT    /users/:id/roles          users:write
//	POST   /users/:id/deactivate     users:write
//	POST   /users/:id/activate       users:write
//	DELETE /users/:id                users:write
func (h *UsersHandler) Register(api *gin.RouterGroup) {
	read, write := h.guard.Require(model.PermUsersRead), h.guard.Require(model.PermUsersWrite)
	fallback := middleware.ErrorFallback(apperr.Unavailable("rbac_unavailable", "Access management is not available right now. Try again shortly."))

	rbac := api.Group("/rbac", fallback)
	rbac.GET("/roles", read, h.Roles)
	rbac.GET("/permissions", read, h.Permissions)

	users := api.Group("/users", fallback)
	users.GET("", read, h.List)
	users.POST("", write, h.Invite)
	users.POST("/:id/invitation", write, h.withUser(h.ResendInvitation))
	users.PUT("/:id/roles", write, h.withUser(h.SetRoles))
	users.POST("/:id/deactivate", write, h.withUser(h.setActive(false)))
	users.POST("/:id/activate", write, h.withUser(h.setActive(true)))
	users.DELETE("/:id", write, h.withUser(h.Delete))
}

var errUserID = apperr.NotFound("user_not_found", "No user has that ID.")

func validUUID(id string) bool {
	var value pgtype.UUID
	return value.Scan(id) == nil && value.Valid
}

// userAction is an endpoint acting on the user in :id, called by the signed-in actor.
type userAction func(c *gin.Context, actor model.UserDTO, userID string)

// withUser validates :id and the actor, then runs action.
func (h *UsersHandler) withUser(action userAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.users == nil {
			fail(c, errNotConfigured)
			return
		}
		id := c.Param("id")
		if !validUUID(id) {
			fail(c, errUserID)
			return
		}
		actor, ok := callerOrAbort(c)
		if !ok {
			return
		}
		action(c, actor, id)
	}
}

func (h *UsersHandler) Roles(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	roles, err := h.users.Roles(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": roles})
}

func (h *UsersHandler) Permissions(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	permissions, err := h.users.Permissions(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": permissions})
}

func (h *UsersHandler) List(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		fail(c, apperr.BadRequest("invalid_limit", "limit must be a whole number from 1 to 100."))
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 100000 {
		fail(c, apperr.BadRequest("invalid_offset", "offset must be a whole number from 0 to 100000."))
		return
	}
	search := strings.TrimSpace(c.Query("q"))
	if utf8.RuneCountInString(search) > 100 {
		fail(c, apperr.BadRequest("invalid_query", "q must be at most 100 characters."))
		return
	}
	role := strings.ToUpper(strings.TrimSpace(c.Query("role")))
	if role != "" {
		if _, err := model.NormalizeRoles([]string{role}); err != nil {
			fail(c, apperr.BadRequest("invalid_role", "role must be one of ADMIN, OPERATOR, LEAD_OPERATOR, COMPLIANCE_AUDITOR and STAFF_ENGINEER."))
			return
		}
	}
	users, total, err := h.users.Users(c.Request.Context(), model.UserQuery{Search: search, Role: role, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	response.Page(c, users, total, limit, offset)
}

// Invite creates the user without a password and emails a set-password link.
// email_status is "pending" (sending in the background) or "not_configured".
func (h *UsersHandler) Invite(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req model.CreateUserRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send first_name, email and roles as JSON."))
		return
	}
	user, status, err := h.users.Invite(c.Request.Context(), actor, req)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, gin.H{"user": user, "email_status": status})
}

// ResendInvitation issues a fresh link (the previous one stops working).
func (h *UsersHandler) ResendInvitation(c *gin.Context, actor model.UserDTO, id string) {
	user, status, err := h.users.ResendInvitation(c.Request.Context(), actor, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, gin.H{"user": user, "email_status": status})
}

func (h *UsersHandler) SetRoles(c *gin.Context, actor model.UserDTO, id string) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.UpdateUserRolesRequest
	if err := bindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send {\"roles\": [...]} with at least one role."))
		return
	}
	user, err := h.users.SetRoles(c.Request.Context(), actor, id, req.Roles)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, user)
}

func (h *UsersHandler) setActive(active bool) userAction {
	return func(c *gin.Context, actor model.UserDTO, id string) {
		user, err := h.users.SetActive(c.Request.Context(), actor, id, active)
		if err != nil {
			fail(c, err)
			return
		}
		response.OK(c, user)
	}
}

func (h *UsersHandler) Delete(c *gin.Context, actor model.UserDTO, id string) {
	if err := h.users.Delete(c.Request.Context(), actor, id); err != nil {
		fail(c, err)
		return
	}
	response.NoContent(c)
}
