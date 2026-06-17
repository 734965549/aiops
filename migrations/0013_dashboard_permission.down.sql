DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code = 'app:dashboard:read'
);

DELETE FROM iam_permission WHERE code = 'app:dashboard:read';
