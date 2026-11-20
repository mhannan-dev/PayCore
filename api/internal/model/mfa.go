package model

import (
	"errors"
	"time"
)

var ErrMFARequired = errors.New("MFA verification required")
var ErrMFADisabled = errors.New("MFA is temporarily disabled by system configuration")
var ErrMFAChallenge = errors.New("invalid or expired MFA challenge")
var ErrMFACode = errors.New("invalid or already used MFA code")
var ErrMFALocked = errors.New("MFA attempts temporarily locked")

type MFAState string

const (
	MFAEnrollmentRequired   MFAState = "mfa_enrollment_required"
	MFAVerificationRequired MFAState = "mfa_required"
)

type MFAEnrollment struct {
	Secret  string `json:"secret"`
	URI     string `json:"uri"`
	QRImage string `json:"qr_image"`
}
type MFAChallengeResponse struct {
	State          MFAState       `json:"state"`
	ChallengeToken string         `json:"challenge_token"`
	ExpiresIn      int64          `json:"expires_in"`
	Enrollment     *MFAEnrollment `json:"enrollment"`
}
type MFAVerifyRequest struct {
	ChallengeToken string `json:"challenge_token" binding:"required,len=64,hexadecimal"`
	Code           string `json:"code" binding:"required,max=64"`
}
type MFALoginResult struct {
	Session       LoginResponse `json:"session"`
	RecoveryCodes []string      `json:"recovery_codes"`
}

// Internal state contains encrypted secrets and hashes, never JSON responses.
type MFARecord struct {
	UserID         string   `json:"-"`
	Secret         []byte   `json:"-"`
	PendingSecret  []byte   `json:"-"`
	LastCounter    int64    `json:"-"`
	RecoveryHashes [][]byte `json:"-"`
}
type MFAApproval struct {
	Counter           int64    `json:"-"`
	RecoveryHash      []byte   `json:"-"`
	NewRecoveryHashes [][]byte `json:"-"`
}

// MFADecide runs under the user/MFA locks. It performs only local cryptography.
type MFADecide func(MFARecord) (MFAApproval, error)
type MFAChallenge struct {
	Hash          []byte    `json:"-"`
	PendingSecret []byte    `json:"-"`
	ExpiresAt     time.Time `json:"-"`
}
