CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name TEXT NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
    content_type TEXT NOT NULL CHECK (length(content_type) BETWEEN 1 AND 255),
    size_bytes BIGINT NOT NULL CHECK (size_bytes BETWEEN 0 AND 10485760),
    sha256 TEXT,
    storage_kind TEXT NOT NULL DEFAULT 'LOCAL' CHECK (storage_kind IN ('LOCAL', 'EXTERNAL')),
    storage_key TEXT UNIQUE,
    external_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (id, owner_user_id),
    CHECK (
        (storage_kind = 'LOCAL' AND storage_key ~ '^[a-f0-9]{64}$'
            AND storage_key IS NOT NULL AND sha256 ~ '^[a-f0-9]{64}$'
            AND sha256 IS NOT NULL AND size_bytes > 0 AND external_url IS NULL)
        OR (storage_kind = 'EXTERNAL' AND external_url IS NOT NULL
            AND storage_key IS NULL AND sha256 IS NULL AND size_bytes = 0)
    )
);
CREATE INDEX idx_attachments_owner_active ON attachments(owner_user_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

ALTER TABLE users ADD COLUMN photo_attachment_id UUID;
ALTER TABLE users ADD CONSTRAINT users_photo_attachment_owner_fk
    FOREIGN KEY (photo_attachment_id, id) REFERENCES attachments(id, owner_user_id);

-- Preserve old photo strings as metadata; never fetch legacy URLs during migration.
WITH imported AS (
    INSERT INTO attachments(owner_user_id, file_name, content_type, size_bytes, storage_kind, external_url)
    SELECT id, 'legacy-user-photo', 'application/octet-stream', 0, 'EXTERNAL', photo
    FROM users WHERE photo IS NOT NULL
    RETURNING id, owner_user_id
)
UPDATE users SET photo_attachment_id = imported.id FROM imported WHERE users.id = imported.owner_user_id;
ALTER TABLE users DROP COLUMN photo;
