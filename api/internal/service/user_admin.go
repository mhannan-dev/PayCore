package service

import (
	"context"
	"errors"

	"paycore/api/internal/model"
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
// repository.RBACRepository).
type UserAdminStore interface {
	ListRoleDetails(context.Context) ([]model.RoleDetail, error)
	ListPermissions(context.Context) ([]model.Permission, error)
	ListUsers(context.Context, model.UserQuery) ([]model.ManagedUser, int, error)
	SetUserRoles(ctx context.Context, userID string, roles []string) (model.ManagedUser, error)
	DeleteUser(ctx context.Context, userID string) error
	SetUserActive(ctx context.Context, userID string, active bool) (model.ManagedUser, error)
}

// Inviter creates and re-sends invitations (implemented by InvitationService).
type Inviter interface {
	Invite(ctx context.Context, req model.CreateUserRequest, invitedBy string) (model.ManagedUser, string, error)
	Resend(ctx context.Context, userID, invitedBy string) (model.ManagedUser, string, error)
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

func (s *UserAdminService) Roles(ctx context.Context) ([]model.RoleDetail, error) {
	return s.store.ListRoleDetails(ctx)
}

func (s *UserAdminService) Permissions(ctx context.Context) ([]model.Permission, error) {
	permissions, err := s.store.ListPermissions(ctx)
	if permissions == nil && err == nil {
		permissions = []model.Permission{}
	}
	return permissions, err
}

func (s *UserAdminService) Users(ctx context.Context, q model.UserQuery) ([]model.ManagedUser, int, error) {
	return s.store.ListUsers(ctx, q)
}

// Invite normalizes the request (returning model.ErrInvalidNewUser) and sends an invitation.
func (s *UserAdminService) Invite(ctx context.Context, actor model.UserDTO, req model.CreateUserRequest) (model.ManagedUser, string, error) {
	if s.invites == nil {
		return model.ManagedUser{}, "", errInvitationsNotEnabled
	}
	if err := req.Normalize(); err != nil {
		return model.ManagedUser{}, "", err
	}
	return s.invites.Invite(ctx, req, actor.ID)
}

func (s *UserAdminService) ResendInvitation(ctx context.Context, actor model.UserDTO, userID string) (model.ManagedUser, string, error) {
	if s.invites == nil {
		return model.ManagedUser{}, "", errInvitationsNotEnabled
	}
	return s.invites.Resend(ctx, userID, actor.ID)
}

// SetRoles replaces a user's roles. An administrator may not drop their own Admin role.
func (s *UserAdminService) SetRoles(ctx context.Context, actor model.UserDTO, userID string, roles []string) (model.ManagedUser, error) {
	normalized, err := model.NormalizeRoles(roles)
	if err != nil {
		return model.ManagedUser{}, ErrInvalidRoles
	}
	if actor.ID == userID && hasRole(actor.Roles, model.RoleAdmin) && !hasRole(normalized, model.RoleAdmin) {
		return model.ManagedUser{}, ErrCannotRemoveOwnAdmin
	}
	return s.store.SetUserRoles(ctx, userID, normalized)
}

// Delete permanently removes another user's account.
func (s *UserAdminService) Delete(ctx context.Context, actor model.UserDTO, userID string) error {
	if actor.ID == userID {
		return ErrCannotDeleteSelf
	}
	return s.store.DeleteUser(ctx, userID)
}

// SetActive deactivates (not yourself) or reactivates a user.
func (s *UserAdminService) SetActive(ctx context.Context, actor model.UserDTO, userID string, active bool) (model.ManagedUser, error) {
	if !active && actor.ID == userID {
		return model.ManagedUser{}, ErrCannotDeactivateSelf
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
