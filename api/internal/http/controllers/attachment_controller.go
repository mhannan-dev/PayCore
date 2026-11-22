package controllers

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"paycore/api/internal/http/requests"
	"strconv"

	"github.com/gin-gonic/gin"
	"paycore/api/internal/apperr"
	"paycore/api/internal/http/responses"
	"paycore/api/internal/models"
)

// Attachments is the transport's use-case contract; no storage or SQL is exposed.
type Attachments interface {
	Upload(context.Context, string, models.AttachmentUpload) (models.Attachment, error)
	List(context.Context, string, int, int) ([]models.Attachment, error)
	Metadata(context.Context, string, string) (models.Attachment, error)
	Download(context.Context, string, string) (models.AttachmentDownload, error)
	Delete(context.Context, string, string) error
	Photo(context.Context, string, *string) (models.AttachmentDownload, error)
}
type ProfilePhotos interface {
	Set(context.Context, string, *string) (models.PhotoResult, error)
}
type AttachmentController struct {
	attachments Attachments
	photos      ProfilePhotos
}

func NewAttachmentController(attachments Attachments, photos ProfilePhotos) *AttachmentController {
	return &AttachmentController{attachments: attachments, photos: photos}
}

func (h *AttachmentController) Ready(c *gin.Context) {
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
func (h *AttachmentController) Upload(c *gin.Context) {
	input, cleanup, err := requests.ReadUpload(c.Writer, c.Request)
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
	responses.Created(c, a)
}
func (h *AttachmentController) List(c *gin.Context) {
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
	responses.List(c, items, limit, offset)
}
func (h *AttachmentController) Metadata(c *gin.Context) {
	id, ok := attachmentID(c)
	if !ok {
		return
	}
	a, err := h.attachments.Metadata(c.Request.Context(), sessionUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	responses.OK(c, a)
}
func serveAttachment(c *gin.Context, result models.AttachmentDownload, inline bool) {
	defer func() { _ = result.Content.Close() }() // Read-only: a close error cannot lose data.
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
func (h *AttachmentController) Download(c *gin.Context) {
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
func (h *AttachmentController) Delete(c *gin.Context) {
	id, ok := attachmentID(c)
	if !ok {
		return
	}
	if err := h.attachments.Delete(c.Request.Context(), sessionUser(c).ID, id); err != nil {
		fail(c, err)
		return
	}
	responses.NoContent(c)
}
func (h *AttachmentController) SetPhoto(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var request struct {
		AttachmentID json.RawMessage `json:"attachment_id" binding:"required"`
	}
	if err := requests.BindJSON(c, &request); err != nil {
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
	responses.OK(c, result)
}

func (h *AttachmentController) Photo(c *gin.Context) {
	u := sessionUser(c)
	result, err := h.attachments.Photo(c.Request.Context(), u.ID, u.PhotoAttachmentID)
	if err != nil {
		fail(c, err)
		return
	}
	serveAttachment(c, result, true)
}

// Guard returns the shared authentication and permission middleware provider.
