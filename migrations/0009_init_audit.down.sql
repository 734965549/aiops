DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code LIKE 'app:audits:%'
);

DELETE FROM iam_permission WHERE code LIKE 'app:audits:%';

DROP TABLE IF EXISTS audit_operation;
