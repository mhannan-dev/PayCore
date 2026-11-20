package models

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// UpdateProfileRequest is the body of PATCH /api/v1/auth/me. Only name fields
// are editable; email, roles and active status are not.
type UpdateProfileRequest struct {
	FirstName string  `json:"first_name"`
	LastName  *string `json:"last_name"`
}

var ErrInvalidProfile = errors.New("first_name is required; first_name and last_name must be at most 80 characters")

// Normalize trims both names and turns a blank last name into NULL, matching
// the users table constraints (1–80 trimmed characters each).
func (r UpdateProfileRequest) Normalize() (string, *string, error) {
	first := strings.TrimSpace(r.FirstName)
	if first == "" || utf8.RuneCountInString(first) > 80 {
		return "", nil, ErrInvalidProfile
	}
	if r.LastName == nil {
		return first, nil, nil
	}
	last := strings.TrimSpace(*r.LastName)
	if last == "" {
		return first, nil, nil
	}
	if utf8.RuneCountInString(last) > 80 {
		return "", nil, ErrInvalidProfile
	}
	return first, &last, nil
}
