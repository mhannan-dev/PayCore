package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	"paycore/api/internal/models"
)

type StatementFiles interface {
	Download(context.Context, string, string) (models.AttachmentDownload, error)
}
type StatementAccounts interface {
	GetAccount(context.Context, string) (models.Account, error)
}
type StatementStore interface {
	Duplicate(context.Context, string, string, string) (*string, error)
	Save(context.Context, string, models.StatementPreview, string) (models.StatementImport, error)
	List(context.Context, string, int, int) ([]models.StatementImport, error)
	Detail(context.Context, string, string, int, int) (models.StatementDetail, error)
}
type StatementService struct {
	files    StatementFiles
	accounts StatementAccounts
	store    StatementStore
}

func NewStatementService(files StatementFiles, accounts StatementAccounts, store StatementStore) *StatementService {
	return &StatementService{files, accounts, store}
}

func (s *StatementService) prepare(ctx context.Context, owner string, req models.StatementRequest) (models.StatementPreview, string, error) {
	if owner == "" {
		return models.StatementPreview{}, "", models.ErrStatementNotFound
	}
	a, err := s.accounts.GetAccount(ctx, req.AccountID)
	if err != nil {
		return models.StatementPreview{}, "", err
	}
	if a.Type != models.AccountAsset && a.Type != models.AccountLiability {
		return models.StatementPreview{}, "", models.ErrStatementAccount
	}
	file, err := s.files.Download(ctx, owner, req.AttachmentID)
	if err != nil {
		return models.StatementPreview{}, "", err
	}
	defer file.Content.Close()
	if file.Attachment.OwnerUserID != owner || !strings.EqualFold(filepath.Ext(file.Attachment.FileName), ".csv") || file.Attachment.SizeBytes > models.StatementMaxBytes {
		return models.StatementPreview{}, "", models.ErrStatementFormat
	}
	data, err := io.ReadAll(io.LimitReader(file.Content, models.StatementMaxBytes+1))
	if err != nil {
		return models.StatementPreview{}, "", err
	}
	digest := sha256.Sum256(data)
	sourceHash := hex.EncodeToString(digest[:])
	if file.Attachment.SHA256 == nil || *file.Attachment.SHA256 != sourceHash || int64(len(data)) != file.Attachment.SizeBytes {
		return models.StatementPreview{}, "", models.ErrStatementChanged
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
func (s *StatementService) Preview(ctx context.Context, owner string, req models.StatementRequest) (models.StatementPreview, error) {
	p, _, err := s.prepare(ctx, owner, req)
	return p, err
}
func (s *StatementService) Import(ctx context.Context, owner string, req models.StatementRequest) (models.StatementImport, error) {
	p, source, err := s.prepare(ctx, owner, req)
	if err != nil {
		return models.StatementImport{}, err
	}
	if p.InvalidRows > 0 || p.ContentHash == nil {
		return models.StatementImport{}, models.ErrStatementInvalid
	}
	if req.ExpectedHash != *p.ContentHash {
		return models.StatementImport{}, models.ErrStatementChanged
	}
	if p.DuplicateImportID != nil {
		return models.StatementImport{}, models.ErrStatementDuplicate
	}
	return s.store.Save(ctx, owner, p, source)
}
func (s *StatementService) List(ctx context.Context, owner string, limit, offset int) ([]models.StatementImport, error) {
	if owner == "" || limit < 1 || limit > 100 || offset < 0 {
		return nil, models.ErrStatementNotFound
	}
	return s.store.List(ctx, owner, limit, offset)
}
func (s *StatementService) Detail(ctx context.Context, owner, id string, limit, offset int) (models.StatementDetail, error) {
	if owner == "" || limit < 1 || limit > 100 || offset < 0 {
		return models.StatementDetail{}, models.ErrStatementNotFound
	}
	return s.store.Detail(ctx, owner, id, limit, offset)
}
