package server

import (
	"github.com/gin-gonic/gin"
)

// Container is the explicit composition-root wrapper for configured controllers.
// It does not resolve dependencies dynamically; cmd/server wires concrete
// repositories and services, then passes the resulting controllers here.
type Container struct {
	Controllers Controllers
}

// NewContainer creates an application container without constructing services.
func NewContainer(controllers Controllers) Container {
	return Container{Controllers: controllers}
}

// Router builds the HTTP engine using the controllers in this container.
func (c Container) Router(db DatabasePinger) *gin.Engine {
	return New(db, c.Controllers)
}
