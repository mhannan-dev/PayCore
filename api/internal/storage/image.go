package storage

import (
	"context"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"paycore/api/internal/model"
)

func (s *Local) InspectImage(ctx context.Context, key string) (model.ImageInfo, error) {
	if err := ctx.Err(); err != nil {
		return model.ImageInfo{}, err
	}
	file, err := s.Open(key)
	if err != nil {
		return model.ImageInfo{}, err
	}
	defer file.Close()
	cfg, format, err := image.DecodeConfig(file)
	if err != nil {
		// An unreadable image header cannot satisfy the profile-photo policy.
		return model.ImageInfo{}, nil
	}
	if err := ctx.Err(); err != nil {
		return model.ImageInfo{}, err
	}
	return model.ImageInfo{Width: cfg.Width, Height: cfg.Height, Format: model.ImageFormat(format)}, nil
}
