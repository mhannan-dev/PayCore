package helpers_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"paycore/api/internal/helpers"
	"paycore/api/internal/models"
)

func TestPrepareUpload(t *testing.T) {
	for _, tc := range []struct {
		name, filename, content string
		code                    models.UploadCode
	}{
		{"content sniffing and safe filename", "C:\\photos\\note.png", strings.Repeat("plain text content ", 100), ""},
		{"invalid filename", strings.Repeat("x", 256), "text", models.UploadInvalidName},
		{"control character", "note\x00.txt", "text", models.UploadInvalidName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := helpers.PrepareUpload(context.Background(), models.AttachmentUpload{FileName: tc.filename, Content: bytes.NewBufferString(tc.content)}, helpers.NewWebPConverter(2))
			if tc.code != "" {
				var invalid *models.UploadError
				if !errors.As(err, &invalid) || invalid.Code != tc.code {
					t.Fatalf("got %v want %s", err, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(result.Content)
			if err != nil || string(data) != tc.content || result.FileName != "note.png" || result.ContentType != "text/plain; charset=utf-8" {
				t.Fatalf("wrong prepared upload: %+v %v", result, err)
			}
		})
	}
}
