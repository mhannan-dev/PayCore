package controllers

import (
	"context"
	"net/http"
	"paycore/api/internal/http/requests"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

// UserAdmin is what user and role administration needs (implemented by
// services.UserAdminService).
type UserAdmin interface {
	Roles(ctx context.Context) ([]models.RoleDetail, error)
	Permissions(ctx context.Context) ([]models.Permission, error)
	Users(ctx context.Context, q models.UserQuery) ([]models.ManagedUser, int, error)
	Invite(ctx context.Context, actor models.UserDTO, req models.CreateUserRequest) (models.ManagedUser, string, error)
	ResendInvitation(ctx context.Context, actor models.UserDTO, userID string) (models.ManagedUser, string, error)
	SetRoles(ctx context.Context, actor models.UserDTO, userID string, roles []string) (models.ManagedUser, error)
	Delete(ctx context.Context, actor models.UserDTO, userID string) error
	SetActive(ctx context.Context, actor models.UserDTO, userID string, active bool) (models.ManagedUser, error)
}

// UsersController serves user, role and permission administration.
type UsersController struct {
	users UserAdmin
}

// NewUsersController: nil dependencies make the routes answer 503 rbac_unavailable.
func NewUsersController(users UserAdmin) *UsersController {
	return &UsersController{users: users}
}

var errUserID = apperr.NotFound("user_not_found", "No user has that ID.")

func validUUID(id string) bool {
	var value pgtype.UUID
	return value.Scan(id) == nil && value.Valid
}

// userAction is an endpoint acting on the user in :id, called by the signed-in actor.
type userAction func(c *gin.Context, actor models.UserDTO, userID string)

// withUser validates :id and the actor, then runs action.
func (h *UsersController) withUser(action userAction) gin.HandlerFunc {
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

func (h *UsersController) Roles(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	roles, err := h.users.Roles(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, gin.H{"items": roles})
}

func (h *UsersController) Permissions(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	permissions, err := h.users.Permissions(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, gin.H{"items": permissions})
}

func (h *UsersController) List(c *gin.Context) {
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
		if _, err := models.NormalizeRoles([]string{role}); err != nil {
			fail(c, apperr.BadRequest("invalid_role", "role must be one of ADMIN, OPERATOR, LEAD_OPERATOR, COMPLIANCE_AUDITOR and STAFF_ENGINEER."))
			return
		}
	}
	users, total, err := h.users.Users(c.Request.Context(), models.UserQuery{Search: search, Role: role, Limit: limit, Offset: offset})
	if err != nil {
		fail(c, err)
		return
	}
	responses.Page(c, users, total, limit, offset)
}

// Invite creates the user without a password and emails a set-password link.
// email_status is "pending" (sending in the background) or "not_configured".
func (h *UsersController) Invite(c *gin.Context) {
	if h.users == nil {
		fail(c, errNotConfigured)
		return
	}
	actor, ok := callerOrAbort(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req models.CreateUserRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send first_name, email and roles as JSON."))
		return
	}
	user, status, err := h.users.Invite(c.Request.Context(), actor, req)
	if err != nil {
		fail(c, err)
		return
	}
	responses.Created(c, gin.H{"user": user, "email_status": status})
}

// ResendInvitation issues a fresh link (the previous one stops working).
func (h *UsersController) ResendInvitation(c *gin.Context) {
	h.withUser(h.resendInvitation)(c)
}

func (h *UsersController) resendInvitation(c *gin.Context, actor models.UserDTO, id string) {
	user, status, err := h.users.ResendInvitation(c.Request.Context(), actor, id)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, gin.H{"user": user, "email_status": status})
}

func (h *UsersController) SetRoles(c *gin.Context) {
	h.withUser(h.setRoles)(c)
}

func (h *UsersController) setRoles(c *gin.Context, actor models.UserDTO, id string) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req models.UpdateUserRolesRequest
	if err := requests.BindJSON(c, &req); err != nil {
		fail(c, apperr.BadRequest("invalid_request", "Send {\"roles\": [...]} with at least one role."))
		return
	}
	user, err := h.users.SetRoles(c.Request.Context(), actor, id, req.Roles)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, user)
}

func (h *UsersController) setActive(active bool) userAction {
	return func(c *gin.Context, actor models.UserDTO, id string) {
		user, err := h.users.SetActive(c.Request.Context(), actor, id, active)
		if err != nil {
			fail(c, err)
			return
		}
		responses.OK(c, user)
	}
}

func (h *UsersController) Activate(c *gin.Context) {
	h.withUser(h.setActive(true))(c)
}

func (h *UsersController) Deactivate(c *gin.Context) {
	h.withUser(h.setActive(false))(c)
}

func (h *UsersController) Delete(c *gin.Context) {
	h.withUser(h.delete)(c)
}

func (h *UsersController) delete(c *gin.Context, actor models.UserDTO, id string) {
	if err := h.users.Delete(c.Request.Context(), actor, id); err != nil {
		fail(c, err)
		return
	}
	responses.NoContent(c)
}

// Guard returns the shared authentication and permission middleware provider.
