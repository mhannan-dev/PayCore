package apperr

import "errors"

// ErrNotConfigured selects an explicitly configured route-area 503 responses.
// Other unexpected failures must not be mislabeled as temporary unavailability.
var ErrNotConfigured = errors.New("controller dependencies are not configured")
