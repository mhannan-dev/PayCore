package server

import "github.com/gin-gonic/gin"

// MiddlewareStack registers global HTTP middleware in application order.
type MiddlewareStack struct {
	handlers []gin.HandlerFunc
}

// NewMiddlewareStack creates an ordered middleware stack.
func NewMiddlewareStack(handlers ...gin.HandlerFunc) *MiddlewareStack {
	return &MiddlewareStack{handlers: append([]gin.HandlerFunc(nil), handlers...)}
}

// Use appends middleware to the stack.
func (s *MiddlewareStack) Use(handlers ...gin.HandlerFunc) {
	if s == nil {
		return
	}
	s.handlers = append(s.handlers, handlers...)
}

// Apply adds every middleware in order to a Gin engine.
func (s *MiddlewareStack) Apply(engine *gin.Engine) {
	if s == nil || engine == nil {
		return
	}
	engine.Use(s.handlers...)
}
