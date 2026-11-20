package services

import (
	"context"
	"errors"
	"testing"

	"paycore/api/internal/models"
)

type adminStore struct {
	setRoles []string
	deleted  string
	active   *bool
}

func (s *adminStore) ListRoleDetails(context.Context) ([]models.RoleDetail, error) { return nil, nil }
func (s *adminStore) ListPermissions(context.Context) ([]models.Permission, error) { return nil, nil }
func (s *adminStore) ListUsers(context.Context, models.UserQuery) ([]models.ManagedUser, int, error) {
	return nil, 0, nil
}
func (s *adminStore) SetUserRoles(_ context.Context, id string, roles []string) (models.ManagedUser, error) {
	s.setRoles = roles
	return models.ManagedUser{ID: id, Roles: roles}, nil
}
func (s *adminStore) DeleteUser(_ context.Context, id string) error { s.deleted = id; return nil }
func (s *adminStore) SetUserActive(_ context.Context, id string, active bool) (models.ManagedUser, error) {
	s.active = &active
	return models.ManagedUser{ID: id, IsActive: active}, nil
}

func TestUserAdminRules(t *testing.T) {
	ctx := context.Background()
	admin := models.UserDTO{ID: "admin", Roles: []string{"ADMIN"}}
	store := &adminStore{}
	svc := NewUserAdminService(store, nil)

	if _, err := svc.SetRoles(ctx, admin, "member", []string{"ROOT"}); !errors.Is(err, ErrInvalidRoles) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, err := svc.SetRoles(ctx, admin, "member", nil); !errors.Is(err, ErrInvalidRoles) {
		t.Fatalf("no roles: %v", err)
	}
	if _, err := svc.SetRoles(ctx, admin, "admin", []string{"OPERATOR"}); !errors.Is(err, ErrCannotRemoveOwnAdmin) {
		t.Fatalf("own admin: %v", err)
	}
	if u, err := svc.SetRoles(ctx, admin, "member", []string{" operator ", "OPERATOR", "compliance_auditor"}); err != nil ||
		len(store.setRoles) != 2 || store.setRoles[0] != "OPERATOR" || u.ID != "member" {
		t.Fatalf("normalized roles: %v %v", store.setRoles, err)
	}
	if err := svc.Delete(ctx, admin, "admin"); !errors.Is(err, ErrCannotDeleteSelf) || store.deleted != "" {
		t.Fatalf("self delete: %v", err)
	}
	if _, err := svc.SetActive(ctx, admin, "admin", false); !errors.Is(err, ErrCannotDeactivateSelf) || store.active != nil {
		t.Fatalf("self deactivate: %v", err)
	}
	if _, err := svc.SetActive(ctx, admin, "admin", true); err != nil {
		t.Fatalf("self activate is harmless: %v", err)
	}
	if _, _, err := svc.Invite(ctx, admin, models.CreateUserRequest{}); err == nil {
		t.Fatal("invite without an invitation service must fail")
	}
	if p, err := svc.Permissions(ctx); err != nil || p == nil {
		t.Fatalf("permissions should be an empty list, not nil: %v %v", p, err)
	}
}
