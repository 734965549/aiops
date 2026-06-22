DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code = 'app:observability:read'
);

DELETE FROM iam_permission WHERE code = 'app:observability:read';

DROP TABLE IF EXISTS obs_evidence_ref;
