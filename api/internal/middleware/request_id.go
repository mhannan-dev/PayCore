package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader carries the request ID in both directions.
	RequestIDHeader = "X-Request-ID"
	requestIDKey    = "paycore.request_id"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// RequestID gives every request an ID, reusing a well-formed incoming
// X-Request-ID (e.g. from a proxy) and otherwise generating one. The ID is
// returned in the response header and included in error logs.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if !validRequestID.MatchString(id) {
			var b [12]byte
			if _, err := rand.Read(b[:]); err == nil {
				id = hex.EncodeToString(b[:])
			} else {
				id = "unknown"
			}
		}
		c.Set(requestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// RequestIDFrom returns the current request's ID ("" outside RequestID).
func RequestIDFrom(c *gin.Context) string {
	id, _ := c.Get(requestIDKey)
	s, _ := id.(string)
	return s
}
