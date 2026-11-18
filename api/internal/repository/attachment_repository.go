package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paycore/api/internal/model"
)

type AttachmentRepository struct{ pool *pgxpool.Pool }

func NewAttachmentRepository(pool *pgxpool.Pool) *AttachmentRepository {
	return &AttachmentRepository{pool: pool}
}

const attachmentColumns = "id, owner_user_id, file_name, content_type, size_bytes, sha256, storage_kind, storage_key, external_url, created_at"

func scanAttachment(row scanner) (a model.Attachment, err error) {
	err = row.Scan(&a.ID, &a.OwnerUserID, &a.FileName, &a.ContentType, &a.SizeBytes, &a.SHA256, &a.StorageKind, &a.StorageKey, &a.ExternalURL, &a.CreatedAt)
	return
}
func (r *AttachmentRepository) Create(ctx context.Context, a model.Attachment) (model.Attachment, error) {
	return scanAttachment(r.pool.QueryRow(ctx, "INSERT INTO attachments (owner_user_id, file_name, content_type, size_bytes, sha256, storage_key) VALUES ($1,$2,$3,$4,$5,$6) RETURNING "+attachmentColumns, a.OwnerUserID, a.FileName, a.ContentType, a.SizeBytes, a.SHA256, a.StorageKey))
}
func (r *AttachmentRepository) Get(ctx context.Context, owner, id string) (model.Attachment, error) {
	return scanAttachment(r.pool.QueryRow(ctx, "SELECT "+attachmentColumns+" FROM attachments WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL", id, owner))
}
func (r *AttachmentRepository) List(ctx context.Context, owner string, limit, offset int) ([]model.Attachment, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+attachmentColumns+" FROM attachments WHERE owner_user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3", owner, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Attachment, 0)
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
func (r *AttachmentRepository) Delete(ctx context.Context, owner, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var locked string
	if err = tx.QueryRow(ctx, "SELECT id FROM attachments WHERE id=$1 AND owner_user_id=$2 AND deleted_at IS NULL FOR UPDATE", id, owner).Scan(&locked); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET photo_attachment_id = NULL, updated_at = NOW() WHERE id=$1 AND photo_attachment_id=$2", owner, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE attachments SET deleted_at = NOW() WHERE id=$1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *AttachmentRepository) SetPhoto(ctx context.Context, owner string, id *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if id != nil {
		var locked string
		if err = tx.QueryRow(ctx, "SELECT id FROM attachments WHERE id=$1 AND owner_user_id=$2 AND deleted_at IS NULL AND storage_kind='LOCAL' AND content_type IN ('image/jpeg','image/png','image/gif') FOR UPDATE", *id, owner).Scan(&locked); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, "UPDATE users SET photo_attachment_id=$1, updated_at=NOW() WHERE id=$2 AND is_active", id, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}
