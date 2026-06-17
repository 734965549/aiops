-- 0015 rollback reference for identity access-control management seeds.

DELETE FROM iam_role_data_scope
WHERE role_id IN (
    SELECT role_id FROM iam_role WHERE code = 'viewer'
);

DELETE FROM iam_role_permission
WHERE role_id IN (
    SELECT role_id FROM iam_role WHERE code IN ('admin', 'viewer')
)
AND permission_id IN (
    SELECT permission_id FROM iam_permission
    WHERE code IN (
        'app:identity.users:read',
        'app:identity.data_scopes:read',
        'app:identity.ai_tool_permissions:read',
        'app:identity.access_control:write',
        'app:identity.profile:read',
        'app:identity.profile.roles:read',
        'app:dashboard:read',
        'app:alerts:read',
        'app:assets:read',
        'app:runbooks:read',
        'app:executions:read'
    )
);

DELETE FROM iam_role WHERE code = 'viewer';

DELETE FROM iam_permission
WHERE code IN (
    'app:identity.users:read',
    'app:identity.data_scopes:read',
    'app:identity.ai_tool_permissions:read',
    'app:identity.access_control:write'
);
