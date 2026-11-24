package server

import (
	"errors"
	"net/http"

	"paycore/api/internal/apperr"
	"paycore/api/internal/models"
	"paycore/api/internal/repositories"
	"paycore/api/internal/services"
)

// ErrorTable is the single mapping from domain errors to HTTP responses (coding
// rule 26). Services and repositories return these errors; controllers pass them to
// c.Error; middleware.Errors writes the matching responses. First match wins.
// Unknown errors become 500 "internal_error" and are logged. Explicitly
// unconfigured dependencies can use a route-specific unavailable responses.
var ErrorTable = apperr.Table{
	{Target: models.ErrQuantumMerchantExists, Error: apperr.Conflict("merchant_exists", "This user already has a Quantum merchant.")},
	{Target: models.ErrQuantumMerchantNotFound, Error: apperr.NotFound("merchant_not_found", "Create a Quantum merchant first.")},
	{Target: models.ErrQuantumInvoiceNotFound, Error: apperr.NotFound("invoice_not_found", "Invoice not found.")},
	{Target: models.ErrQuantumCheckoutNotFound, Error: apperr.NotFound("checkout_not_found", "Checkout not found.")},
	{Target: models.ErrQuantumNotPending, Error: apperr.Conflict("invoice_not_pending", "This invoice is not accepting new payments.")},
	{Target: models.ErrQuantumTxSubmitted, Error: apperr.Conflict("tx_already_submitted", "A transaction is already attached to this attempt. Poll its status.")},
	{Target: models.ErrQuantumAttemptExpired, Error: apperr.Conflict("attempt_expired", "This unused payment intent expired. Register a new attempt before signing a transfer.")},
	{Target: models.ErrQuantumAmbiguous, Error: apperr.Conflict("payment_ambiguous", "An indistinguishable payment intent is already open. Finish or wait for that intent before paying another invoice.")},
	{Target: models.ErrQuantumDuplicate, Error: apperr.Conflict("duplicate_transfer", "This transfer is already claimed.")},
	{Target: models.ErrQuantumIdempotency, Error: apperr.Conflict("idempotency_conflict", "The same Idempotency-Key was used with a different invoice request.")},
	{Target: models.ErrQuantumAttempts, Error: apperr.New(http.StatusTooManyRequests, "too_many_attempts", "At most five payment attempts may be open on an invoice.").WithHeader("Retry-After", "60")},
	{Target: models.ErrDeliveryNotFound, Error: apperr.NotFound("delivery_not_found", "Notification delivery not found.")},
	{Target: models.ErrDeliveryStale, Error: apperr.Conflict("delivery_stale", "This delivery changed or is not ready for manual retry. Refresh its details.")},
	{Target: models.ErrPayoutNotFound, Error: apperr.NotFound("settlement_not_found", "Settlement not found.")},
	{Target: models.ErrPayoutSource, Error: apperr.BadRequest("invalid_source_account", "Select an active customer liability wallet in this currency; clearing accounts cannot fund payouts.")},
	{Target: models.ErrPayoutClearing, Error: apperr.Conflict("clearing_account_missing", "An active payout clearing liability account is required in this currency.")},
	{Target: models.ErrPayoutBank, Error: apperr.Conflict("operating_bank_missing", "An active operating bank asset account is required in this currency.")},
	{Target: models.ErrPayoutState, Error: apperr.Conflict("invalid_settlement_state", "Only RESERVED payouts can be captured or released.")},
	{Target: models.ErrPayoutProvider, Error: apperr.Conflict("unsupported_provider", "Only sandbox payouts can be completed with this workflow.")},
	{Target: models.ErrMFADisabled, Error: apperr.New(http.StatusConflict, "mfa_disabled", "MFA is temporarily disabled by system configuration.")},
	{Target: models.ErrMatchInvalid, Error: apperr.BadRequest("invalid_matching_request", "Supply a valid direction mapping or review action with the current version. Rejection, reset and notes require a reason of 3–500 characters.")},
	{Target: models.ErrMatchStale, Error: apperr.New(http.StatusConflict, "matching_stale", "This review changed. Refresh matching before trying again.")},
	{Target: models.ErrMatchCandidate, Error: apperr.New(http.StatusConflict, "matching_candidate_unavailable", "The posting does not meet the exact matching criteria, was rejected, or is already reconciled. Refresh matching.")},
	{Target: models.ErrMatchModeLocked, Error: apperr.New(http.StatusConflict, "matching_mode_locked", "Reset all confirmed matches before changing the direction mapping.")},
	{Target: models.ErrStatementFormat, Error: apperr.BadRequest("invalid_statement_format", "Use the six-column CSV template with 1–2000 rows, at most 2 MiB, encoded as UTF-8.")},
	{Target: models.ErrStatementAccount, Error: apperr.BadRequest("invalid_statement_account", "Select an asset or liability account.")},
	{Target: models.ErrStatementInvalid, Error: apperr.BadRequest("invalid_statement_rows", "Fix all validation errors before importing.")},
	{Target: models.ErrStatementChanged, Error: apperr.New(http.StatusConflict, "statement_changed", "The source or preview changed. Preview the statement again.")},
	{Target: models.ErrStatementDuplicate, Error: apperr.New(http.StatusConflict, "statement_duplicate", "This statement is already imported for this account. Refresh the preview to open it.")},
	{Target: models.ErrStatementNotFound, Error: apperr.NotFound("statement_not_found", "Statement import not found.")},
	{Target: models.ErrSecuritySession, Error: apperr.New(http.StatusUnauthorized, "invalid_credentials", "Your session is no longer valid. Sign in again.")},
	{Target: models.ErrSecurityTarget, Error: apperr.NotFound("session_not_found", "Session not found.")},
	{Target: models.ErrSecurityCurrentSession, Error: apperr.New(http.StatusConflict, "current_session", "Use Sign out to end this session.")},
	{Target: models.ErrSecurityReplacement, Error: apperr.New(http.StatusConflict, "replacement_expired", "The replacement expired or belongs to another session. Start again.")},
	{Target: models.ErrMFARequired, Error: apperr.New(http.StatusUnauthorized, "mfa_required", "Sign in again to complete MFA verification.")},
	{Target: models.ErrMFAChallenge, Error: apperr.New(http.StatusUnauthorized, "mfa_challenge_invalid", "This MFA challenge expired or is no longer valid. Sign in again.")},
	{Target: models.ErrMFACode, Error: apperr.New(http.StatusUnauthorized, "mfa_code_invalid", "The code is invalid or has already been used.")},
	{Target: models.ErrMFALocked, Error: apperr.New(http.StatusTooManyRequests, "mfa_locked", "Too many MFA attempts. Try again in five minutes.").WithHeader("Retry-After", "300")},
	{Match: invalidUpload},
	{Target: services.ErrExternalAttachment, Error: apperr.New(http.StatusConflict, "legacy_external_file_not_stored_locally", "This file is stored externally and cannot be served from here.")},
	{Target: services.ErrInvalidStoredPhoto, Error: apperr.New(http.StatusUnsupportedMediaType, "invalid_photo_type", "The stored profile photo is not a WebP, JPEG, PNG or GIF image.")},
	{Target: services.ErrAttachmentPagination, Error: apperr.BadRequest("invalid_attachment_pagination", "Invalid attachment pagination.")},
	{Target: services.ErrInvalidPhoto, Error: apperr.New(http.StatusUnsupportedMediaType, "photo_must_be_jpeg_png_or_gif_max_25_megapixels", "The profile photo must be a WebP, JPEG, PNG or GIF of at most 25 megapixels (10000 px per side).")},
	// Authentication
	{Target: services.ErrCredentials, Error: apperr.New(http.StatusUnauthorized, "invalid_credentials", "Invalid credentials or session.")},
	{Target: services.ErrSession, Error: apperr.New(http.StatusUnauthorized, "invalid_credentials", "Invalid credentials or session.")},
	{Target: services.ErrCaptcha, Error: apperr.BadRequest("invalid_captcha", "CAPTCHA is invalid, expired or already used. Request a new challenge.")},
	{Target: models.ErrInvalidProfile, Error: apperr.BadRequest("invalid_profile", "First name is required; first and last name must be at most 80 characters.")},
	{Target: services.ErrPasswordLength, Error: apperr.BadRequest("invalid_password", "The password must be exactly 10 characters.")},

	// Invitation and password-reset links
	{Target: services.ErrInvitation, Error: apperr.NotFound("invitation_invalid", "This link is invalid, has expired or was already used. Ask your administrator to send a new invitation.")},
	{Target: services.ErrResetToken, Error: apperr.NotFound("reset_invalid", "This reset link is invalid, has expired or was already used. Request a new one.")},

	// User administration
	{Match: invalidNewUser},
	{Target: services.ErrInvalidRoles, Error: apperr.BadRequest("invalid_roles", "Choose at least one role from ADMIN, ACCOUNTANT and COMPLIANCE_AUDITOR.")},
	{Target: services.ErrCannotRemoveOwnAdmin, Error: apperr.New(http.StatusConflict, "cannot_remove_own_admin", "You cannot remove your own Admin role. Ask another administrator.")},
	{Target: services.ErrCannotDeleteSelf, Error: apperr.New(http.StatusConflict, "cannot_delete_self", "You cannot delete your own account. Ask another administrator.")},
	{Target: services.ErrCannotDeactivateSelf, Error: apperr.New(http.StatusConflict, "cannot_deactivate_self", "You cannot deactivate your own account. Ask another administrator.")},
	{Target: repositories.ErrEmailTaken, Error: apperr.New(http.StatusConflict, "email_taken", "A user with this email already exists.")},
	{Target: repositories.ErrAlreadyActivated, Error: apperr.New(http.StatusConflict, "already_activated", "This user has already set a password.")},
	{Target: repositories.ErrLastAdmin, Error: apperr.New(http.StatusConflict, "last_admin", "At least one active user must keep the Admin role.")},
	{Target: repositories.ErrUserHasMerchant, Error: apperr.Conflict("user_has_merchant", "This user owns a Quantum merchant profile and cannot be deleted. Deactivate the user to revoke access while preserving payment history.")},

	// Not found (named sentinels; a bare pgx.ErrNoRows is deliberately not mapped)
	{Target: repositories.ErrUserNotFound, Error: apperr.NotFound("user_not_found", "No user has that ID.")},
	{Target: models.ErrAttachmentNotFound, Error: apperr.NotFound("attachment_not_found", "Attachment not found.")},

	// Ledger
	{Match: invalidLedgerRequest},
	{Match: insufficientFunds},
	{Target: repositories.ErrAccountNotFound, Error: apperr.NotFound("account_not_found", "No ledger account has that ID.")},
	{Target: repositories.ErrJournalNotFound, Error: apperr.NotFound("transfer_not_found", "No transfer has that ID.")},
	{Target: repositories.ErrAccountNumberTaken, Error: apperr.New(http.StatusConflict, "account_number_taken", "An account with this account number already exists.")},
	{Target: repositories.ErrSystemRoleTaken, Error: apperr.New(http.StatusConflict, "system_role_taken", "Another account in this currency already has that system role.")},
	{Target: repositories.ErrAccountInactive, Error: apperr.New(http.StatusUnprocessableEntity, "account_inactive", "One of the accounts is inactive and cannot receive postings.")},
	{Target: repositories.ErrCurrencyMismatch, Error: apperr.New(http.StatusUnprocessableEntity, "currency_mismatch", "Every account in a transfer must use the transfer's currency.")},
	{Target: repositories.ErrAlreadyReversed, Error: apperr.New(http.StatusConflict, "already_reversed", "This transfer has already been reversed.")},
	{Target: services.ErrCannotReverseReversal, Error: apperr.New(http.StatusConflict, "cannot_reverse_reversal", "A reversal cannot itself be reversed.")},
	{Target: repositories.ErrIdempotencyMismatch, Error: apperr.New(http.StatusUnprocessableEntity, "idempotency_key_reused", "This Idempotency-Key was already used with a different request. Use a new key for a new transfer.")},
	{Target: repositories.ErrIdempotencyInFlight, Error: apperr.New(http.StatusConflict, "idempotency_in_flight", "A request with this Idempotency-Key is still being processed. Retry shortly with the same key.").WithHeader("Retry-After", "1")},
	{Target: repositories.ErrLedgerBusy, Error: apperr.Unavailable("ledger_busy", "The accounts are busy with other transfers. Retry shortly with the same Idempotency-Key.").WithHeader("Retry-After", "1")},

	// Infrastructure
	{Target: services.ErrUnavailable, Error: apperr.Unavailable("auth_unavailable", "The authentication service is unavailable. Try again shortly.")},
	{Target: services.ErrOutboxPublisher, Error: apperr.Unavailable("outbox_unavailable", "Event delivery is not configured. Try again shortly.")},
	{Target: services.ErrInvalidOutboxReplay, Error: apperr.BadRequest("invalid_outbox_replay", "Replay only accepts valid event IDs (at most 200).")},
}

func invalidUpload(err error) (*apperr.Error, bool) {
	var invalid *models.UploadError
	if !errors.As(err, &invalid) {
		return nil, false
	}
	status := http.StatusBadRequest
	var message string
	switch invalid.Code {
	case models.UploadTooLarge:
		status = http.StatusRequestEntityTooLarge
		message = "The request is too large."
	case models.UploadInvalidSize:
		status = http.StatusRequestEntityTooLarge
		message = "The file must be between 1 byte and 10 MiB."
	case models.UploadInvalidMultipart:
		message = "Send the file as multipart/form-data."
	case models.UploadOneFileRequired:
		message = "Send exactly one file in the \"file\" field."
	case models.UploadInvalidName:
		message = "The file name is missing or not allowed."
	case models.UploadInvalidFile:
		message = "The file could not be read."
	case models.UploadImageBusy:
		status = http.StatusServiceUnavailable
		message = "Image conversion is busy. Try again shortly."
	case models.UploadUnsupportedImage:
		status = http.StatusUnsupportedMediaType
		message = "Use a JPEG, PNG, GIF, WebP, BMP or TIFF image. Other image formats cannot be converted."
	case models.UploadInvalidImage:
		message = "The image is corrupt or could not be decoded."
	case models.UploadUnsupportedAnimation:
		status = http.StatusUnsupportedMediaType
		message = "This animated image format is not supported. Use an animated GIF or a still image."
	case models.UploadImageDimensions:
		status = http.StatusUnsupportedMediaType
		message = "Images must fit within 10000 pixels per side and 25 megapixels; GIFs allow up to 200 frames and 25 million total frame pixels."
	default:
		return nil, false
	}
	return apperr.New(status, string(invalid.Code), message), true
}

// invalidNewUser answers 400 with the specific reason from CreateUserRequest.Normalize.
func invalidNewUser(err error) (*apperr.Error, bool) {
	var invalid models.ErrInvalidNewUser
	if !errors.As(err, &invalid) {
		return nil, false
	}
	return apperr.BadRequest("invalid_user", invalid.Reason), true
}

// invalidLedgerRequest answers 400 with the request's own code and reason.
func invalidLedgerRequest(err error) (*apperr.Error, bool) {
	var invalid models.InvalidRequestError
	if !errors.As(err, &invalid) {
		return nil, false
	}
	return apperr.BadRequest(invalid.Code, invalid.Reason), true
}

// insufficientFunds answers 422 naming the account and its balance.
func insufficientFunds(err error) (*apperr.Error, bool) {
	var funds repositories.InsufficientFundsError
	if !errors.As(err, &funds) {
		return nil, false
	}
	e := apperr.New(http.StatusUnprocessableEntity, "insufficient_funds",
		"Account "+funds.AccountNumber+" has insufficient funds for this transfer.")
	e.Details = map[string]string{"account_number": funds.AccountNumber, "balance": funds.Balance, "change": funds.Change}
	return e, true
}
