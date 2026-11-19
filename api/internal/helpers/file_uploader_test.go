package helpers_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"paycore/api/internal/helpers"
	"paycore/api/internal/storage"
)

func TestUploadFile(t *testing.T) {
	for _, tc := range []struct {
		name, filename, content, errorCode string
		count                              int
	}{
		{"content sniffing and safe filename", `C:\photos\note.png`, strings.Repeat("plain text content ", 100), "", 1},
		{"empty file", "empty.txt", "", "file_size_must_be_1_byte_to_10_mib", 1},
		{"multiple files", "note.txt", "text", "one_file_required", 2},
		{"invalid filename", strings.Repeat("x", 256), "text", "invalid_file_name", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, err := storage.OpenLocal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer files.Close()
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			for i := 0; i < tc.count; i++ {
				part, err := writer.CreateFormFile("file", tc.filename)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = io.WriteString(part, tc.content); err != nil {
					t.Fatal(err)
				}
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/upload", &body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			result, err := helpers.UploadFile(httptest.NewRecorder(), request, files, helpers.NewWebPConverter(2))
			if tc.errorCode != "" {
				var validation *helpers.UploadError
				if !errors.As(err, &validation) || validation.Code != tc.errorCode {
					t.Fatalf("got %v, want %s", err, tc.errorCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(tc.content))
			if !strings.HasSuffix(result.StorageKey, ".png") {
				t.Fatal("stored upload is missing its original file extension")
			}
			if result.FileName != "note.png" || result.ContentType != "text/plain; charset=utf-8" || result.SizeBytes != int64(len(tc.content)) || result.SHA256 != hex.EncodeToString(digest[:]) {
				t.Fatalf("unexpected upload metadata: %+v", result)
			}
			file, err := files.Open(result.StorageKey)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			stored, err := io.ReadAll(file)
			if err != nil || string(stored) != tc.content {
				t.Fatalf("stored content differs: %v", err)
			}
		})
	}
}
