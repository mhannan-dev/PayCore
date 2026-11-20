INSERT INTO permissions(code,name,description) VALUES
('attachments:read','Read Own Attachments','List and download attachments owned by the signed-in user'),
('attachments:write','Upload Own Attachments','Upload files and assign the signed-in user photo'),
('attachments:delete','Delete Own Attachments','Soft-delete attachments owned by the signed-in user')
ON CONFLICT(code) DO NOTHING;

-- Preserve the existing ability of the five built-in roles to manage their own
-- files. These grants never authorize access to another user's attachment.
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.code IN ('ADMIN','OPERATOR','LEAD_OPERATOR','COMPLIANCE_AUDITOR','STAFF_ENGINEER')
AND p.code IN ('attachments:read','attachments:write','attachments:delete')
ON CONFLICT DO NOTHING;
