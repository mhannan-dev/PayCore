package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"paycore/api/internal/model"
)

type statementFixture struct {
	StatementStore
	data      string
	owner     string
	mismatch  bool
	closed    bool
	saved     int
	duplicate *string
	account   model.Account
}
type statementReader struct {
	io.Reader
	closed *bool
}

func (r statementReader) Close() error { *r.closed = true; return nil }
func (f *statementFixture) Download(_ context.Context, owner, id string) (model.AttachmentDownload, error) {
	if owner != f.owner {
		return model.AttachmentDownload{}, model.ErrAttachmentNotFound
	}
	sum := sha256.Sum256([]byte(f.data))
	hash := hex.EncodeToString(sum[:])
	if f.mismatch {
		hash = strings.Repeat("0", 64)
	}
	return model.AttachmentDownload{Attachment: model.Attachment{ID: id, OwnerUserID: f.owner, FileName: "test.csv", SizeBytes: int64(len(f.data)), SHA256: &hash}, Content: statementReader{strings.NewReader(f.data), &f.closed}}, nil
}
func (f *statementFixture) GetAccount(context.Context, string) (model.Account, error) {
	return f.account, nil
}
func (f *statementFixture) Duplicate(context.Context, string, string, string) (*string, error) {
	return f.duplicate, nil
}
func (f *statementFixture) Save(_ context.Context, owner string, p model.StatementPreview, source string) (model.StatementImport, error) {
	f.saved++
	return model.StatementImport{ID: "import", RowCount: len(p.Rows)}, nil
}

func TestStatementServiceRequiresValidOwnedUnchangedPreview(t *testing.T) {
	ctx := context.Background()
	f := &statementFixture{owner: "owner", data: csvHeader + "2026-11-20,ref,USD,12.50,CREDIT,receipt\n", account: model.Account{ID: "account", Type: model.AccountAsset, Currency: model.CurrencyUSD}}
	s := NewStatementService(f, f, f)
	req := model.StatementRequest{AccountID: "account", AttachmentID: "file"}
	p, err := s.Preview(ctx, "owner", req)
	if err != nil || !p.CanImport || !f.closed {
		t.Fatalf("preview %+v %v", p, err)
	}
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, model.ErrStatementChanged) || f.saved != 0 {
		t.Fatalf("missing hash accepted: %v", err)
	}
	if p.ContentHash == nil {
		t.Fatal("missing content hash")
	}
	req.ExpectedHash = *p.ContentHash
	if _, err = s.Import(ctx, "other", req); !errors.Is(err, model.ErrAttachmentNotFound) || f.saved != 0 {
		t.Fatalf("foreign source accepted: %v", err)
	}
	if _, err = s.Import(ctx, "owner", req); err != nil || f.saved != 1 {
		t.Fatalf("valid import: %v", err)
	}
	f.mismatch = true
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, model.ErrStatementChanged) || f.saved != 1 {
		t.Fatalf("tampered source accepted %v", err)
	}
	f.mismatch = false
	id := "existing"
	f.duplicate = &id
	p, err = s.Preview(ctx, "owner", req)
	if err != nil || p.CanImport || p.DuplicateImportID == nil {
		t.Fatalf("duplicate %+v %v", p, err)
	}
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, model.ErrStatementDuplicate) {
		t.Fatal(err)
	}
	f.duplicate = nil
	f.data = csvHeader + "2026-11-20,ref,USD,0,CREDIT,\n"
	if _, err = s.Import(ctx, "owner", req); !errors.Is(err, model.ErrStatementInvalid) || f.saved != 1 {
		t.Fatalf("invalid rows saved %v", err)
	}
	f.account.Type = model.AccountExpense
	if _, err = s.Preview(ctx, "owner", req); !errors.Is(err, model.ErrStatementAccount) {
		t.Fatal(err)
	}
}
