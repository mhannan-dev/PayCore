package models

import "time"

type AttachmentStorageKind string

const (
	AttachmentLocal    AttachmentStorageKind = "LOCAL"
	AttachmentExternal AttachmentStorageKind = "EXTERNAL"
)

type Attachment struct {
	ID          string                `json:"id"`
	OwnerUserID string                `json:"owner_user_id"`
	FileName    string                `json:"file_name"`
	ContentType string                `json:"content_type"`
	SizeBytes   int64                 `json:"size_bytes"`
	SHA256      *string               `json:"sha256"`
	StorageKind AttachmentStorageKind `json:"storage_kind"`
	StorageKey  *string               `json:"-"`
	ExternalURL *string               `json:"external_url,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
}
