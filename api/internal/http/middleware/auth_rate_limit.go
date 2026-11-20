package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/http/responses"
)

type window struct {
	count   int
	expires time.Time
}

// AuthRateLimit is bounded process-local protection; proxies are disabled in the router.
func AuthRateLimit() gin.HandlerFunc {
	var mu sync.Mutex
	entries := make(map[string]window)
	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP()
		mu.Lock()
		for k, w := range entries {
			if !now.Before(w.expires) {
				delete(entries, k)
			}
		}
		w, exists := entries[key]
		if !exists && len(entries) >= 4096 || w.count >= 20 {
			mu.Unlock()
			c.Header("Retry-After", "60")
			responses.Abort(c, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts. Wait a minute and try again.")
			return
		}
		if !exists {
			w.expires = now.Add(time.Minute)
		}
		w.count++
		entries[key] = w
		mu.Unlock()
		c.Next()
	}
}
