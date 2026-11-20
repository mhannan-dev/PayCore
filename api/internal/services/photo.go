package services

import (
	"context"
	"errors"

	"paycore/api/internal/models"
)

var ErrInvalidPhoto = errors.New("attachment is not a supported local profile photo")

type PhotoStore interface {
	Get(context.Context, string, string) (models.Attachment, error)
	SetPhoto(context.Context, string, *string) error
}
type PhotoInspector interface {
	InspectImage(context.Context, string) (models.ImageInfo, error)
}

type PhotoService struct {
	store  PhotoStore
	images PhotoInspector
}

func NewPhotoService(store PhotoStore, images PhotoInspector) *PhotoService {
	return &PhotoService{store: store, images: images}
}

// Set validates ownership and image policy before changing the profile reference.
// Clearing a photo does not delete the former attachment.
func (s *PhotoService) Set(ctx context.Context, owner string, id *string) (models.PhotoResult, error) {
	if id != nil {
		a, err := s.store.Get(ctx, owner, *id)
		if err != nil {
			return models.PhotoResult{}, err
		}
		if a.StorageKind != models.AttachmentLocal || a.StorageKey == nil {
			return models.PhotoResult{}, ErrInvalidPhoto
		}
		info, err := s.images.InspectImage(ctx, *a.StorageKey)
		if err != nil {
			return models.PhotoResult{}, err
		}
		if !info.ValidPhoto() {
			return models.PhotoResult{}, ErrInvalidPhoto
		}
	}
	if err := ctx.Err(); err != nil {
		return models.PhotoResult{}, err
	}
	if err := s.store.SetPhoto(ctx, owner, id); err != nil {
		return models.PhotoResult{}, err
	}
	result := models.PhotoResult{AttachmentID: id}
	if id != nil {
		url := "/api/v1/auth/photo"
		result.Photo = &url
	}
	return result, nil
}
