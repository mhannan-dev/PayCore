CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL CHECK (code = upper(btrim(code)) AND length(code) BETWEEN 1 AND 50),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL CHECK (code = lower(btrim(code)) AND length(code) BETWEEN 1 AND 80),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);
CREATE INDEX idx_user_roles_role ON user_roles(role_id);
CREATE INDEX idx_user_roles_user ON user_roles(user_id);

CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);
CREATE INDEX idx_role_permissions_perm ON role_permissions(permission_id);
CREATE INDEX idx_role_permissions_role ON role_permissions(role_id);

-- Insert baseline roles
INSERT INTO roles (code, name, description) VALUES
    ('ADMIN', 'System Administrator', 'Full system administrative, configuration, and user management control'),
    ('OPERATOR', 'Standard Operator', 'Basic financial operations and ledger reading'),
    ('LEAD_OPERATOR', 'Lead Operator', 'High-value ledger transfers, batch operations, and transaction reversals'),
    ('COMPLIANCE_AUDITOR', 'Compliance Auditor', 'Financial audit, zero-sum invariant verification, and compliance reporting'),
    ('STAFF_ENGINEER', 'Staff SRE / Engineer', 'Infrastructure diagnostics, request simulator, and system telemetry')
ON CONFLICT (code) DO NOTHING;

-- Insert baseline permissions
INSERT INTO permissions (code, name, description) VALUES
    ('users:read', 'Read Users', 'View user profiles, roles, and status'),
    ('users:write', 'Manage Users', 'Create, update, and assign roles to users'),
    ('transfers:read', 'Read Transfers', 'View transfer history and journal entries'),
    ('transfers:create', 'Create Transfers', 'Execute double-entry ledger transfers'),
    ('transfers:reverse', 'Reverse Transfers', 'Issue compensating reversal entries'),
    ('ledger:read', 'Read Ledger', 'View append-only ledger entries and balances'),
    ('audit:read', 'Read Audit', 'Inspect system audit trails and outbox events'),
    ('audit:verify', 'Verify Invariants', 'Run zero-sum balance and consistency checks'),
    ('reports:read', 'Read Reports', 'View financial statements and reconciliation summaries'),
    ('reports:export', 'Export Reports', 'Export compliance and transaction reports'),
    ('simulator:run', 'Run Simulator', 'Execute idempotency and load simulation tools'),
    ('system:read', 'Read System Status', 'View system health, database, and broker metrics')
ON CONFLICT (code) DO NOTHING;

-- Map permissions to roles
-- ADMIN gets all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p WHERE r.code = 'ADMIN'
ON CONFLICT DO NOTHING;

-- LEAD_OPERATOR
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'LEAD_OPERATOR' AND p.code IN (
    'transfers:read', 'transfers:create', 'transfers:reverse',
    'ledger:read', 'audit:read', 'reports:read', 'users:read'
)
ON CONFLICT DO NOTHING;

-- OPERATOR
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'OPERATOR' AND p.code IN (
    'transfers:read', 'transfers:create', 'ledger:read'
)
ON CONFLICT DO NOTHING;

-- COMPLIANCE_AUDITOR
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'COMPLIANCE_AUDITOR' AND p.code IN (
    'ledger:read', 'audit:read', 'audit:verify', 'reports:read', 'reports:export', 'transfers:read'
)
ON CONFLICT DO NOTHING;

-- STAFF_ENGINEER
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'STAFF_ENGINEER' AND p.code IN (
    'ledger:read', 'audit:read', 'simulator:run', 'system:read'
)
ON CONFLICT DO NOTHING;

-- Backfill user_roles from existing users.roles array
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
CROSS JOIN LATERAL unnest(u.roles) AS user_role_code
JOIN roles r ON r.code = user_role_code
ON CONFLICT DO NOTHING;
