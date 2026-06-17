-- 0004 user provisioning and external identity management permissions.

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000010', 'app:identity.users:create', 'Create platform users', 'identity.users', 'create', 'Admin-created local platform accounts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000011', 'app:identity.external_identities:create', 'Provision external identities', 'identity.external_identities', 'create', 'Admin-provisioned LDAP/AD/OAuth identity bindings', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN (
    'app:identity.users:create',
    'app:identity.external_identities:create'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
