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

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
