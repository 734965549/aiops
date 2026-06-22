-- 0018 rollback Integration context (reference only, not auto-run).

DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission
    WHERE code IN (
        'app:integrations:read',
        'app:integrations:create',
        'app:integrations:update',
        'app:integrations:delete',
        'app:integrations:check'
    )
);

DELETE FROM iam_permission
WHERE code IN (
    'app:integrations:read',
    'app:integrations:create',
    'app:integrations:update',
    'app:integrations:delete',
    'app:integrations:check'
);

DROP TABLE IF EXISTS integration_check_result;
DROP TABLE IF EXISTS integration_capability;
DROP TABLE IF EXISTS integration_credential_ref;
DROP TABLE IF EXISTS integration_account;
