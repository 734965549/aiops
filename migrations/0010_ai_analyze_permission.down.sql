DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code = 'app:ai.analysis:analyze'
);

DELETE FROM iam_permission WHERE code = 'app:ai.analysis:analyze';
