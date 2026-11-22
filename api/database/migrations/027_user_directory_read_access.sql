-- Baseline user-directory and role-catalogue visibility for the two non-admin
-- roles. Administrative mutations continue to require users:write.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles AS r
CROSS JOIN permissions AS p
WHERE r.code IN ('ACCOUNTANT', 'COMPLIANCE_AUDITOR')
  AND p.code = 'users:read'
ON CONFLICT DO NOTHING;
