-- Add ADMIN without changing any existing user's role. Retain LEAD_OPERATOR
-- for databases created before ADMIN was introduced.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('OPERATOR', 'ADMIN', 'LEAD_OPERATOR', 'COMPLIANCE_AUDITOR', 'STAFF_ENGINEER'));
