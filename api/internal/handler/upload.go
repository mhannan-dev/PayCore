package handler

import (
	"errors"
	"net/http"

	"paycore/api/internal/model"
	"paycore/api/internal/storage"
)

// readUpload bounds and parses HTTP input only. The returned cleanup closes the
// opened part and removes multipart temporary files on every response path.
func readUpload(w http.ResponseWriter, r *http.Request) (model.AttachmentUpload, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxFileSize+64*1024)
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		cleanup()
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return model.AttachmentUpload{}, func() {}, &model.UploadError{Code: model.UploadTooLarge}
		}
		return model.AttachmentUpload{}, func() {}, &model.UploadError{Code: model.UploadInvalidMultipart}
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		cleanup()
		return model.AttachmentUpload{}, func() {}, &model.UploadError{Code: model.UploadOneFileRequired}
	}
	header := r.MultipartForm.File["file"][0]
	file, err := header.Open()
	if err != nil {
		cleanup()
		return model.AttachmentUpload{}, func() {}, &model.UploadError{Code: model.UploadInvalidFile}
	}
	return model.AttachmentUpload{FileName: header.Filename, DeclaredContentType: header.Header.Get("Content-Type"), Content: file}, func() { _ = file.Close(); cleanup() }, nil
}
