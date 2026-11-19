package helpers

import (
	"context"
	"errors"
	"testing"
)

type unreadUpload struct{ read bool }

func (r *unreadUpload) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("upload should not be read")
}

func TestBusyConverterDoesNotBufferUpload(t *testing.T) {
	converter := NewWebPConverter(1)
	converter.slots <- struct{}{}
	defer func() { <-converter.slots }()
	source := &unreadUpload{}
	_, _, err := converter.Convert(context.Background(), source, "photo.png")
	var uploadErr *UploadError
	if !errors.As(err, &uploadErr) || source.read {
		t.Fatalf("busy converter read upload or returned wrong error: %v", err)
	}
}
