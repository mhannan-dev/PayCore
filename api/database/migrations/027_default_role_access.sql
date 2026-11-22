-- Default access for every built-in role after the 026 role consolidation.
-- ADMIN already holds every permission. The two other roles gain the read and
-- day-to-day grants their job needs, without breaking separation of duties:
--   ACCOUNTANT          + reports:read            (daily financial summaries)
--                       + quantum:read/quantum:write (own testnet merchant invoices;
--                                                  ownership is enforced in SQL)
--   COMPLIANCE_AUDITOR  + system:read             (read-only event delivery health)
-- Still ADMIN-only: users:write, accounts:write, transfers:reverse (settlement
-- capture/release), notifications:retry, system:write, simulator:run, audit:read
-- for accountants. Auditors keep no money-moving or configuration grants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM (VALUES
    ('ACCOUNTANT', 'reports:read'),
    ('ACCOUNTANT', 'quantum:read'),
    ('ACCOUNTANT', 'quantum:write'),
    ('COMPLIANCE_AUDITOR', 'system:read')
) AS grant_pair(role_code, permission_code)
JOIN roles r ON r.code = grant_pair.role_code
JOIN permissions p ON p.code = grant_pair.permission_code
ON CONFLICT DO NOTHING;
