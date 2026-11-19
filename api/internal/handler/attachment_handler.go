package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"

	"paycore/api/internal/apperr"
	"paycore/api/internal/helpers"
	"paycore/api/internal/middleware"
	"paycore/api/internal/model"
	"paycore/api/internal/repository"
	"paycore/api/pkg/response"

	"github.com/gin-gonic/gin"
)

type AttachmentStore interface {
	Create(context.Context, model.Attachment) (model.Attachment, error)
	Get(context.Context, string, string) (model.Attachment, error)
	List(context.Context, string, int, int) ([]model.Attachment, error)
	Delete(context.Context, string, string) error
	SetPhoto(context.Context, string, *string) error
}
type AttachmentFiles interface {
	Save(io.Reader, ...string) (string, string, int64, error)
	Open(string) (*os.File, error)
}
type AttachmentHandler struct {
	guard  *Guard
	repo   AttachmentStore
	files  AttachmentFiles
	photos ProfilePhotos
	images helpers.ImageTransformer
}

type ProfilePhotos interface {
	Set(context.Context, string, *string) (model.PhotoResult, error)
}

// NewAttachmentHandler: nil dependencies make the routes answer 503 attachments_unavailable.
func NewAttachmentHandler(guard *Guard, repo AttachmentStore, files AttachmentFiles, photos ProfilePhotos, images helpers.ImageTransformer) *AttachmentHandler {
	return &AttachmentHandler{guard: guard, repo: repo, files: files, photos: photos, images: images}
}
func (h *AttachmentHandler) Register(api *gin.RouterGroup) {
	fallback := middleware.ErrorFallback(apperr.Unavailable("attachments_unavailable", "File storage is not available right now."))
	files := api.Group("/attachments", fallback, h.ready, h.guard.Session())
	files.POST("", h.Upload)
	files.GET("", h.List)
	files.GET("/:id", h.Metadata)
	files.GET("/:id/content", h.Download)
	files.DELETE("/:id", h.Delete)
	photo := api.Group("/auth", fallback, h.ready, h.guard.Session())
	photo.PUT("/photo", h.SetPhoto)
	photo.GET("/photo", h.Photo)
}

// ready stops requests when storage is not configured.
func (h *AttachmentHandler) ready(c *gin.Context) {
	if h.repo == nil || h.files == nil {
		fail(c, errNotConfigured)
		return
	}
	c.Next()
}

// attachmentUser is the user set by the Guard. Without it (a wiring bug) it
// returns the zero user, whose empty ID matches no attachment owner: fail closed.
func attachmentUser(c *gin.Context) model.UserDTO {
	u, _ := authenticatedUser.get(c)
	return u
}
func validAttachmentID(id string) bool { return validUUID(id) }

// uploadMessages gives a readable message for each helpers.UploadError code.
var uploadMessages = map[string]string{
	"image_conversion_busy":              "Image conversion is busy. Try again shortly.",
	"file_too_large":                     "The request is too large.",
	"invalid_multipart":                  "Send the file as multipart/form-data.",
	"one_file_required":                  "Send exactly one file in the \"file\" field.",
	"invalid_file_name":                  "The file name is missing or not allowed.",
	"invalid_file":                       "The file could not be read.",
	"file_size_must_be_1_byte_to_10_mib": "The file must be between 1 byte and 10 MiB.",
	"unsupported_image":                  "Use a JPEG, PNG, GIF, WebP, BMP or TIFF image. Other image formats cannot be converted.",
	"invalid_image":                      "The image is corrupt or could not be decoded.",
	"unsupported_animated_image":         "This animated image format is not supported. Use an animated GIF or a still image.",
	"image_dimensions_exceeded":          "Images must fit within 10000 pixels per side and 25 megapixels; GIFs allow up to 200 frames and 25 million total frame pixels.",
}

var errAttachmentID = apperr.BadRequest("invalid_attachment_id", "Attachment ID must be a UUID.")

func (h *AttachmentHandler) get(c *gin.Context, id string) (model.Attachment, bool) {
	if !validAttachmentID(id) {
		fail(c, errAttachmentID)
		return model.Attachment{}, false
	}
	a, err := h.repo.Get(c.Request.Context(), attachmentUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return model.Attachment{}, false
	}
	return a, true
}
func (h *AttachmentHandler) Upload(c *gin.Context) {
	file, err := helpers.UploadFile(c.Writer, c.Request, h.files, h.images)
	if err != nil {
		var validation *helpers.UploadError
		if errors.As(err, &validation) {
			fail(c, apperr.New(validation.Status, validation.Code, uploadMessages[validation.Code]))
		} else {
			fail(c, err)
		}
		return
	}
	a, err := h.repo.Create(c.Request.Context(), model.Attachment{OwnerUserID: attachmentUser(c).ID, FileName: file.FileName, ContentType: file.ContentType, SizeBytes: file.SizeBytes, SHA256: &file.SHA256, StorageKey: &file.StorageKey})
	if err != nil {
		// Keep the file on an ambiguous DB error: deleting could break a commit
		// whose acknowledgement was lost. Reconcile unreferenced storage keys later.
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
	items, err := h.repo.List(c.Request.Context(), attachmentUser(c).ID, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	response.List(c, items, limit, offset)
}
func (h *AttachmentHandler) Metadata(c *gin.Context) {
	if a, ok := h.get(c, c.Param("id")); ok {
		response.OK(c, a)
	}
}
func (h *AttachmentHandler) serve(c *gin.Context, a model.Attachment, inline bool) {
	if a.StorageKind != "LOCAL" || a.StorageKey == nil {
		fail(c, apperr.New(http.StatusConflict, "legacy_external_file_not_stored_locally", "This file is stored externally and cannot be served from here."))
		return
	}
	file, err := h.files.Open(*a.StorageKey)
	if err != nil {
		fail(c, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != a.SizeBytes {
		fail(c, errors.New("invalid stored file"))
		return
	}
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.FileName}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.DataFromReader(http.StatusOK, a.SizeBytes, a.ContentType, file, nil)
}
func (h *AttachmentHandler) Download(c *gin.Context) {
	if a, ok := h.get(c, c.Param("id")); ok {
		h.serve(c, a, false)
	}
}
func (h *AttachmentHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if !validAttachmentID(id) {
		fail(c, errAttachmentID)
		return
	}
	if err := h.repo.Delete(c.Request.Context(), attachmentUser(c).ID, id); err != nil {
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
	result, err := h.photos.Set(c.Request.Context(), attachmentUser(c).ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	response.OK(c, result)
}
func (h *AttachmentHandler) Photo(c *gin.Context) {
	u := attachmentUser(c)
	if u.PhotoAttachmentID == nil {
		fail(c, repository.ErrAttachmentNotFound)
		return
	}
	a, ok := h.get(c, *u.PhotoAttachmentID)
	if !ok {
		return
	}
	if a.StorageKind == "LOCAL" && a.ContentType != "image/jpeg" && a.ContentType != "image/png" && a.ContentType != "image/gif" && a.ContentType != helpers.WebPContentType {
		fail(c, apperr.New(http.StatusUnsupportedMediaType, "invalid_photo_type", "The stored profile photo is not a WebP, JPEG, PNG or GIF image."))
		return
	}
	h.serve(c, a, true)
}
