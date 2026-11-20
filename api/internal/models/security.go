package models

import (
	"errors"
	"time"
)

var ErrSecuritySession = errors.New("security session invalid")
var ErrSecurityTarget = errors.New("session not found")
var ErrSecurityCurrentSession = errors.New("use sign out to revoke the current session")
var ErrSecurityReplacement = errors.New("authenticator replacement is unavailable or expired")

type SecurityAction string

const (
	SecurityRecoveryCodes  SecurityAction = "recovery_codes"
	SecurityReplaceBegin   SecurityAction = "replace_begin"
	SecurityReplaceConfirm SecurityAction = "replace_confirm"
	SecurityRevokeSession  SecurityAction = "revoke_session"
	SecurityRevokeOthers   SecurityAction = "revoke_others"
)

type SecurityProof struct {
	Password string `json:"password" binding:"required,max=72"`
	Code     string `json:"code" binding:"max=64"`
}
type SessionInfo struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	MFAVerified bool      `json:"mfa_verified"`
	Current     bool      `json:"current"`
}
type SecurityOverview struct {
	MFAEnforced            bool          `json:"mfa_enforced"` // deployment policy; enrolled secrets remain stored when false
	MFAEnabled             bool          `json:"mfa_enabled"`
	MFARequired            bool          `json:"mfa_required"`
	EnabledAt              *time.Time    `json:"enabled_at"`
	RecoveryCodesRemaining int           `json:"recovery_codes_remaining"`
	Sessions               []SessionInfo `json:"sessions"`
	MoreSessions           bool          `json:"more_sessions"`
}
type RecoveryCodesResult struct {
	RecoveryCodes []string `json:"recovery_codes"`
}
type ReplacementSetup struct {
	Enrollment MFAEnrollment `json:"enrollment"`
	ExpiresIn  int64         `json:"expires_in"`
}
type SecurityActionResult struct {
	RevokedCount int64 `json:"revoked_count"`
}
type ReplacementResult struct {
	RecoveryCodes []string `json:"recovery_codes"`
	RevokedCount  int64    `json:"revoked_count"`
}

// Transaction commands and locked state never form a public API payload.
type SecurityCommand struct {
	Action        SecurityAction `json:"-"`
	TargetID      string         `json:"-"`
	PendingSecret []byte         `json:"-"`
	ExpiresAt     time.Time      `json:"-"`
}
type SecurityRecord struct {
	User      User      `json:"-"`
	MFA       MFARecord `json:"-"`
	SessionID string    `json:"-"`
}
type SecurityDecide func(SecurityRecord) (MFAApproval, error)
