package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/responses"
)

const fallbackKey = "paycore.error_fallback"

// ErrorFallback sets the response for an explicitly unconfigured dependency in a
// route group. Unexpected errors still become 500 internal_error.
func ErrorFallback(fallback *apperr.Error) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(fallbackKey, fallback)
		c.Next()
	}
}

// Errors is the single place where errors become HTTP responses. Controllers call
// c.Error(err) and return; after the controller chain this middleware writes the
// standard envelope for the last error:
//
//  1. an *apperr.Error is written as is;
//  2. otherwise the first matching rule in table is used;
//  3. ErrNotConfigured uses the route group's fallback; unknown errors become 500.
//
// Unexpected errors (3) and 5xx responses are logged with the request ID; their
// details are never sent to the client.
func Errors(table apperr.Table) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err
		if c.Writer.Written() {
			logRequestError(c, err, "response_already_written", c.Writer.Status())
			return
		}

		var out *apperr.Error
		var known *apperr.Error
		switch {
		case errors.As(err, &known):
			out = known
		default:
			if mapped, ok := table.Resolve(err); ok {
				out = mapped
			} else if fallback, ok := c.Get(fallbackKey); ok && errors.Is(err, apperr.ErrNotConfigured) {
				if f, ok := fallback.(*apperr.Error); ok {
					out = f.WithCause(err)
				}
			}
			if out == nil {
				out = apperr.New(http.StatusInternalServerError, "internal_error", "").WithCause(err)
			}
		}
		if out.Status >= 500 {
			logRequestError(c, err, out.Code, out.Status)
		}
		for name, value := range out.Header {
			c.Header(name, value)
		}
		if out.Details != nil {
			responses.ErrorWithDetails(c, out.Status, out.Code, out.Message, out.Details)
		} else {
			responses.Error(c, out.Status, out.Code, out.Message)
		}
	}
}
