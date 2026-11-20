-- Version 002 preserved the legacy name as first_name before creating this
-- redundant generated column. First/last names remain intact.
ALTER TABLE users DROP COLUMN IF EXISTS name;
