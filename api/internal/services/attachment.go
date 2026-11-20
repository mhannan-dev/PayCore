package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"paycore/api/internal/helpers"
	"paycore/api/internal/models"
	"paycore/api/internal/storage"
)

type AttachmentStore interface {
	// Create guarantees that ordinary errors did not commit. An uncertain commit
	// must wrap models.ErrAttachmentCommitUnknown so file cleanup stays safe.
	Create(context.Context, models.Attachment) (models.Attachment, error)
	Get(context.Context, string, string) (models.Attachment, error)
	List(context.Context, string, int, int) ([]models.Attachment, error)
	Delete(context.Context, string, string) error
}
type AttachmentStorage interface {
	Write(context.Context, io.Reader, string) (models.StoredFile, error)
	Read(context.Context, string) (models.FileContent, error)
	Remove(context.Context, string) error
}
type AttachmentImages interface {
	Convert(context.Context, io.Reader, string) ([]byte, string, error)
}

var ErrExternalAttachment = errors.New("attachment is not stored locally")
var ErrInvalidStoredPhoto = errors.New("stored profile photo has an unsupported content type")
var ErrInvalidStoredAttachment = errors.New("stored attachment size differs from metadata")
var ErrAttachmentPagination = errors.New("invalid attachment pagination")

type AttachmentService struct {
	store  AttachmentStore
	files  AttachmentStorage
	images AttachmentImages
}

func NewAttachmentService(store AttachmentStore, files AttachmentStorage, images AttachmentImages) *AttachmentService {
	return &AttachmentService{store: store, files: files, images: images}
}

func (s *AttachmentService) Upload(ctx context.Context, owner string, input models.AttachmentUpload) (models.Attachment, error) {
	if owner == "" {
		return models.Attachment{}, models.ErrAttachmentNotFound
	}
	prepared, err := helpers.PrepareUpload(ctx, input, s.images)
	if errors.Is(err, storage.ErrFileSize) {
		return models.Attachment{}, &models.UploadError{Code: models.UploadInvalidSize}
	}
	if err != nil {
		return models.Attachment{}, err
	}
	file, err := s.files.Write(ctx, prepared.Content, prepared.FileName)
	if errors.Is(err, storage.ErrFileSize) {
		return models.Attachment{}, &models.UploadError{Code: models.UploadInvalidSize}
	}
	if err != nil {
		return models.Attachment{}, fmt.Errorf("store attachment: %w", err)
	}
	a, err := s.store.Create(ctx, models.Attachment{OwnerUserID: owner, FileName: prepared.FileName, ContentType: prepared.ContentType, SizeBytes: file.Size, SHA256: &file.SHA256, StorageKey: &file.Key})
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, models.ErrAttachmentCommitUnknown) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		cleanupErr := s.files.Remove(cleanupCtx, file.Key)
		cancel()
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove uncommitted upload: %w", cleanupErr))
		}
	}
	return models.Attachment{}, fmt.Errorf("save attachment metadata: %w", err)
}

func (s *AttachmentService) Metadata(ctx context.Context, owner, id string) (models.Attachment, error) {
	if owner == "" {
		return models.Attachment{}, models.ErrAttachmentNotFound
	}
	a, err := s.store.Get(ctx, owner, id)
	if err != nil {
		return models.Attachment{}, err
	}
	if a.OwnerUserID != owner {
		return models.Attachment{}, models.ErrAttachmentNotFound
	}
	return a, nil
}
func (s *AttachmentService) List(ctx context.Context, owner string, limit, offset int) ([]models.Attachment, error) {
	if owner == "" {
		return nil, models.ErrAttachmentNotFound
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return nil, ErrAttachmentPagination
	}
	return s.store.List(ctx, owner, limit, offset)
}
func (s *AttachmentService) Delete(ctx context.Context, owner, id string) error {
	if _, err := s.Metadata(ctx, owner, id); err != nil {
		return err
	}
	return s.store.Delete(ctx, owner, id) // Logical deletion; retention owns the bytes.
}
func (s *AttachmentService) Download(ctx context.Context, owner, id string) (models.AttachmentDownload, error) {
	return s.open(ctx, owner, id, false)
}
func (s *AttachmentService) Photo(ctx context.Context, owner string, id *string) (models.AttachmentDownload, error) {
	if id == nil {
		return models.AttachmentDownload{}, models.ErrAttachmentNotFound
	}
	return s.open(ctx, owner, *id, true)
}
func (s *AttachmentService) open(ctx context.Context, owner, id string, photo bool) (models.AttachmentDownload, error) {
	a, err := s.Metadata(ctx, owner, id)
	if err != nil {
		return models.AttachmentDownload{}, err
	}
	if a.StorageKind != models.AttachmentLocal || a.StorageKey == nil {
		return models.AttachmentDownload{}, ErrExternalAttachment
	}
	if photo {
		switch a.ContentType {
		case "image/jpeg", "image/png", "image/gif", helpers.WebPContentType:
		default:
			return models.AttachmentDownload{}, ErrInvalidStoredPhoto
		}
	}
	file, err := s.files.Read(ctx, *a.StorageKey)
	if err != nil {
		return models.AttachmentDownload{}, fmt.Errorf("open attachment content: %w", err)
	}
	if file.Size != a.SizeBytes {
		_ = file.Reader.Close()
		return models.AttachmentDownload{}, ErrInvalidStoredAttachment
	}
	return models.AttachmentDownload{Attachment: a, Content: file.Reader}, nil
}
