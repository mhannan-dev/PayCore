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

	"paycore/api/internal/model"
)

type ImageTransformer interface {
	Convert(context.Context, io.Reader, string) ([]byte, string, error)
}

// PreparedUpload contains validated bytes. Its original stream remains caller-owned.
type PreparedUpload struct {
	FileName, ContentType string
	Content               io.Reader
}

func PrepareUpload(ctx context.Context, input model.AttachmentUpload, images ImageTransformer) (PreparedUpload, error) {
	if err := ctx.Err(); err != nil {
		return PreparedUpload{}, err
	}
	name := path.Base(strings.ReplaceAll(input.FileName, "\\", "/"))
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 255 || strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return PreparedUpload{}, &model.UploadError{Code: model.UploadInvalidName}
	}
	if input.Content == nil {
		return PreparedUpload{}, &model.UploadError{Code: model.UploadInvalidFile}
	}
	var prefix [512]byte
	n, err := io.ReadFull(input.Content, prefix[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return PreparedUpload{}, &model.UploadError{Code: model.UploadInvalidFile}
	}
	contentType := http.DetectContentType(prefix[:n])
	var payload io.Reader = io.MultiReader(bytes.NewReader(prefix[:n]), input.Content)
	if imageUpload(prefix[:n], contentType, input.DeclaredContentType, name) {
		if images == nil {
			return PreparedUpload{}, &model.UploadError{Code: model.UploadImageBusy}
		}
		converted, convertedName, err := images.Convert(ctx, payload, name)
		if err != nil {
			return PreparedUpload{}, err
		}
		name = convertedName
		payload = bytes.NewReader(converted)
		contentType = WebPContentType
	}
	if err := ctx.Err(); err != nil {
		return PreparedUpload{}, err
	}
	return PreparedUpload{FileName: name, ContentType: contentType, Content: payload}, nil
}
