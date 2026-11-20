// Package responses is the single place that writes HTTP responses for the API.
//
// Success bodies are the resource itself (no wrapper), so existing clients keep
// working. Every error uses one envelope:
//
//	{"error": "machine_readable_code", "message": "Human-readable text.", "details": {...}}
//
// "error" is stable and meant for programs; "message" is for people and may
// change; "details" is optional extra data (for example, field-level problems).
package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorBody is the JSON error envelope returned by every failing endpoint.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// ListBody is the envelope for offset-paginated collections.
type ListBody[T any] struct {
	Items  []T `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// PageBody is ListBody plus the total number of matching items, for page navigation.
type PageBody[T any] struct {
	Items  []T `json:"items"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// --- Success -----------------------------------------------------------------

// JSON writes v with the given status. Prefer the named helpers below.
func JSON(c *gin.Context, status int, v any) { c.JSON(status, v) }

// OK writes 200 with v as the body.
func OK(c *gin.Context, v any) { JSON(c, http.StatusOK, v) }

// Created writes 201 with the new resource as the body.
func Created(c *gin.Context, v any) { JSON(c, http.StatusCreated, v) }

// NoContent writes 204 with no body.
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// List writes 200 with an items/limit/offset envelope. A nil slice is sent as [].
func List[T any](c *gin.Context, items []T, limit, offset int) {
	if items == nil {
		items = []T{}
	}
	OK(c, ListBody[T]{Items: items, Limit: limit, Offset: offset})
}

// Page writes a PageBody; nil items become [].
func Page[T any](c *gin.Context, items []T, total, limit, offset int) {
	if items == nil {
		items = []T{}
	}
	OK(c, PageBody[T]{Items: items, Limit: limit, Offset: offset, Total: total})
}

// --- Errors ------------------------------------------------------------------

// Error writes the error envelope. An empty message falls back to the standard
// HTTP status text, so clients always receive something readable.
func Error(c *gin.Context, status int, code, message string) {
	JSON(c, status, newError(status, code, message, nil))
}

// ErrorWithDetails writes the error envelope with extra structured details.
func ErrorWithDetails(c *gin.Context, status int, code, message string, details any) {
	JSON(c, status, newError(status, code, message, details))
}

// Abort writes the error envelope and stops the remaining controllers (for middleware).
func Abort(c *gin.Context, status int, code, message string) {
	c.Abort()
	Error(c, status, code, message)
}

func newError(status int, code, message string, details any) ErrorBody {
	if code == "" {
		code = defaultCode(status)
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return ErrorBody{Error: code, Message: message, Details: details}
}

func defaultCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusUnsupportedMediaType:
		return "unsupported_media_type"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "error"
	}
}

// Shorthands for the statuses the API uses. code may be "" to use the default.

func BadRequest(c *gin.Context, code, message string) { Error(c, http.StatusBadRequest, code, message) }
func Unauthorized(c *gin.Context, code, message string) {
	Error(c, http.StatusUnauthorized, code, message)
}
func Forbidden(c *gin.Context, code, message string) { Error(c, http.StatusForbidden, code, message) }
func NotFound(c *gin.Context, code, message string)  { Error(c, http.StatusNotFound, code, message) }
func Conflict(c *gin.Context, code, message string)  { Error(c, http.StatusConflict, code, message) }
func UnsupportedMediaType(c *gin.Context, code, message string) {
	Error(c, http.StatusUnsupportedMediaType, code, message)
}
func ServiceUnavailable(c *gin.Context, code, message string) {
	Error(c, http.StatusServiceUnavailable, code, message)
}
func InternalError(c *gin.Context) {
	Error(c, http.StatusInternalServerError, "internal_error", "Something went wrong. Try again later.")
}

// --- Router-level controllers ---------------------------------------------------

// RouteNotFound is for gin's NoRoute: unknown paths get the JSON envelope
// instead of gin's plain-text "404 page not found".
func RouteNotFound(c *gin.Context) {
	Error(c, http.StatusNotFound, "route_not_found", "No API route matches "+c.Request.Method+" "+c.Request.URL.Path+".")
}

// MethodNotAllowed is for gin's NoMethod (enable engine.HandleMethodNotAllowed).
func MethodNotAllowed(c *gin.Context) {
	Error(c, http.StatusMethodNotAllowed, "method_not_allowed", c.Request.Method+" is not supported for "+c.Request.URL.Path+".")
}
