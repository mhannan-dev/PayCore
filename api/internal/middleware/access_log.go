package middleware

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessLog records route templates, not raw URLs, query strings or credentials.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.InfoContext(c.Request.Context(), "request_completed", "request_id", RequestIDFrom(c), "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}

func logRequestError(c *gin.Context, err error, code string, status int) {
	slog.ErrorContext(c.Request.Context(), "request_failed", "request_id", RequestIDFrom(c), "method", c.Request.Method, "route", c.FullPath(), "status", status, "code", code, "error_type", fmt.Sprintf("%T", err))
}
