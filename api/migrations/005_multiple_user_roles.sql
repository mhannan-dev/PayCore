CREATE FUNCTION valid_user_roles(candidate TEXT[]) RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE AS $$
    SELECT COALESCE(
        array_ndims(candidate) = 1 AND array_lower(candidate, 1) = 1
        AND cardinality(candidate) BETWEEN 1 AND 5
        AND array_position(candidate, NULL) IS NULL
        AND candidate <@ ARRAY['OPERATOR', 'ADMIN', 'LEAD_OPERATOR', 'COMPLIANCE_AUDITOR', 'STAFF_ENGINEER']::TEXT[]
        AND cardinality(candidate) = (SELECT count(DISTINCT role) FROM unnest(candidate) AS role),
        FALSE
    );
$$;

ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ALTER COLUMN role DROP DEFAULT;
ALTER TABLE users RENAME COLUMN role TO roles;
ALTER TABLE users ALTER COLUMN roles TYPE TEXT[] USING ARRAY[roles];
ALTER TABLE users ALTER COLUMN roles SET DEFAULT ARRAY['OPERATOR']::TEXT[];
ALTER TABLE users ADD CONSTRAINT users_roles_check CHECK (valid_user_roles(roles));
