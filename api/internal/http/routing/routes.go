// Package routing provides small declarative helpers over Gin route groups.
package routing

import "github.com/gin-gonic/gin"

// Route describes one endpoint registered under a Gin route group.
type Route struct {
	Method     string
	Path       string
	Middleware []gin.HandlerFunc
	Handler    gin.HandlerFunc
}

// Group wraps a Gin router group so resource routes share one registration API.
type Group struct {
	*gin.RouterGroup
}

// NewGroup creates a named route group with shared middleware.
func NewGroup(parent *gin.RouterGroup, prefix string, middleware ...gin.HandlerFunc) *Group {
	return &Group{RouterGroup: parent.Group(prefix, middleware...)}
}

// Register adds a list of declarative routes.
func (g *Group) Register(routes ...Route) {
	for _, route := range routes {
		handlers := make([]gin.HandlerFunc, 0, len(route.Middleware)+1)
		handlers = append(handlers, route.Middleware...)
		handlers = append(handlers, route.Handler)
		g.Handle(route.Method, route.Path, handlers...)
	}
}
