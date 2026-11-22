// Package apperr carries HTTP-ready errors through the API without coupling the
// inner layers to HTTP. Services and repositories return plain sentinel errors;
// a Table (owned by internal/server) maps them to an *Error, and the error
// middleware writes the standard envelope. Controllers create *Error values directly
// only for request-shape problems (malformed IDs, bad query parameters).
package apperr

import (
	"errors"
	"net/http"
)

// Error is an error with its HTTP status, a stable machine code and a message
// safe to show to clients. Cause, if set, is logged but never sent.
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
	// Header holds response headers to send with the error (e.g. Retry-After).
	Header map[string]string
	Cause  error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// New returns an Error without a cause.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// WithCause returns a copy of e that records cause for logging.
func (e *Error) WithCause(cause error) *Error {
	c := *e
	c.Cause = cause
	return &c
}

// WithHeader returns a copy of e that also sends header name: value.
func (e *Error) WithHeader(name, value string) *Error {
	c := *e
	c.Header = map[string]string{}
	for k, v := range e.Header {
		c.Header[k] = v
	}
	c.Header[name] = value
	return &c
}

// Shorthands for request-shape errors raised by controllers.
func BadRequest(code, message string) *Error { return New(http.StatusBadRequest, code, message) }
func NotFound(code, message string) *Error   { return New(http.StatusNotFound, code, message) }
func Forbidden(code, message string) *Error  { return New(http.StatusForbidden, code, message) }
func Conflict(code, message string) *Error   { return New(http.StatusConflict, code, message) }
func Unavailable(code, message string) *Error {
	return New(http.StatusServiceUnavailable, code, message)
}

// Rule maps errors to an *Error. Exactly one of Target or Match is set:
// Target matches with errors.Is; Match handles typed errors (errors.As) and may
// build the response from the error's fields.
type Rule struct {
	Target error
	Match  func(error) (*Error, bool)
	Error  *Error
}

// Table is an ordered list of rules; the first match wins.
type Table []Rule

// Resolve returns the response for err, or false when no rule matches.
func (t Table) Resolve(err error) (*Error, bool) {
	for _, r := range t {
		if r.Match != nil {
			if e, ok := r.Match(err); ok {
				return e.WithCause(err), true
			}
			continue
		}
		if r.Target != nil && errors.Is(err, r.Target) {
			return r.Error.WithCause(err), true
		}
	}
	return nil, false
}
