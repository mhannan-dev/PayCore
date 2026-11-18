package handler

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"paycore/api/internal/helpers"
	"paycore/api/internal/model"
	"paycore/api/pkg/response"
)

type AttachmentStore interface {
	Create(context.Context, model.Attachment) (model.Attachment, error)
	Get(context.Context, string, string) (model.Attachment, error)
	List(context.Context, string, int, int) ([]model.Attachment, error)
	Delete(context.Context, string, string) error
	SetPhoto(context.Context, string, *string) error
}
type SessionVerifier interface {
	CurrentUser(context.Context, string) (model.UserDTO, error)
}
type AttachmentFiles interface {
	Save(io.Reader) (string, string, int64, error)
	Open(string) (*os.File, error)
}
type AttachmentHandler struct {
	auth  SessionVerifier
	repo  AttachmentStore
	files AttachmentFiles
}

func NewAttachmentHandler(auth SessionVerifier, repo AttachmentStore, files AttachmentFiles) *AttachmentHandler {
	return &AttachmentHandler{auth: auth, repo: repo, files: files}
}
func (h *AttachmentHandler) Register(api *gin.RouterGroup) {
	files := api.Group("/attachments", h.authenticate)
	files.POST("", h.Upload)
	files.GET("", h.List)
	files.GET("/:id", h.Metadata)
	files.GET("/:id/content", h.Download)
	files.DELETE("/:id", h.Delete)
	photo := api.Group("/auth", h.authenticate)
	photo.PUT("/photo", h.SetPhoto)
	photo.GET("/photo", h.Photo)
}
func (h *AttachmentHandler) authenticate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.auth == nil || h.repo == nil || h.files == nil {
		response.Abort(c, http.StatusServiceUnavailable, "attachments_unavailable", "File storage is not available right now.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	u, err := h.auth.CurrentUser(ctx, bearer(c))
	if err != nil {
		authError(c, err)
		c.Abort()
		return
	}
	authenticatedUser.set(c, u)
	c.Next()
}

// attachmentUser is the user set by authenticate. Without it (a wiring bug) it
// returns the zero user, whose empty ID matches no attachment owner: fail closed.
func attachmentUser(c *gin.Context) model.UserDTO {
	u, _ := authenticatedUser.get(c)
	return u
}
func validAttachmentID(id string) bool {
	var value pgtype.UUID
	return value.Scan(id) == nil && value.Valid
}

// uploadMessages gives a readable message for each helpers.UploadError code.
var uploadMessages = map[string]string{
	"file_too_large":                     "The request is too large.",
	"invalid_multipart":                  "Send the file as multipart/form-data.",
	"one_file_required":                  "Send exactly one file in the \"file\" field.",
	"invalid_file_name":                  "The file name is missing or not allowed.",
	"invalid_file":                       "The file could not be read.",
	"file_size_must_be_1_byte_to_10_mib": "The file must be between 1 byte and 10 MiB.",
}

func attachmentError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		response.NotFound(c, "attachment_not_found", "Attachment not found.")
		return
	}
	response.ServiceUnavailable(c, "attachments_unavailable", "File storage is not available right now.")
}
func (h *AttachmentHandler) get(c *gin.Context, id string) (model.Attachment, bool) {
	if !validAttachmentID(id) {
		response.BadRequest(c, "invalid_attachment_id", "Attachment ID must be a UUID.")
		return model.Attachment{}, false
	}
	a, err := h.repo.Get(c.Request.Context(), attachmentUser(c).ID, id)
	if err != nil {
		attachmentError(c, err)
		return model.Attachment{}, false
	}
	return a, true
}
func (h *AttachmentHandler) Upload(c *gin.Context) {
	file, err := helpers.UploadFile(c.Writer, c.Request, h.files)
	if err != nil {
		var validation *helpers.UploadError
		if errors.As(err, &validation) {
			response.Error(c, validation.Status, validation.Code, uploadMessages[validation.Code])
		} else {
			attachmentError(c, err)
		}
		return
	}
	a, err := h.repo.Create(c.Request.Context(), model.Attachment{OwnerUserID: attachmentUser(c).ID, FileName: file.FileName, ContentType: file.ContentType, SizeBytes: file.SizeBytes, SHA256: &file.SHA256, StorageKey: &file.StorageKey})
	if err != nil {
		// Keep the file on an ambiguous DB error: deleting could break a commit
		// whose acknowledgement was lost. Reconcile unreferenced storage keys later.
		attachmentError(c, err)
		return
	}
	response.Created(c, a)
}
func (h *AttachmentHandler) List(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		response.BadRequest(c, "invalid_limit", "limit must be a whole number from 1 to 100.")
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 || offset > 10000 {
		response.BadRequest(c, "invalid_offset", "offset must be a whole number from 0 to 10000.")
		return
	}
	items, err := h.repo.List(c.Request.Context(), attachmentUser(c).ID, limit, offset)
	if err != nil {
		attachmentError(c, err)
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
		response.Conflict(c, "legacy_external_file_not_stored_locally", "This file is stored externally and cannot be served from here.")
		return
	}
	file, err := h.files.Open(*a.StorageKey)
	if err != nil {
		attachmentError(c, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != a.SizeBytes {
		attachmentError(c, errors.New("invalid stored file"))
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
		response.BadRequest(c, "invalid_attachment_id", "Attachment ID must be a UUID.")
		return
	}
	if err := h.repo.Delete(c.Request.Context(), attachmentUser(c).ID, id); err != nil {
		attachmentError(c, err)
		return
	}
	response.NoContent(c)
}
func (h *AttachmentHandler) SetPhoto(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var request struct {
		AttachmentID json.RawMessage `json:"attachment_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "attachment_id_required", "Send JSON with attachment_id (a UUID, or null to remove the photo).")
		return
	}
	var id *string
	if string(request.AttachmentID) != "null" {
		var value string
		if json.Unmarshal(request.AttachmentID, &value) != nil {
			response.BadRequest(c, "invalid_attachment_id", "Attachment ID must be a UUID.")
			return
		}
		a, ok := h.get(c, value)
		if !ok {
			return
		}
		if a.StorageKind != "LOCAL" || a.StorageKey == nil {
			response.UnsupportedMediaType(c, "photo_must_be_local_raster_image", "The profile photo must be an uploaded JPEG, PNG or GIF image.")
			return
		}
		file, err := h.files.Open(*a.StorageKey)
		if err != nil {
			attachmentError(c, err)
			return
		}
		cfg, format, err := image.DecodeConfig(file)
		file.Close()
		if err != nil || (format != "jpeg" && format != "png" && format != "gif") || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 10000 || cfg.Height > 10000 || int64(cfg.Width)*int64(cfg.Height) > 25000000 {
			response.UnsupportedMediaType(c, "photo_must_be_jpeg_png_or_gif_max_25_megapixels", "The profile photo must be a JPEG, PNG or GIF of at most 25 megapixels (10000 px per side).")
			return
		}
		id = &value
	}
	if err := h.repo.SetPhoto(c.Request.Context(), attachmentUser(c).ID, id); err != nil {
		attachmentError(c, err)
		return
	}
	var photo *string
	if id != nil {
		url := "/api/v1/auth/photo"
		photo = &url
	}
	response.OK(c, gin.H{"photo_attachment_id": id, "photo": photo})
}
func (h *AttachmentHandler) Photo(c *gin.Context) {
	u := attachmentUser(c)
	if u.PhotoAttachmentID == nil {
		attachmentError(c, pgx.ErrNoRows)
		return
	}
	a, ok := h.get(c, *u.PhotoAttachmentID)
	if !ok {
		return
	}
	if a.StorageKind == "LOCAL" && a.ContentType != "image/jpeg" && a.ContentType != "image/png" && a.ContentType != "image/gif" {
		response.UnsupportedMediaType(c, "invalid_photo_type", "The stored profile photo is not a JPEG, PNG or GIF image.")
		return
	}
	h.serve(c, a, true)
}
