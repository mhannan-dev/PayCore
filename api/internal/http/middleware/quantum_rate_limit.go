package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"paycore/api/internal/http/responses"
	"sync"
	"time"
)

// QuantumRateLimit is a bounded per-IP window. Reads allow 3-second status polling;
// writes have a separate tighter budget so polling cannot exhaust payment slots.
func QuantumRateLimit() gin.HandlerFunc {
	var mu sync.Mutex
	entries := map[string]window{}
	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP()
		limit := 120
		if c.Request.Method != "GET" {
			key += ":write"
			limit = 20
		}
		mu.Lock()
		for k, w := range entries {
			if !now.Before(w.expires) {
				delete(entries, k)
			}
		}
		w, exists := entries[key]
		if (!exists && len(entries) >= 4096) || w.count >= limit {
			mu.Unlock()
			c.Header("Retry-After", "60")
			responses.Abort(c, http.StatusTooManyRequests, "rate_limited", "Too many checkout requests. Wait a minute and retry.")
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
