package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"paycore/api/internal/models"
)

type attachmentStoreFake struct {
	attachment       models.Attachment
	createErr        error
	cancel           context.CancelFunc
	created, deleted bool
}

func (s *attachmentStoreFake) Create(_ context.Context, a models.Attachment) (models.Attachment, error) {
	s.created = true
	if s.cancel != nil {
		s.cancel()
	}
	if s.createErr != nil {
		return models.Attachment{}, s.createErr
	}
	a.ID = "id"
	a.StorageKind = models.AttachmentLocal
	s.attachment = a
	return a, nil
}
func (s *attachmentStoreFake) Get(context.Context, string, string) (models.Attachment, error) {
	return s.attachment, nil
}
func (s *attachmentStoreFake) List(context.Context, string, int, int) ([]models.Attachment, error) {
	return []models.Attachment{s.attachment}, nil
}
func (s *attachmentStoreFake) Delete(context.Context, string, string) error {
	s.deleted = true
	return nil
}

type closeTrackingReader struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error { r.closed = true; return nil }

type attachmentStorageFake struct {
	data                   []byte
	wrote, removed, opened bool
	removeErr              error
	cleanupContextValid    bool
	reader                 *closeTrackingReader
	size                   int64
}

func (s *attachmentStorageFake) Write(_ context.Context, r io.Reader, _ string) (models.StoredFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return models.StoredFile{}, err
	}
	s.data = data
	s.wrote = true
	return models.StoredFile{Key: "key", SHA256: "hash", Size: int64(len(data))}, nil
}
func (s *attachmentStorageFake) Remove(ctx context.Context, _ string) error {
	s.removed = true
	deadline, ok := ctx.Deadline()
	s.cleanupContextValid = ctx.Err() == nil && ok && time.Until(deadline) <= 5*time.Second
	return s.removeErr
}
func (s *attachmentStorageFake) Read(context.Context, string) (models.FileContent, error) {
	s.opened = true
	s.reader = &closeTrackingReader{Reader: bytes.NewReader(s.data)}
	return models.FileContent{Reader: s.reader, Size: s.size}, nil
}

func TestAttachmentUploadCleanup(t *testing.T) {
	failed := errors.New("metadata rejected")
	cleanupFailed := errors.New("disk removal failed")
	for _, tc := range []struct {
		name            string
		err, cleanupErr error
		cancel, remove  bool
	}{
		{"success", nil, nil, false, false},
		{"definite failure", failed, nil, false, true},
		{"request canceled", context.Canceled, nil, true, true},
		{"cleanup failure", failed, cleanupFailed, false, true},
		{"uncertain commit", errors.Join(models.ErrAttachmentCommitUnknown, failed), nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &attachmentStoreFake{createErr: tc.err}
			if tc.cancel {
				store.cancel = cancel
			}
			files := &attachmentStorageFake{removeErr: tc.cleanupErr}
			svc := NewAttachmentService(store, files, nil)
			a, err := svc.Upload(ctx, "owner", models.AttachmentUpload{FileName: `C:\files\note.txt`, Content: strings.NewReader("sample text")})
			if tc.err == nil {
				if err != nil || a.FileName != "note.txt" || a.OwnerUserID != "owner" || a.ContentType != "text/plain; charset=utf-8" {
					t.Fatalf("wrong result: %+v %v", a, err)
				}
			} else if !errors.Is(err, tc.err) {
				t.Fatalf("lost metadata error: %v", err)
			}
			if !files.wrote || files.removed != tc.remove {
				t.Fatalf("cleanup=%v, want %v", files.removed, tc.remove)
			}
			if tc.remove && !files.cleanupContextValid {
				t.Fatal("cleanup inherited cancellation or lacked deadline")
			}
			if tc.cleanupErr != nil && !errors.Is(err, tc.cleanupErr) {
				t.Fatal("cleanup failure was lost")
			}
		})
	}
}

func TestAttachmentOwnershipAndContentPolicy(t *testing.T) {
	key := "key"
	for _, tc := range []struct {
		name, owner string
		kind        models.AttachmentStorageKind
		mime        string
		photo       bool
		size        int64
		want        error
		opened      bool
	}{
		{"owned download", "owner", "LOCAL", "text/plain", false, 4, nil, true},
		{"other owner", "intruder", "LOCAL", "text/plain", false, 4, models.ErrAttachmentNotFound, false},
		{"missing caller", "", "LOCAL", "text/plain", false, 4, models.ErrAttachmentNotFound, false},
		{"external", "owner", "EXTERNAL", "image/png", false, 4, ErrExternalAttachment, false},
		{"HTML photo", "owner", "LOCAL", "text/html", true, 4, ErrInvalidStoredPhoto, false},
		{"WebP photo", "owner", "LOCAL", "image/webp", true, 4, nil, true},
		{"size mismatch", "owner", "LOCAL", "text/plain", false, 9, ErrInvalidStoredAttachment, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &attachmentStoreFake{attachment: models.Attachment{ID: "id", OwnerUserID: "owner", StorageKind: tc.kind, StorageKey: &key, ContentType: tc.mime, SizeBytes: 4}}
			files := &attachmentStorageFake{data: []byte("data"), size: tc.size}
			svc := NewAttachmentService(store, files, nil)
			var result models.AttachmentDownload
			var err error
			if tc.photo {
				id := "id"
				result, err = svc.Photo(context.Background(), tc.owner, &id)
			} else {
				result, err = svc.Download(context.Background(), tc.owner, "id")
			}
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v want %v", err, tc.want)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = io.ReadAll(result.Content); err != nil {
					t.Fatal(err)
				}
				_ = result.Content.Close()
			}
			if files.opened != tc.opened {
				t.Fatal("storage opened before ownership/content validation")
			}
			if files.opened && !files.reader.closed {
				t.Fatal("opened content was not closed")
			}
			if errors.Is(tc.want, models.ErrAttachmentNotFound) {
				if err = svc.Delete(context.Background(), tc.owner, "id"); !errors.Is(err, models.ErrAttachmentNotFound) || store.deleted {
					t.Fatal("unauthorized delete reached repository")
				}
			}
		})
	}
}

type failingImageTransformer struct{ called bool }

func (t *failingImageTransformer) Convert(context.Context, io.Reader, string) ([]byte, string, error) {
	t.called = true
	return nil, "", &models.UploadError{Code: models.UploadUnsupportedImage}
}
func TestAttachmentInvalidImageDoesNotWrite(t *testing.T) {
	files := &attachmentStorageFake{}
	store := &attachmentStoreFake{}
	images := &failingImageTransformer{}
	svc := NewAttachmentService(store, files, images)
	_, err := svc.Upload(context.Background(), "owner", models.AttachmentUpload{FileName: "vector.svg", Content: strings.NewReader("<svg></svg>")})
	var invalid *models.UploadError
	if !errors.As(err, &invalid) || invalid.Code != models.UploadUnsupportedImage || !images.called || files.wrote || store.created {
		t.Fatalf("invalid image was stored: %v", err)
	}
	if _, err = svc.Upload(context.Background(), "", models.AttachmentUpload{FileName: "note.txt", Content: strings.NewReader("text")}); !errors.Is(err, models.ErrAttachmentNotFound) || files.wrote {
		t.Fatal("empty owner wrote a file")
	}
}
