ALTER TABLE attachments DROP CONSTRAINT IF EXISTS attachments_check;
ALTER TABLE attachments ADD CONSTRAINT attachments_check CHECK (
    (storage_kind = 'LOCAL' AND storage_key ~ '^[a-f0-9]{64}(\.[a-zA-Z0-9]{1,10})?$'
        AND storage_key IS NOT NULL AND sha256 ~ '^[a-f0-9]{64}$'
        AND sha256 IS NOT NULL AND size_bytes > 0 AND external_url IS NULL)
    OR (storage_kind = 'EXTERNAL' AND external_url IS NOT NULL
        AND storage_key IS NULL AND sha256 IS NULL AND size_bytes = 0)
);
