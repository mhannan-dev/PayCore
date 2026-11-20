package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"paycore/api/internal/models"
)

type statementFixture struct {
	StatementStore
	data      string
	owner     string
	mismatch  bool
	closed    bool
	saved     int
	duplicate *string
	account   models.Account
}
type statementReader struct {
	io.Reader
	closed *bool
}

func (r statementReader) Close() error { *r.closed = true; return nil }
func (f *statementFixture) Download(_ context.Context, owner, id string) (models.AttachmentDownload, error) {
	if owner != f.owner {
		return models.AttachmentDownload{}, models.ErrAttachmentNotFound
	}
	sum := sha256.Sum256([]byte(f.data))
	hash := hex.EncodeToString(sum[:])
	if f.mismatch {
		hash = strings.Repeat("0", 64)
	}
	return models.AttachmentDownload{Attachment: models.Attachment{ID: id, OwnerUserID: f.owner, FileName: "test.csv", SizeBytes: int64(len(f.data)), SHA256: &hash}, Content: statementReader{strings.NewReader(f.data), &f.closed}}, nil
}
func (f *statementFixture) GetAccount(context.Context, string) (models.Account, error) {
	return f.account, nil
}
func (f *statementFixture) Duplicate(context.Context, string, string, string) (*string, error) {
	return f.duplicate, nil
}
func (f *statementFixture) Save(_ context.Context, owner string, p models.StatementPreview, source string) (models.StatementImport, error) {
	f.saved++
	return models.StatementImport{ID: "import", RowCount: len(p.Rows)}, nil
}

func TestStatementServiceRequiresValidOwnedUnchangedPreview(t *testing.T) {
	ctx := context.Background()
	f := &statementFixture{owner: "owner", data: csvHeader + "2026-11-20,ref,USD,12.50,CREDIT,receipt\n", account: models.Account{ID: "account", Type: models.AccountAsset, Currency: models.CurrencyUSD}}
	s := NewStatementService(f, f, f)
	req := models.StatementRequest{AccountID: "account", AttachmentID: "file"}
	p, err := s.Preview(ctx, "owner", req)
	if err != nil || !p.CanImport || !f.closed {
		t.Fatalf("preview %+v %v", p, err)
	}
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, models.ErrStatementChanged) || f.saved != 0 {
		t.Fatalf("missing hash accepted: %v", err)
	}
	if p.ContentHash == nil {
		t.Fatal("missing content hash")
	}
	req.ExpectedHash = *p.ContentHash
	if _, err = s.Import(ctx, "other", req); !errors.Is(err, models.ErrAttachmentNotFound) || f.saved != 0 {
		t.Fatalf("foreign source accepted: %v", err)
	}
	if _, err = s.Import(ctx, "owner", req); err != nil || f.saved != 1 {
		t.Fatalf("valid import: %v", err)
	}
	f.mismatch = true
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, models.ErrStatementChanged) || f.saved != 1 {
		t.Fatalf("tampered source accepted %v", err)
	}
	f.mismatch = false
	id := "existing"
	f.duplicate = &id
	p, err = s.Preview(ctx, "owner", req)
	if err != nil || p.CanImport || p.DuplicateImportID == nil {
		t.Fatalf("duplicate %+v %v", p, err)
	}
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, models.ErrStatementDuplicate) {
		t.Fatal(err)
	}
	f.duplicate = nil
	f.data = csvHeader + "2026-11-20,ref,USD,0,CREDIT,\n"
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, models.ErrStatementInvalid) || f.saved != 1 {
		t.Fatalf("invalid rows saved %v", err)
	}
	f.account.Type = models.AccountExpense
	if _, err = s.Preview(ctx, "owner", req); !errors.Is(err, models.ErrStatementAccount) {
		t.Fatal(err)
	}
}
