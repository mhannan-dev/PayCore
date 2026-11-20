package services

import (
	"context"
	"errors"

	"paycore/api/internal/models"
)

// Business-rule errors for user administration (mapped to HTTP in internal/server).
var (
	ErrInvalidRoles          = errors.New("at least one known role is required")
	ErrCannotRemoveOwnAdmin  = errors.New("administrators cannot remove their own Admin role")
	ErrCannotDeleteSelf      = errors.New("users cannot delete their own account")
	ErrCannotDeactivateSelf  = errors.New("users cannot deactivate their own account")
	errInvitationsNotEnabled = errors.New("invitations are not configured")
)

// UserAdminStore is the data access user administration needs (implemented by
// repositories.RBACRepository).
type UserAdminStore interface {
	ListRoleDetails(context.Context) ([]models.RoleDetail, error)
	ListPermissions(context.Context) ([]models.Permission, error)
	ListUsers(context.Context, models.UserQuery) ([]models.ManagedUser, int, error)
	SetUserRoles(ctx context.Context, userID string, roles []string) (models.ManagedUser, error)
	DeleteUser(ctx context.Context, userID string) error
	SetUserActive(ctx context.Context, userID string, active bool) (models.ManagedUser, error)
}

// Inviter creates and re-sends invitations (implemented by InvitationService).
type Inviter interface {
	Invite(ctx context.Context, req models.CreateUserRequest, invitedBy string) (models.ManagedUser, string, error)
	Resend(ctx context.Context, userID, invitedBy string) (models.ManagedUser, string, error)
}

// UserAdminService holds the rules for managing other users' accounts. The
// "last active administrator" rule is enforced by the store inside its
// transaction; the rules about the acting user are enforced here.
type UserAdminService struct {
	store   UserAdminStore
	invites Inviter
}

// NewUserAdminService: invites may be nil (inviting then reports unavailable).
func NewUserAdminService(store UserAdminStore, invites Inviter) *UserAdminService {
	return &UserAdminService{store: store, invites: invites}
}

func (s *UserAdminService) Roles(ctx context.Context) ([]models.RoleDetail, error) {
	return s.store.ListRoleDetails(ctx)
}

func (s *UserAdminService) Permissions(ctx context.Context) ([]models.Permission, error) {
	permissions, err := s.store.ListPermissions(ctx)
	if permissions == nil && err == nil {
		permissions = []models.Permission{}
	}
	return permissions, err
}

func (s *UserAdminService) Users(ctx context.Context, q models.UserQuery) ([]models.ManagedUser, int, error) {
	return s.store.ListUsers(ctx, q)
}

// Invite normalizes the request (returning models.ErrInvalidNewUser) and sends an invitation.
func (s *UserAdminService) Invite(ctx context.Context, actor models.UserDTO, req models.CreateUserRequest) (models.ManagedUser, string, error) {
	if s.invites == nil {
		return models.ManagedUser{}, "", errInvitationsNotEnabled
	}
	if err := req.Normalize(); err != nil {
		return models.ManagedUser{}, "", err
	}
	return s.invites.Invite(ctx, req, actor.ID)
}

func (s *UserAdminService) ResendInvitation(ctx context.Context, actor models.UserDTO, userID string) (models.ManagedUser, string, error) {
	if s.invites == nil {
		return models.ManagedUser{}, "", errInvitationsNotEnabled
	}
	return s.invites.Resend(ctx, userID, actor.ID)
}

// SetRoles replaces a user's roles. An administrator may not drop their own Admin role.
func (s *UserAdminService) SetRoles(ctx context.Context, actor models.UserDTO, userID string, roles []string) (models.ManagedUser, error) {
	normalized, err := models.NormalizeRoles(roles)
	if err != nil {
		return models.ManagedUser{}, ErrInvalidRoles
	}
	if actor.ID == userID && hasRole(actor.Roles, models.RoleAdmin) && !hasRole(normalized, models.RoleAdmin) {
		return models.ManagedUser{}, ErrCannotRemoveOwnAdmin
	}
	return s.store.SetUserRoles(ctx, userID, normalized)
}

// Delete permanently removes another user's account.
func (s *UserAdminService) Delete(ctx context.Context, actor models.UserDTO, userID string) error {
	if actor.ID == userID {
		return ErrCannotDeleteSelf
	}
	return s.store.DeleteUser(ctx, userID)
}

// SetActive deactivates (not yourself) or reactivates a user.
func (s *UserAdminService) SetActive(ctx context.Context, actor models.UserDTO, userID string, active bool) (models.ManagedUser, error) {
	if !active && actor.ID == userID {
		return models.ManagedUser{}, ErrCannotDeactivateSelf
	}
	return s.store.SetUserActive(ctx, userID, active)
}

func hasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
