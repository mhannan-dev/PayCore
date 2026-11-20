package storage

import (
	"context"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"paycore/api/internal/models"
)

func (s *Local) InspectImage(ctx context.Context, key string) (models.ImageInfo, error) {
	if err := ctx.Err(); err != nil {
		return models.ImageInfo{}, err
	}
	file, err := s.Open(key)
	if err != nil {
		return models.ImageInfo{}, err
	}
	defer file.Close()
	cfg, format, err := image.DecodeConfig(file)
	if err != nil {
		// An unreadable image header cannot satisfy the profile-photo policy.
		return models.ImageInfo{}, nil
	}
	if err := ctx.Err(); err != nil {
		return models.ImageInfo{}, err
	}
	return models.ImageInfo{Width: cfg.Width, Height: cfg.Height, Format: models.ImageFormat(format)}, nil
}
