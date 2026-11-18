package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/pkg/response"
)

// corsMiddleware configures browser access from WEB_ORIGINS and the environment.
func corsMiddleware() gin.HandlerFunc {
	// Bearer-token requests only; never combine wildcard origins with credentials.
	origins := os.Getenv("WEB_ORIGINS")
	// Without an explicit allowlist, development accepts any loopback port so a
	// web dev server that moved off :3000 (e.g. to :3001) can still reach the API.
	allowAnyLoopback := origins == "" && isDevelopment()
	if origins == "" {
		origins = "http://localhost:3000,http://127.0.0.1:3000"
	}
	allowedOrigins := make(map[string]bool)
	for _, origin := range strings.Split(origins, ",") {
		allowedOrigins[strings.TrimSpace(origin)] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		c.Header("Vary", "Origin")
		if origin != "" && !allowedOrigins[origin] && !(allowAnyLoopback && isLoopbackOrigin(origin)) {
			response.Abort(c, http.StatusForbidden, "origin_not_allowed", "This browser origin is not allowed to call the API.")
			return
		}
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, Idempotency-Key")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// registerHealthRoutes keeps liveness independent of database readiness.
func registerHealthRoutes(r *gin.Engine, db DatabasePinger) {
	// Health check endpoint
	r.GET("/", func(c *gin.Context) {
		response.OK(c, gin.H{
			"service": "PayCore API",
			"status":  "running",
		})
	})
	r.GET("/health/live", func(c *gin.Context) {
		response.OK(c, gin.H{"status": "alive"})
	})
	r.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if db == nil || db.Ping(ctx) != nil {
			response.JSON(c, http.StatusServiceUnavailable, gin.H{"status": "not_ready", "database": "unavailable"})
			return
		}
		response.OK(c, gin.H{"status": "ready", "database": "connected"})
	})
}

func isDevelopment() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ENV"))) {
	case "", "development", "local", "test":
		return true
	}
	return false
}

// isLoopbackOrigin reports whether origin is exactly http://{localhost|127.0.0.1|[::1]}[:port].
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
