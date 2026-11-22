package requests

import (
	"errors"
	"net/http"

	"paycore/api/internal/models"
	"paycore/api/internal/storage"
)

// ReadUpload bounds and parses HTTP input only. The returned cleanup closes the
// opened part and removes multipart temporary files on every response path.
func ReadUpload(w http.ResponseWriter, r *http.Request) (models.AttachmentUpload, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxFileSize+64*1024)
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	//nolint:gosec // G120: r.Body is bounded by http.MaxBytesReader at the top of this function.
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		cleanup()
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return models.AttachmentUpload{}, func() {}, &models.UploadError{Code: models.UploadTooLarge}
		}
		return models.AttachmentUpload{}, func() {}, &models.UploadError{Code: models.UploadInvalidMultipart}
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		cleanup()
		return models.AttachmentUpload{}, func() {}, &models.UploadError{Code: models.UploadOneFileRequired}
	}
	header := r.MultipartForm.File["file"][0]
	file, err := header.Open()
	if err != nil {
		cleanup()
		return models.AttachmentUpload{}, func() {}, &models.UploadError{Code: models.UploadInvalidFile}
	}
	return models.AttachmentUpload{FileName: header.Filename, DeclaredContentType: header.Header.Get("Content-Type"), Content: file}, func() { _ = file.Close(); cleanup() }, nil
}
