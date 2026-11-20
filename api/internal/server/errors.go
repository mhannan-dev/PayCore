package server

import (
	"errors"
	"net/http"

	"paycore/api/internal/apperr"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/internal/service"
)

// ErrorTable is the single mapping from domain errors to HTTP responses (coding
// rule 26). Services and repositories return these errors; handlers pass them to
// c.Error; middleware.Errors writes the matching response. First match wins.
// Unknown errors become 500 "internal_error" and are logged. Explicitly
// unconfigured dependencies can use a route-specific unavailable response.
var ErrorTable = apperr.Table{
	{Target: model.ErrMFADisabled, Error: apperr.New(http.StatusConflict, "mfa_disabled", "MFA is temporarily disabled by system configuration.")},
	{Target: model.ErrMatchInvalid, Error: apperr.BadRequest("invalid_matching_request", "Supply a valid direction mapping or review action with the current version. Rejection, reset and notes require a reason of 3–500 characters.")},
	{Target: model.ErrMatchStale, Error: apperr.New(http.StatusConflict, "matching_stale", "This review changed. Refresh matching before trying again.")},
	{Target: model.ErrMatchCandidate, Error: apperr.New(http.StatusConflict, "matching_candidate_unavailable", "The posting does not meet the exact matching criteria, was rejected, or is already reconciled. Refresh matching.")},
	{Target: model.ErrMatchModeLocked, Error: apperr.New(http.StatusConflict, "matching_mode_locked", "Reset all confirmed matches before changing the direction mapping.")},
	{Target: model.ErrStatementFormat, Error: apperr.BadRequest("invalid_statement_format", "Use the six-column CSV template with 1–2000 rows, at most 2 MiB, encoded as UTF-8.")},
	{Target: model.ErrStatementAccount, Error: apperr.BadRequest("invalid_statement_account", "Select an asset or liability account.")},
	{Target: model.ErrStatementInvalid, Error: apperr.BadRequest("invalid_statement_rows", "Fix all validation errors before importing.")},
	{Target: model.ErrStatementChanged, Error: apperr.New(http.StatusConflict, "statement_changed", "The source or preview changed. Preview the statement again.")},
	{Target: model.ErrStatementDuplicate, Error: apperr.New(http.StatusConflict, "statement_duplicate", "This statement is already imported for this account. Refresh the preview to open it.")},
	{Target: model.ErrStatementNotFound, Error: apperr.NotFound("statement_not_found", "Statement import not found.")},
	{Target: model.ErrSecuritySession, Error: apperr.New(http.StatusUnauthorized, "invalid_credentials", "Your session is no longer valid. Sign in again.")},
	{Target: model.ErrSecurityTarget, Error: apperr.NotFound("session_not_found", "Session not found.")},
	{Target: model.ErrSecurityCurrentSession, Error: apperr.New(http.StatusConflict, "current_session", "Use Sign out to end this session.")},
	{Target: model.ErrSecurityReplacement, Error: apperr.New(http.StatusConflict, "replacement_expired", "The replacement expired or belongs to another session. Start again.")},
	{Target: model.ErrMFARequired, Error: apperr.New(http.StatusUnauthorized, "mfa_required", "Sign in again to complete MFA verification.")},
	{Target: model.ErrMFAChallenge, Error: apperr.New(http.StatusUnauthorized, "mfa_challenge_invalid", "This MFA challenge expired or is no longer valid. Sign in again.")},
	{Target: model.ErrMFACode, Error: apperr.New(http.StatusUnauthorized, "mfa_code_invalid", "The code is invalid or has already been used.")},
	{Target: model.ErrMFALocked, Error: apperr.New(http.StatusTooManyRequests, "mfa_locked", "Too many MFA attempts. Try again in five minutes.").WithHeader("Retry-After", "300")},
	{Match: invalidUpload},
	{Target: service.ErrExternalAttachment, Error: apperr.New(http.StatusConflict, "legacy_external_file_not_stored_locally", "This file is stored externally and cannot be served from here.")},
	{Target: service.ErrInvalidStoredPhoto, Error: apperr.New(http.StatusUnsupportedMediaType, "invalid_photo_type", "The stored profile photo is not a WebP, JPEG, PNG or GIF image.")},
	{Target: service.ErrAttachmentPagination, Error: apperr.BadRequest("invalid_attachment_pagination", "Invalid attachment pagination.")},
	{Target: service.ErrInvalidPhoto, Error: apperr.New(http.StatusUnsupportedMediaType, "photo_must_be_jpeg_png_or_gif_max_25_megapixels", "The profile photo must be a WebP, JPEG, PNG or GIF of at most 25 megapixels (10000 px per side).")},
	// Authentication
	{Target: service.ErrCredentials, Error: apperr.New(http.StatusUnauthorized, "invalid_credentials", "Invalid credentials or session.")},
	{Target: service.ErrSession, Error: apperr.New(http.StatusUnauthorized, "invalid_credentials", "Invalid credentials or session.")},
	{Target: service.ErrCaptcha, Error: apperr.BadRequest("invalid_captcha", "CAPTCHA is invalid, expired or already used. Request a new challenge.")},
	{Target: model.ErrInvalidProfile, Error: apperr.BadRequest("invalid_profile", "First name is required; first and last name must be at most 80 characters.")},
	{Target: service.ErrPasswordLength, Error: apperr.BadRequest("invalid_password", "The password must be exactly 10 characters.")},

	// Invitation and password-reset links
	{Target: service.ErrInvitation, Error: apperr.NotFound("invitation_invalid", "This link is invalid, has expired or was already used. Ask your administrator to send a new invitation.")},
	{Target: service.ErrResetToken, Error: apperr.NotFound("reset_invalid", "This reset link is invalid, has expired or was already used. Request a new one.")},

	// User administration
	{Match: invalidNewUser},
	{Target: service.ErrInvalidRoles, Error: apperr.BadRequest("invalid_roles", "Choose at least one role from ADMIN, OPERATOR, LEAD_OPERATOR, COMPLIANCE_AUDITOR and STAFF_ENGINEER.")},
	{Target: service.ErrCannotRemoveOwnAdmin, Error: apperr.New(http.StatusConflict, "cannot_remove_own_admin", "You cannot remove your own Admin role. Ask another administrator.")},
	{Target: service.ErrCannotDeleteSelf, Error: apperr.New(http.StatusConflict, "cannot_delete_self", "You cannot delete your own account. Ask another administrator.")},
	{Target: service.ErrCannotDeactivateSelf, Error: apperr.New(http.StatusConflict, "cannot_deactivate_self", "You cannot deactivate your own account. Ask another administrator.")},
	{Target: repository.ErrEmailTaken, Error: apperr.New(http.StatusConflict, "email_taken", "A user with this email already exists.")},
	{Target: repository.ErrAlreadyActivated, Error: apperr.New(http.StatusConflict, "already_activated", "This user has already set a password.")},
	{Target: repository.ErrLastAdmin, Error: apperr.New(http.StatusConflict, "last_admin", "At least one active user must keep the Admin role.")},

	// Not found (named sentinels; a bare pgx.ErrNoRows is deliberately not mapped)
	{Target: repository.ErrUserNotFound, Error: apperr.NotFound("user_not_found", "No user has that ID.")},
	{Target: model.ErrAttachmentNotFound, Error: apperr.NotFound("attachment_not_found", "Attachment not found.")},

	// Ledger
	{Match: invalidLedgerRequest},
	{Match: insufficientFunds},
	{Target: repository.ErrAccountNotFound, Error: apperr.NotFound("account_not_found", "No ledger account has that ID.")},
	{Target: repository.ErrJournalNotFound, Error: apperr.NotFound("transfer_not_found", "No transfer has that ID.")},
	{Target: repository.ErrAccountNumberTaken, Error: apperr.New(http.StatusConflict, "account_number_taken", "An account with this account number already exists.")},
	{Target: repository.ErrAccountInactive, Error: apperr.New(http.StatusUnprocessableEntity, "account_inactive", "One of the accounts is inactive and cannot receive postings.")},
	{Target: repository.ErrCurrencyMismatch, Error: apperr.New(http.StatusUnprocessableEntity, "currency_mismatch", "Every account in a transfer must use the transfer's currency.")},
	{Target: repository.ErrAlreadyReversed, Error: apperr.New(http.StatusConflict, "already_reversed", "This transfer has already been reversed.")},
	{Target: service.ErrCannotReverseReversal, Error: apperr.New(http.StatusConflict, "cannot_reverse_reversal", "A reversal cannot itself be reversed.")},
	{Target: repository.ErrIdempotencyMismatch, Error: apperr.New(http.StatusUnprocessableEntity, "idempotency_key_reused", "This Idempotency-Key was already used with a different request. Use a new key for a new transfer.")},
	{Target: repository.ErrIdempotencyInFlight, Error: apperr.New(http.StatusConflict, "idempotency_in_flight", "A request with this Idempotency-Key is still being processed. Retry shortly with the same key.").WithHeader("Retry-After", "1")},
	{Target: repository.ErrLedgerBusy, Error: apperr.Unavailable("ledger_busy", "The accounts are busy with other transfers. Retry shortly with the same Idempotency-Key.").WithHeader("Retry-After", "1")},

	// Infrastructure
	{Target: service.ErrUnavailable, Error: apperr.Unavailable("auth_unavailable", "The authentication service is unavailable. Try again shortly.")},
}

func invalidUpload(err error) (*apperr.Error, bool) {
	var invalid *model.UploadError
	if !errors.As(err, &invalid) {
		return nil, false
	}
	status := http.StatusBadRequest
	var message string
	switch invalid.Code {
	case model.UploadTooLarge:
		status = http.StatusRequestEntityTooLarge
		message = "The request is too large."
	case model.UploadInvalidSize:
		status = http.StatusRequestEntityTooLarge
		message = "The file must be between 1 byte and 10 MiB."
	case model.UploadInvalidMultipart:
		message = "Send the file as multipart/form-data."
	case model.UploadOneFileRequired:
		message = "Send exactly one file in the \"file\" field."
	case model.UploadInvalidName:
		message = "The file name is missing or not allowed."
	case model.UploadInvalidFile:
		message = "The file could not be read."
	case model.UploadImageBusy:
		status = http.StatusServiceUnavailable
		message = "Image conversion is busy. Try again shortly."
	case model.UploadUnsupportedImage:
		status = http.StatusUnsupportedMediaType
		message = "Use a JPEG, PNG, GIF, WebP, BMP or TIFF image. Other image formats cannot be converted."
	case model.UploadInvalidImage:
		message = "The image is corrupt or could not be decoded."
	case model.UploadUnsupportedAnimation:
		status = http.StatusUnsupportedMediaType
		message = "This animated image format is not supported. Use an animated GIF or a still image."
	case model.UploadImageDimensions:
		status = http.StatusUnsupportedMediaType
		message = "Images must fit within 10000 pixels per side and 25 megapixels; GIFs allow up to 200 frames and 25 million total frame pixels."
	default:
		return nil, false
	}
	return apperr.New(status, string(invalid.Code), message), true
}

// invalidNewUser answers 400 with the specific reason from CreateUserRequest.Normalize.
func invalidNewUser(err error) (*apperr.Error, bool) {
	var invalid model.ErrInvalidNewUser
	if !errors.As(err, &invalid) {
		return nil, false
	}
	return apperr.BadRequest("invalid_user", invalid.Reason), true
}

// invalidLedgerRequest answers 400 with the request's own code and reason.
func invalidLedgerRequest(err error) (*apperr.Error, bool) {
	var invalid model.InvalidRequestError
	if !errors.As(err, &invalid) {
		return nil, false
	}
	return apperr.BadRequest(invalid.Code, invalid.Reason), true
}

// insufficientFunds answers 422 naming the account and its balance.
func insufficientFunds(err error) (*apperr.Error, bool) {
	var funds repository.InsufficientFundsError
	if !errors.As(err, &funds) {
		return nil, false
	}
	e := apperr.New(http.StatusUnprocessableEntity, "insufficient_funds",
		"Account "+funds.AccountNumber+" has insufficient funds for this transfer.")
	e.Details = map[string]string{"account_number": funds.AccountNumber, "balance": funds.Balance, "change": funds.Change}
	return e, true
}
