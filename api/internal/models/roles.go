package models

import (
	"errors"
	"strings"
)

// NormalizeRoles validates a nonempty role set and removes repeated entries.
func NormalizeRoles(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one role is required")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		role := strings.ToUpper(strings.TrimSpace(value))
		switch role {
		case RoleAccountant, RoleAdmin, RoleComplianceAuditor:
		default:
			return nil, errors.New("unsupported role")
		}
		if !seen[role] {
			result = append(result, role)
			seen[role] = true
		}
	}
	return result, nil
}

func (u User) HasRole(role string) bool {
	for _, assigned := range u.Roles {
		if assigned == role {
			return true
		}
	}
	return false
}
