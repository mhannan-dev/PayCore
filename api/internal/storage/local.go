package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxFileSize int64 = 10 << 20

var ErrFileSize = errors.New("file must be between 1 byte and 10 MiB")

type Local struct{ root *os.Root }

func OpenLocal(directory string) (*Local, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Local{root: root}, nil
}
func (s *Local) Close() error { return s.root.Close() }

func validKey(key string) bool {
	if len(key) < 64 || len(key) > 75 {
		return false
	}
	base := key[:64]
	for _, c := range base {
		if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	if len(key) > 64 {
		ext := key[64:]
		if ext[0] != '.' || len(ext) < 2 {
			return false
		}
		for _, c := range ext[1:] {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}
	return true
}

func sanitizeExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) < 2 || len(ext) > 11 {
		return ""
	}
	for _, c := range ext[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return ""
		}
	}
	return ext
}

// FileExtension preserves a simple original extension, normalized to lowercase.
// Unknown or unsafe extensions use .bin; paths are never part of a storage key.
func FileExtension(filename string) string {
	if ext := sanitizeExt(filename); ext != "" {
		return ext
	}
	return ".bin"
}

func (s *Local) Save(reader io.Reader, filename ...string) (key, digest string, size int64, err error) {
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return
	}
	key = hex.EncodeToString(random[:])
	if len(filename) > 0 {
		key += FileExtension(filename[0])
	}
	file, err := s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return
	}
	complete := false
	defer func() {
		_ = file.Close() // Checked below on success; after a failure the file is removed.
		if !complete {
			_ = s.root.Remove(key)
		}
	}()
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, MaxFileSize+1))
	if err != nil {
		return
	}
	if size == 0 || size > MaxFileSize {
		err = ErrFileSize
		return
	}
	if err = file.Sync(); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}
	digest = hex.EncodeToString(hash.Sum(nil))
	complete = true
	return
}
func (s *Local) Open(key string) (*os.File, error) {
	if !validKey(key) {
		return nil, os.ErrNotExist
	}
	return s.root.Open(key)
}
