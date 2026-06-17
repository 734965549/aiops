-- 0002 seed base permissions and bind them to the built-in admin role.

INSERT INTO iam_role (role_id, code, name, description, status, is_system, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'admin', 'System Admin', 'Built-in administrator role with base management and AI tool permissions', 'active', TRUE, NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    is_system = TRUE,
    updated_at = NOW();

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000001', 'app:identity.roles:read', 'Read roles', 'identity.roles', 'read', 'List Identity roles', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000002', 'app:identity.permissions:read', 'Read permissions', 'identity.permissions', 'read', 'List Identity permissions', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000003', 'app:identity.profile:read', 'Read current user', 'identity.profile', 'read', 'Read current logged-in user profile', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000004', 'app:identity.profile.roles:read', 'Read current user roles', 'identity.profile.roles', 'read', 'Read current logged-in user role bindings', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000005', 'app:identity.authorization:execute', 'Execute authorization check', 'identity.authorization', 'execute', 'Invoke unified authorization check API', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000006', 'app:ai.providers:read', 'Read AI providers', 'ai.providers', 'read', 'Read AI tool provider configuration', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000007', 'app:ai.providers:write', 'Manage AI providers', 'ai.providers', 'write', 'Create or update AI tool provider configuration', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000008', 'app:ai.providers:delete', 'Delete AI providers', 'ai.providers', 'delete', 'Delete AI tool provider configuration', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000009', 'app:ai.tools:invoke', 'Invoke AI tools', 'ai.tools', 'invoke', 'Invoke tools through the AI tool gateway', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_data_scope (data_scope_id, code, name, scope_type, scope_config, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0002-000000000001', 'all-data', 'All data', 'all', '{}'::jsonb, 'Allow access to all platform data', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    scope_type = EXCLUDED.scope_type,
    scope_config = EXCLUDED.scope_config,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_ai_tool_permission (tool_permission_id, tool_code, tool_name, permission_mode, allow_confirm, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0003-000000000001', 'alarm.analyze', 'Alert analysis tool', 'read_only', FALSE, 'Allow read-only alert analysis tool calls', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000002', 'metrics.query', 'Metrics query tool', 'read_only', FALSE, 'Allow metrics query calls', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000003', 'logs.search', 'Log search tool', 'read_only', FALSE, 'Allow log search calls', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000004', 'execution.runbook', 'Runbook execution tool', 'require_confirm', TRUE, 'Allow runbook execution tool calls after manual confirmation', NOW(), NOW())
ON CONFLICT (tool_code) DO UPDATE SET
    tool_name = EXCLUDED.tool_name,
    permission_mode = EXCLUDED.permission_mode,
    allow_confirm = EXCLUDED.allow_confirm,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN (
    'app:identity.roles:read',
    'app:identity.permissions:read',
    'app:identity.profile:read',
    'app:identity.profile.roles:read',
    'app:identity.authorization:execute',
    'app:ai.providers:read',
    'app:ai.providers:write',
    'app:ai.providers:delete',
    'app:ai.tools:invoke'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_role_data_scope (role_id, data_scope_id, created_at, updated_at)
SELECT r.role_id, ds.data_scope_id, NOW(), NOW()
FROM iam_role r
JOIN iam_data_scope ds ON ds.code = 'all-data'
WHERE r.code = 'admin'
ON CONFLICT (role_id, data_scope_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_role_ai_tool_permission (role_id, tool_permission_id, created_at, updated_at)
SELECT r.role_id, tp.tool_permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_ai_tool_permission tp ON tp.tool_code IN ('alarm.analyze', 'metrics.query', 'logs.search', 'execution.runbook')
WHERE r.code = 'admin'
ON CONFLICT (role_id, tool_permission_id) DO UPDATE SET updated_at = NOW();
