package service

import (
	"context"
	"errors"

	"paycore/api/internal/model"
)

var ErrInvalidPhoto = errors.New("attachment is not a supported local profile photo")

type PhotoStore interface {
	Get(context.Context, string, string) (model.Attachment, error)
	SetPhoto(context.Context, string, *string) error
}
type PhotoInspector interface {
	InspectImage(context.Context, string) (model.ImageInfo, error)
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
func (s *PhotoService) Set(ctx context.Context, owner string, id *string) (model.PhotoResult, error) {
	if id != nil {
		a, err := s.store.Get(ctx, owner, *id)
		if err != nil {
			return model.PhotoResult{}, err
		}
		if a.StorageKind != "LOCAL" || a.StorageKey == nil {
			return model.PhotoResult{}, ErrInvalidPhoto
		}
		info, err := s.images.InspectImage(ctx, *a.StorageKey)
		if err != nil {
			return model.PhotoResult{}, err
		}
		if !info.ValidPhoto() {
			return model.PhotoResult{}, ErrInvalidPhoto
		}
	}
	if err := ctx.Err(); err != nil {
		return model.PhotoResult{}, err
	}
	if err := s.store.SetPhoto(ctx, owner, id); err != nil {
		return model.PhotoResult{}, err
	}
	result := model.PhotoResult{AttachmentID: id}
	if id != nil {
		url := "/api/v1/auth/photo"
		result.Photo = &url
	}
	return result, nil
}
