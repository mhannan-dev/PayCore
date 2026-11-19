package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"paycore/api/internal/storage"
)

// FileSaver stores a stream and returns its storage key, SHA-256 and byte size.
type FileSaver interface {
	Save(io.Reader, ...string) (string, string, int64, error)
}

type ImageTransformer interface {
	Convert(context.Context, io.Reader, string) ([]byte, string, error)
}

type UploadedFile struct {
	FileName    string
	ContentType string
	StorageKey  string
	SHA256      string
	SizeBytes   int64
}

// UploadError is a safe validation error that can be returned to the client.
type UploadError struct {
	Status int
	Code   string
}

func (e *UploadError) Error() string { return e.Code }

// UploadFile accepts exactly one multipart file in the "file" field. It bounds
// request size, cleans up multipart temporary files and detects MIME from bytes.
// The caller handles authentication and persists the returned metadata.
func UploadFile(w http.ResponseWriter, r *http.Request, files FileSaver, images ImageTransformer) (UploadedFile, error) {
	var result UploadedFile
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxFileSize+64*1024)
	err := r.ParseMultipartForm(1024 * 1024)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return result, &UploadError{http.StatusRequestEntityTooLarge, "file_too_large"}
		}
		return result, &UploadError{http.StatusBadRequest, "invalid_multipart"}
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		return result, &UploadError{http.StatusBadRequest, "one_file_required"}
	}
	header := r.MultipartForm.File["file"][0]
	name := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 255 || strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return result, &UploadError{http.StatusBadRequest, "invalid_file_name"}
	}
	file, err := header.Open()
	if err != nil {
		return result, &UploadError{http.StatusBadRequest, "invalid_file"}
	}
	defer file.Close()
	var prefix [512]byte
	n, err := io.ReadFull(file, prefix[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return result, &UploadError{http.StatusBadRequest, "invalid_file"}
	}
	err = nil
	contentType := http.DetectContentType(prefix[:n])
	var payload io.Reader = io.MultiReader(bytes.NewReader(prefix[:n]), file)
	if imageUpload(prefix[:n], contentType, header.Header.Get("Content-Type"), name) {
		var converted []byte
		if images == nil {
			return result, &UploadError{http.StatusServiceUnavailable, "image_conversion_busy"}
		}
		converted, name, err = images.Convert(r.Context(), payload, name)
		if err == nil {
			payload = bytes.NewReader(converted)
			contentType = WebPContentType
		}
	}
	var key, hash string
	var size int64
	if err == nil {
		key, hash, size, err = files.Save(payload, name)
	}
	if errors.Is(err, storage.ErrFileSize) {
		return result, &UploadError{http.StatusRequestEntityTooLarge, "file_size_must_be_1_byte_to_10_mib"}
	}
	if err != nil {
		return result, err
	}
	return UploadedFile{FileName: name, ContentType: contentType, StorageKey: key, SHA256: hash, SizeBytes: size}, nil
}
