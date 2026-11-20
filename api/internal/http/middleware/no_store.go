package middleware

import "github.com/gin-gonic/gin"

// NoStore marks responses as private and uncacheable (auth and token endpoints).
func NoStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Next()
}
