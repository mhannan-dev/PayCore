package models

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

// Standard system role codes
const (
	RoleAdmin             = "ADMIN"
	RoleOperator          = "OPERATOR"
	RoleLeadOperator      = "LEAD_OPERATOR"
	RoleComplianceAuditor = "COMPLIANCE_AUDITOR"
	RoleStaffEngineer     = "STAFF_ENGINEER"
)

// Standard system permission codes
type PermissionCode string

const (
	PermReconciliationMatch  = "reconciliation:match"
	PermReconciliationRead   = "reconciliation:read"
	PermReconciliationImport = "reconciliation:import"
	PermUsersRead            = "users:read"
	PermUsersWrite           = "users:write"
	PermTransfersRead        = "transfers:read"
	PermTransfersCreate      = "transfers:create"
	PermTransfersReverse     = "transfers:reverse"
	PermLedgerRead           = "ledger:read"
	PermAuditRead            = "audit:read"
	PermAuditVerify          = "audit:verify"
	PermReportsRead          = "reports:read"
	PermReportsExport        = "reports:export"
	PermSimulatorRun         = "simulator:run"
	PermSystemRead           = "system:read"
	PermSystemWrite          = "system:write"
	PermAccountsWrite        = "accounts:write"
	PermAttachmentsRead      = "attachments:read"
	PermAttachmentsWrite     = "attachments:write"
	PermAttachmentsDelete    = "attachments:delete"
	PermNotificationsRead    = "notifications:read"
	PermNotificationsRetry   = "notifications:retry"
)

// Valid rejects misspelled or empty route requirements instead of bypassing RBAC.
func (p PermissionCode) Valid() bool {
	switch p {
	case PermNotificationsRead, PermNotificationsRetry, PermReconciliationMatch, PermReconciliationRead, PermReconciliationImport, PermUsersRead, PermUsersWrite, PermTransfersRead, PermTransfersCreate, PermTransfersReverse,
		PermLedgerRead, PermAuditRead, PermAuditVerify, PermReportsRead, PermReportsExport,
		PermSimulatorRun, PermSystemRead, PermSystemWrite, PermAccountsWrite, PermAttachmentsRead, PermAttachmentsWrite, PermAttachmentsDelete:
		return true
	default:
		return false
	}
}

// Role represents a system role definition.
type Role struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Permission represents a fine-grained authorization capability.
type Permission struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserRole represents a junction record assigning a role to a user.
type UserRole struct {
	UserID     string    `json:"user_id"`
	RoleID     string    `json:"role_id"`
	RoleCode   string    `json:"role_code,omitempty"`
	AssignedAt time.Time `json:"assigned_at"`
}

// RolePermission represents a junction record granting a permission to a role.
type RolePermission struct {
	RoleID       string    `json:"role_id"`
	PermissionID string    `json:"permission_id"`
	AssignedAt   time.Time `json:"assigned_at"`
}

// HasPermission checks whether a slice of permission codes contains the target.
func HasPermission(permissions []string, target string) bool {
	if target == "" {
		return false
	}
	for _, p := range permissions {
		if p == target {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if any of the specified targets are present.
func HasAnyPermission(permissions []string, targets ...string) bool {
	for _, target := range targets {
		if HasPermission(permissions, target) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if all of the specified targets are present.
func HasAllPermissions(permissions []string, targets ...string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !HasPermission(permissions, target) {
			return false
		}
	}
	return true
}

// RoleDetail is a role with the permission codes it grants and how many active users hold it.
type RoleDetail struct {
	Role
	Permissions []string `json:"permissions"`
	UserCount   int      `json:"user_count"`
}

// ManagedUser is the administrator's view of a user account.
type ManagedUser struct {
	ID        string    `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  *string   `json:"last_name"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	// PasswordSet is false while the user has not accepted their invitation.
	PasswordSet bool `json:"password_set"`
	// InviteExpiresAt is the latest unused invitation's expiry, if any.
	InviteExpiresAt *time.Time `json:"invite_expires_at"`
	// InviteEmailStatus is that invitation's email delivery: pending, sent, failed,
	// not_configured or unknown.
	InviteEmailStatus *string    `json:"invite_email_status"`
	InviteEmailSentAt *time.Time `json:"invite_email_sent_at"`
	InviteEmailError  *string    `json:"invite_email_error"`
}

// UpdateUserRolesRequest replaces a user's complete role set.
type UpdateUserRolesRequest struct {
	Roles []string `json:"roles" binding:"required"`
}

// CreateUserRequest is an administrator inviting a user; the user sets their own
// password through the emailed invitation link.
type CreateUserRequest struct {
	FirstName string   `json:"first_name"`
	LastName  *string  `json:"last_name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
}

// InvitationInfo is what the set-password page may show about a valid invitation.
type InvitationInfo struct {
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InvitationTokenRequest carries an emailed invitation token (in the body, so it never appears in access logs).
type InvitationTokenRequest struct {
	Token string `json:"token" binding:"required,len=64,hexadecimal"`
}

// AcceptInvitationRequest sets the invited user's password.
type AcceptInvitationRequest struct {
	Token    string `json:"token" binding:"required,len=64,hexadecimal"`
	Password string `json:"password" binding:"required"`
}

// ErrInvalidNewUser describes why a CreateUserRequest was rejected.
type ErrInvalidNewUser struct{ Reason string }

func (e ErrInvalidNewUser) Error() string { return e.Reason }

// Normalize trims names, lowercases the email, de-duplicates roles and validates them.
func (r *CreateUserRequest) Normalize() error {
	r.FirstName = strings.TrimSpace(r.FirstName)
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	if r.LastName != nil {
		last := strings.TrimSpace(*r.LastName)
		r.LastName = &last
		if last == "" {
			r.LastName = nil
		}
	}
	switch {
	case r.FirstName == "" || utf8.RuneCountInString(r.FirstName) > 80:
		return ErrInvalidNewUser{"First name is required and must be at most 80 characters."}
	case r.LastName != nil && utf8.RuneCountInString(*r.LastName) > 80:
		return ErrInvalidNewUser{"Last name must be at most 80 characters."}
	}
	address, err := mail.ParseAddress(r.Email)
	if err != nil || address.Address != r.Email || len(r.Email) > 254 {
		return ErrInvalidNewUser{"Enter a valid email address, like name@example.com."}
	}
	roles, err := NormalizeRoles(r.Roles)
	if err != nil {
		return ErrInvalidNewUser{"Choose at least one role from ADMIN, OPERATOR, LEAD_OPERATOR, COMPLIANCE_AUDITOR and STAFF_ENGINEER."}
	}
	r.Roles = roles
	return nil
}

// UserQuery filters and pages the admin user list.
type UserQuery struct {
	Search string // matched against name and email, case-insensitive
	Role   string // a role code, or "" for any
	Limit  int
	Offset int
}

// PasswordResetRequest asks for a reset link; the captcha limits automated requests.
type PasswordResetRequest struct {
	Email         string `json:"email" binding:"required,email,max=254"`
	CaptchaID     string `json:"captcha_id" binding:"required,len=64,hexadecimal"`
	CaptchaAnswer string `json:"captcha_answer" binding:"required,max=3,numeric"`
}

// PasswordResetInfo is what the reset page may show about a valid link.
type PasswordResetInfo struct {
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ResetUser is the account a reset link would be issued for.
type ResetUser struct {
	ID        string
	Email     string
	FirstName string
}
