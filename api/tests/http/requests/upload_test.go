package requests_test

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"paycore/api/internal/http/requests"
	"testing"

	"paycore/api/internal/models"
)

func TestReadUploadShapeAndTemporaryFileCleanup(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			data := bytes.Repeat([]byte{'x'}, (1<<20)+100)
			for i := 0; i < count; i++ {
				part, err := writer.CreateFormFile("file", "file.txt")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = part.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/", &body)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			input, cleanup, err := requests.ReadUpload(httptest.NewRecorder(), r)
			defer cleanup()
			if count != 1 {
				var invalid *models.UploadError
				if !errors.As(err, &invalid) || invalid.Code != models.UploadOneFileRequired {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(input.Content)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatal("parsed stream differs")
			}
			cleanup()
			if file, err := r.MultipartForm.File["file"][0].Open(); err == nil {
				_ = file.Close()
				t.Fatal("multipart temporary file was retained")
			}
		})
	}
}
