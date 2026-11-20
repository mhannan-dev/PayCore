package handler

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

// Attachments is the transport's use-case contract; no storage or SQL is exposed.
type Attachments interface {
	Upload(context.Context, string, model.AttachmentUpload) (model.Attachment, error)
	List(context.Context, string, int, int) ([]model.Attachment, error)
	Metadata(context.Context, string, string) (model.Attachment, error)
	Download(context.Context, string, string) (model.AttachmentDownload, error)
	Delete(context.Context, string, string) error
	Photo(context.Context, string, *string) (model.AttachmentDownload, error)
}
type ProfilePhotos interface {
	Set(context.Context, string, *string) (model.PhotoResult, error)
}
type AttachmentHandler struct {
	guard       *Guard
	attachments Attachments
	photos      ProfilePhotos
}

func NewAttachmentHandler(guard *Guard, attachments Attachments, photos ProfilePhotos) *AttachmentHandler {
	return &AttachmentHandler{guard: guard, attachments: attachments, photos: photos}
}
func (h *AttachmentHandler) Register(api *gin.RouterGroup) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("attachments_unavailable", "File storage is not available right now."))
	files := api.Group("/attachments", fallback, h.ready)
	files.POST("", h.guard.Require(model.PermAttachmentsWrite), h.Upload)
	files.GET("", h.guard.Require(model.PermAttachmentsRead), h.List)
	files.GET("/:id", h.guard.Require(model.PermAttachmentsRead), h.Metadata)
	files.GET("/:id/content", h.guard.Require(model.PermAttachmentsRead), h.Download)
	files.DELETE("/:id", h.guard.Require(model.PermAttachmentsDelete), h.Delete)
	photo := api.Group("/auth", fallback, h.ready)
	photo.PUT("/photo", h.guard.Require(model.PermAttachmentsWrite), h.SetPhoto)
	photo.GET("/photo", h.guard.Require(model.PermAttachmentsRead), h.Photo)
}
func (h *AttachmentHandler) ready(c *gin.Context) {
	if h.attachments == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}

func validAttachmentID(id string) bool { return validUUID(id) }

var errAttachmentID = apperr.BadRequest("invalid_attachment_id", "Attachment ID must be a UUID.")

func attachmentID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !validAttachmentID(id) {
		fail(c, errAttachmentID)
		return "", false
	}
	return id, true
}
func (h *AttachmentHandler) Upload(c *gin.Context) {
	input, cleanup, err := readUpload(c.Writer, c.Request)
	defer cleanup()
	if err != nil {
		fail(c, err)
		return
	}
	a, err := h.attachments.Upload(c.Request.Context(), sessionUser(c).ID, input)
	if err != nil {
		fail(c, err)
		return
	}
	response.Created(c, a)
}
func (h *AttachmentHandler) List(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		fail(c, apperr.BadRequest("invalid_limit", "limit must be a whole number from 1 to 100."))
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 10000 {
		fail(c, apperr.BadRequest("invalid_offset", "offset must be a whole number from 0 to 10000."))
		return
	}
	items, err := h.attachments.List(c.Request.Context(), sessionUser(c).ID, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	response.List(c, items, limit, offset)
}
func (h *AttachmentHandler) Metadata(c *gin.Context) {
	id, ok := attachmentID(c)
	if !ok {
		return
	}
	a, err := h.attachments.Metadata(c.Request.Context(), sessionUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, a)
}
func serveAttachment(c *gin.Context, result model.AttachmentDownload, inline bool) {
	defer result.Content.Close()
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	a := result.Attachment
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.FileName}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.DataFromReader(http.StatusOK, a.SizeBytes, a.ContentType, result.Content, nil)
}
func (h *AttachmentHandler) Download(c *gin.Context) {
	id, ok := attachmentID(c)
	if !ok {
		return
	}
	result, err := h.attachments.Download(c.Request.Context(), sessionUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	serveAttachment(c, result, false)
}
func (h *AttachmentHandler) Delete(c *gin.Context) {
	id, ok := attachmentID(c)
	if !ok {
		return
	}
	if err := h.attachments.Delete(c.Request.Context(), sessionUser(c).ID, id); err != nil {
		fail(c, err)
		return
	}
	response.NoContent(c)
}
func (h *AttachmentHandler) SetPhoto(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var request struct {
		AttachmentID json.RawMessage `json:"attachment_id" binding:"required"`
	}
	if err := bindJSON(c, &request); err != nil {
		fail(c, apperr.BadRequest("attachment_id_required", "Send JSON with attachment_id (a UUID, or null to remove the photo)."))
		return
	}
	var id *string
	if string(request.AttachmentID) != "null" {
		var value string
		if json.Unmarshal(request.AttachmentID, &value) != nil {
			fail(c, errAttachmentID)
			return
		}
		if !validAttachmentID(value) {
			fail(c, errAttachmentID)
			return
		}
		id = &value
	}
	if h.photos == nil {
		fail(c, errNotConfigured)
		return
	}
	result, err := h.photos.Set(c.Request.Context(), sessionUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *AttachmentHandler) Photo(c *gin.Context) {
	u := sessionUser(c)
	result, err := h.attachments.Photo(c.Request.Context(), u.ID, u.PhotoAttachmentID)
	if err != nil {
		fail(c, err)
		return
	}
	serveAttachment(c, result, true)
}
