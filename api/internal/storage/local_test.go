package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLocalFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key, digest, size, err := s.Save(strings.NewReader("sample file"))
	if err != nil || size != 11 || len(key) != 64 {
		t.Fatal("save failed")
	}
	expected := sha256.Sum256([]byte("sample file"))
	if digest != hex.EncodeToString(expected[:]) {
		t.Fatal("checksum mismatch")
	}
	f, err := s.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(body) != "sample file" {
		t.Fatal("stored content mismatch")
	}
	for _, unsafe := range []string{"../outside", "/outside", "..\\outside", "NUL", strings.Repeat("g", 64)} {
		if _, err := s.Open(unsafe); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unsafe storage key accepted")
		}
	}
	if _, _, _, err := s.Save(strings.NewReader("")); !errors.Is(err, ErrFileSize) {
		t.Fatal("empty file accepted")
	}
	if _, _, _, err := s.Save(io.LimitReader(zeroReader{}, MaxFileSize+1)); !errors.Is(err, ErrFileSize) {
		t.Fatal("oversized file accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("invalid uploads left partial files")
	}
}

type zeroReader struct{}

func TestLocalFileExtensions(t *testing.T) {
	s, err := OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, tc := range []struct{ filename, suffix string }{
		{"photo.JPG", ".jpg"},
		{"document.pdf", ".pdf"},
		{"archive.tar.gz", ".gz"},
		{"file.abcdefghij", ".abcdefghij"},
		{"file.abcdefghijk", ".bin"},
		{"file.bad-ext", ".bin"},
		{"no-extension", ".bin"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			key, _, _, err := s.Save(strings.NewReader("file bytes"), tc.filename)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(key, tc.suffix) || !validKey(key) {
				t.Fatalf("unexpected storage key %q", key)
			}
			file, err := s.Open(key)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil || string(body) != "file bytes" {
				t.Fatal("extension upload changed file bytes")
			}
		})
	}
	for _, key := range []string{
		strings.Repeat("a", 64) + ".../outside",
		strings.Repeat("a", 64) + ".png/../outside",
		strings.Repeat("a", 64) + ".png:stream",
		strings.Repeat("a", 64) + ".",
		strings.Repeat("a", 64) + ".abcdefghijk",
	} {
		if _, err := s.Open(key); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unsafe extension storage key accepted")
		}
	}
}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
