DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code LIKE 'app:executions:%'
);

DELETE FROM iam_permission WHERE code LIKE 'app:executions:%';

DROP TABLE IF EXISTS exec_step;
DROP TABLE IF EXISTS exec_task;

DELETE FROM schema_migrations WHERE version = '0011';
