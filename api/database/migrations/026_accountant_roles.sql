CREATE OR REPLACE FUNCTION valid_user_roles(candidate TEXT[]) RETURNS BOOLEAN
LANGUAGE SQL IMMUTABLE AS $$
    SELECT COALESCE(
        array_ndims(candidate) = 1 AND array_lower(candidate, 1) = 1
        AND cardinality(candidate) BETWEEN 1 AND 3
        AND array_position(candidate, NULL) IS NULL
        AND candidate <@ ARRAY['ADMIN', 'ACCOUNTANT', 'COMPLIANCE_AUDITOR']::TEXT[]
        AND cardinality(candidate) = (SELECT count(DISTINCT role) FROM unnest(candidate) AS role),
        FALSE
    );
$$;

ALTER TABLE users ALTER COLUMN roles SET DEFAULT ARRAY['ACCOUNTANT']::TEXT[];

INSERT INTO roles (code, name, description)
VALUES ('ACCOUNTANT', 'Accountant', 'Daily financial operations and ledger reading')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles AS r
CROSS JOIN permissions AS p
WHERE r.code = 'ACCOUNTANT'
  AND p.code IN (
    'transfers:read', 'transfers:create', 'ledger:read',
    'attachments:read', 'attachments:write', 'attachments:delete',
    'reconciliation:read', 'reconciliation:import', 'reconciliation:match',
    'notifications:read'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_at)
SELECT accountant.id, source.permission_id, source.assigned_at
FROM roles AS accountant
JOIN roles AS operator ON operator.code = 'OPERATOR'
JOIN role_permissions AS source ON source.role_id = operator.id
WHERE accountant.code = 'ACCOUNTANT'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_at)
SELECT admin.id, source.permission_id, source.assigned_at
FROM roles AS admin
JOIN roles AS legacy ON legacy.code IN ('LEAD_OPERATOR', 'STAFF_ENGINEER')
JOIN role_permissions AS source ON source.role_id = legacy.id
WHERE admin.code = 'ADMIN'
ON CONFLICT DO NOTHING;

DELETE FROM roles WHERE code IN ('OPERATOR', 'LEAD_OPERATOR', 'STAFF_ENGINEER');

UPDATE users
SET roles = ARRAY(
    SELECT mapped.role
    FROM (
        SELECT DISTINCT CASE role
            WHEN 'OPERATOR' THEN 'ACCOUNTANT'
            WHEN 'LEAD_OPERATOR' THEN 'ADMIN'
            WHEN 'STAFF_ENGINEER' THEN 'ADMIN'
            ELSE role
        END AS role
        FROM unnest(roles) AS role
    ) AS mapped
    ORDER BY CASE mapped.role
        WHEN 'ADMIN' THEN 1
        WHEN 'ACCOUNTANT' THEN 2
        WHEN 'COMPLIANCE_AUDITOR' THEN 3
        ELSE 4
    END
)
WHERE roles && ARRAY['OPERATOR', 'LEAD_OPERATOR', 'STAFF_ENGINEER']::TEXT[];

ALTER TABLE users VALIDATE CONSTRAINT users_roles_check;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
CROSS JOIN LATERAL unnest(u.roles) AS role_code
JOIN roles r ON r.code = role_code
ON CONFLICT DO NOTHING;
