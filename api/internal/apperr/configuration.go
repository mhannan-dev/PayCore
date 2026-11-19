package apperr

import "errors"

// ErrNotConfigured selects an explicitly configured route-area 503 response.
// Other unexpected failures must not be mislabeled as temporary unavailability.
var ErrNotConfigured = errors.New("handler dependencies are not configured")
