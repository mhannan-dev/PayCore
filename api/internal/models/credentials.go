package models

import "errors"

// ErrCredentialsChanged rejects a login whose verified credentials are stale.
var ErrCredentialsChanged = errors.New("credentials changed before session issuance")
