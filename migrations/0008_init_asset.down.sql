DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code LIKE 'app:assets:%'
);

DELETE FROM iam_permission WHERE code LIKE 'app:assets:%';

DROP TABLE IF EXISTS asset_resource;
DROP TABLE IF EXISTS asset_application;
