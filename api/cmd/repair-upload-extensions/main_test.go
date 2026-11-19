package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCopyVerifiedPreservesOriginalAndSupportsRetry(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	key := strings.Repeat("a", 64)
	body := []byte("original file contents")
	digest := sha256.Sum256(body)
	file := legacyFile{key: key, digest: hex.EncodeToString(digest[:]), size: int64(len(body))}
	if err = root.WriteFile(key, body, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = copyVerified(root, file, key+".txt"); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{key, key + ".txt"} {
			stored, err := root.ReadFile(name)
			if err != nil || string(stored) != string(body) {
				t.Fatal("repair lost or changed original bytes")
			}
		}
	}
	// A conflicting destination must never be overwritten.
	if err = root.WriteFile(key+".txt", []byte("different content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = copyVerified(root, file, key+".txt"); err == nil {
		t.Fatal("conflicting destination accepted")
	}
	stored, err := root.ReadFile(key + ".txt")
	if err != nil || string(stored) != "different content" {
		t.Fatal("conflicting destination overwritten")
	}
	file.digest = strings.Repeat("0", 64)
	if err = copyVerified(root, file, key+".pdf"); err == nil {
		t.Fatal("incorrect source checksum accepted")
	}
	if _, err = root.Stat(key + ".pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed copy left a partial destination")
	}
	if _, err = root.Stat(key); err != nil {
		t.Fatal("failed repair removed original")
	}
}
