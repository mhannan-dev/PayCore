package storage

import (
	"context"
	"errors"
	"io"
	"os"

	"paycore/api/internal/model"
)

var ErrInvalidFile = errors.New("stored content is not a regular file")

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (s *Local) Write(ctx context.Context, reader io.Reader, name string) (model.StoredFile, error) {
	if err := ctx.Err(); err != nil {
		return model.StoredFile{}, err
	}
	key, hash, size, err := s.Save(contextReader{ctx, reader}, name)
	if err != nil {
		return model.StoredFile{}, err
	}
	return model.StoredFile{Key: key, SHA256: hash, Size: size}, nil
}

func (s *Local) Read(ctx context.Context, key string) (model.FileContent, error) {
	if err := ctx.Err(); err != nil {
		return model.FileContent{}, err
	}
	file, err := s.Open(key)
	if err != nil {
		return model.FileContent{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return model.FileContent{}, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return model.FileContent{}, ErrInvalidFile
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return model.FileContent{}, err
	}
	return model.FileContent{Reader: file, Size: info.Size()}, nil
}

// Remove only accepts the same confined, random keys used by Open.
func (s *Local) Remove(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validKey(key) {
		return os.ErrNotExist
	}
	err := s.root.Remove(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
