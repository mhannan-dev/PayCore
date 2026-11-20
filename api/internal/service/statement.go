package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	"paycore/api/internal/model"
)

type StatementFiles interface {
	Download(context.Context, string, string) (model.AttachmentDownload, error)
}
type StatementAccounts interface {
	GetAccount(context.Context, string) (model.Account, error)
}
type StatementStore interface {
	Duplicate(context.Context, string, string, string) (*string, error)
	Save(context.Context, string, model.StatementPreview, string) (model.StatementImport, error)
	List(context.Context, string, int, int) ([]model.StatementImport, error)
	Detail(context.Context, string, string, int, int) (model.StatementDetail, error)
}
type StatementService struct {
	files    StatementFiles
	accounts StatementAccounts
	store    StatementStore
}

func NewStatementService(files StatementFiles, accounts StatementAccounts, store StatementStore) *StatementService {
	return &StatementService{files, accounts, store}
}

func (s *StatementService) prepare(ctx context.Context, owner string, req model.StatementRequest) (model.StatementPreview, string, error) {
	if owner == "" {
		return model.StatementPreview{}, "", model.ErrStatementNotFound
	}
	a, err := s.accounts.GetAccount(ctx, req.AccountID)
	if err != nil {
		return model.StatementPreview{}, "", err
	}
	if a.Type != model.AccountAsset && a.Type != model.AccountLiability {
		return model.StatementPreview{}, "", model.ErrStatementAccount
	}
	file, err := s.files.Download(ctx, owner, req.AttachmentID)
	if err != nil {
		return model.StatementPreview{}, "", err
	}
	defer file.Content.Close()
	if file.Attachment.OwnerUserID != owner || !strings.EqualFold(filepath.Ext(file.Attachment.FileName), ".csv") || file.Attachment.SizeBytes > model.StatementMaxBytes {
		return model.StatementPreview{}, "", model.ErrStatementFormat
	}
	data, err := io.ReadAll(io.LimitReader(file.Content, model.StatementMaxBytes+1))
	if err != nil {
		return model.StatementPreview{}, "", err
	}
	digest := sha256.Sum256(data)
	sourceHash := hex.EncodeToString(digest[:])
	if file.Attachment.SHA256 == nil || *file.Attachment.SHA256 != sourceHash || int64(len(data)) != file.Attachment.SizeBytes {
		return model.StatementPreview{}, "", model.ErrStatementChanged
	}
	p, err := parseStatementCSV(ctx, data, a.Currency)
	if err != nil {
		return p, "", err
	}
	p.AttachmentID = req.AttachmentID
	p.AccountID = req.AccountID
	p.FileName = file.Attachment.FileName
	if p.ContentHash != nil {
		p.DuplicateImportID, err = s.store.Duplicate(ctx, owner, a.ID, *p.ContentHash)
		p.CanImport = p.DuplicateImportID == nil
	}
	return p, sourceHash, err
}
func (s *StatementService) Preview(ctx context.Context, owner string, req model.StatementRequest) (model.StatementPreview, error) {
	p, _, err := s.prepare(ctx, owner, req)
	return p, err
}
func (s *StatementService) Import(ctx context.Context, owner string, req model.StatementRequest) (model.StatementImport, error) {
	p, source, err := s.prepare(ctx, owner, req)
	if err != nil {
		return model.StatementImport{}, err
	}
	if p.InvalidRows > 0 || p.ContentHash == nil {
		return model.StatementImport{}, model.ErrStatementInvalid
	}
	if req.ExpectedHash != *p.ContentHash {
		return model.StatementImport{}, model.ErrStatementChanged
	}
	if p.DuplicateImportID != nil {
		return model.StatementImport{}, model.ErrStatementDuplicate
	}
	return s.store.Save(ctx, owner, p, source)
}
func (s *StatementService) List(ctx context.Context, owner string, limit, offset int) ([]model.StatementImport, error) {
	if owner == "" || limit < 1 || limit > 100 || offset < 0 {
		return nil, model.ErrStatementNotFound
	}
	return s.store.List(ctx, owner, limit, offset)
}
func (s *StatementService) Detail(ctx context.Context, owner, id string, limit, offset int) (model.StatementDetail, error) {
	if owner == "" || limit < 1 || limit > 100 || offset < 0 {
		return model.StatementDetail{}, model.ErrStatementNotFound
	}
	return s.store.Detail(ctx, owner, id, limit, offset)
}
