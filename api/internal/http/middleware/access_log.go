package middleware

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog records route templates, not raw URLs, query strings or credentials.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.InfoContext(c.Request.Context(), "request_completed", "request_id", RequestIDFrom(c), "method", c.Request.Method, "route", privateRoute(c), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}

func logRequestError(c *gin.Context, err error, code string, status int) {
	slog.ErrorContext(c.Request.Context(), "request_failed", "request_id", RequestIDFrom(c), "method", c.Request.Method, "route", c.FullPath(), "status", status, "code", code, "error_type", fmt.Sprintf("%T", err))
}

// Route templates are safe even for unknown paths; checkout capabilities are always redacted.
func privateRoute(c *gin.Context) string {
	route := c.FullPath()
	if n := strings.Index(route, "/checkout/"); n >= 0 {
		return route[:n] + "/checkout/[redacted]"
	}
	return route
}
