DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission
    WHERE code IN (
        'app:identity.users:create',
        'app:identity.external_identities:create'
    )
);

DELETE FROM iam_permission
WHERE code IN (
    'app:identity.users:create',
    'app:identity.external_identities:create'
);
