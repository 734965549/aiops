-- 0015 identity access-control management permissions and viewer role.

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000033', 'app:identity.users:read', 'Read platform users', 'identity.users', 'read', 'List platform users for access-control management', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000034', 'app:identity.data_scopes:read', 'Read data scopes', 'identity.data_scopes', 'read', 'List Identity data scopes', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000035', 'app:identity.ai_tool_permissions:read', 'Read AI tool permissions', 'identity.ai_tool_permissions', 'read', 'List AI tool permission dictionary', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000036', 'app:identity.access_control:write', 'Manage access control', 'identity.access_control', 'write', 'Manage user-role and role-permission bindings', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role (role_id, code, name, description, status, is_system, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0000-000000000002', 'viewer', 'Viewer', 'Built-in read-only role for demos and non-admin verification', 'active', TRUE, NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    is_system = TRUE,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN (
    'app:identity.users:read',
    'app:identity.data_scopes:read',
    'app:identity.ai_tool_permissions:read',
    'app:identity.access_control:write'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN (
    'app:identity.profile:read',
    'app:identity.profile.roles:read',
    'app:dashboard:read',
    'app:alerts:read',
    'app:assets:read',
    'app:runbooks:read',
    'app:executions:read'
)
WHERE r.code = 'viewer'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_role_data_scope (role_id, data_scope_id, created_at, updated_at)
SELECT r.role_id, ds.data_scope_id, NOW(), NOW()
FROM iam_role r
JOIN iam_data_scope ds ON ds.code = 'all-data'
WHERE r.code = 'viewer'
ON CONFLICT (role_id, data_scope_id) DO UPDATE SET updated_at = NOW();
