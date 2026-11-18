-- Preserve existing display names without guessing how to split personal names.
-- New databases already have these columns from migration 001.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'first_name'
    ) THEN
        ALTER TABLE users RENAME COLUMN name TO first_name;
    END IF;
END $$;

ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name TEXT;
ALTER TABLE users ALTER COLUMN last_name DROP NOT NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS photo TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS name TEXT
    GENERATED ALWAYS AS (btrim(first_name || COALESCE(' ' || last_name, ''))) STORED;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conrelid = 'users'::regclass AND conname = 'users_last_name_check'
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_last_name_check
            CHECK (last_name IS NULL OR length(btrim(last_name)) BETWEEN 1 AND 80);
    END IF;
END $$;
