package routes

import (
	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/controllers"
	"paycore/api/internal/http/middleware"
	"paycore/api/internal/models"
)

// RegisterAttachment registers the attachment endpoints and their middleware.
func RegisterAttachment(api *gin.RouterGroup, h *controllers.AttachmentController, auth *middleware.Auth) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("attachments_unavailable", "File storage is not available right now."))
	files := api.Group("/attachments", fallback, h.Ready)
	files.POST("", auth.Can(models.PermAttachmentsWrite), h.Upload)
	files.GET("", auth.Can(models.PermAttachmentsRead), h.List)
	files.GET("/:id", auth.Can(models.PermAttachmentsRead), h.Metadata)
	files.GET("/:id/content", auth.Can(models.PermAttachmentsRead), h.Download)
	files.DELETE("/:id", auth.Can(models.PermAttachmentsDelete), h.Delete)
	photo := api.Group("/auth", fallback, h.Ready)
	photo.PUT("/photo", auth.Can(models.PermAttachmentsWrite), h.SetPhoto)
	photo.GET("/photo", auth.Can(models.PermAttachmentsRead), h.Photo)
}
