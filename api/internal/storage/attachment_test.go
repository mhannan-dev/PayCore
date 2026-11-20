package storage_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"paycore/api/internal/storage"
)

func TestAttachmentStorageLifecycleAndCancellation(t *testing.T) {
	files, err := storage.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	ctx := context.Background()
	stored, err := files.Write(ctx, strings.NewReader("saved bytes"), "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	content, err := files.Read(ctx, stored.Key)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(content.Reader)
	_ = content.Reader.Close()
	if err != nil || string(data) != "saved bytes" || content.Size != stored.Size {
		t.Fatal("file content/metadata differ")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = files.Write(canceled, strings.NewReader("never saved"), "note.txt"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled write accepted")
	}
	if _, err = files.Read(canceled, stored.Key); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read accepted")
	}
	if err = files.Remove(canceled, stored.Key); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled removal accepted")
	}
	if err = files.Remove(ctx, "../outside.txt"); err == nil {
		t.Fatal("unconfined key accepted")
	}
	if err = files.Remove(ctx, stored.Key); err != nil {
		t.Fatal(err)
	}
	if err = files.Remove(ctx, stored.Key); err != nil {
		t.Fatal("cleanup must be idempotent")
	}
	if _, err = files.Read(ctx, stored.Key); err == nil {
		t.Fatal("removed content still available")
	}
}
