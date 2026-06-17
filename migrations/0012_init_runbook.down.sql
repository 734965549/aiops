ALTER TABLE exec_step DROP COLUMN IF EXISTS timeout_seconds;
ALTER TABLE exec_step DROP COLUMN IF EXISTS rollback_plan;
ALTER TABLE exec_step DROP COLUMN IF EXISTS dry_run;
ALTER TABLE exec_step DROP COLUMN IF EXISTS risk_level;
ALTER TABLE exec_step DROP COLUMN IF EXISTS parameters;
ALTER TABLE exec_step DROP COLUMN IF EXISTS runbook_step_id;

ALTER TABLE exec_task DROP COLUMN IF EXISTS dry_run;
ALTER TABLE exec_task DROP COLUMN IF EXISTS runbook_snapshot;
ALTER TABLE exec_task DROP COLUMN IF EXISTS runbook_template_id;

DROP TABLE IF EXISTS runbook_step;
DROP TABLE IF EXISTS runbook_template;

DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission
    WHERE code IN ('app:runbooks:read', 'app:runbooks:create', 'app:runbooks:update', 'app:runbooks:delete')
);

DELETE FROM iam_permission
WHERE code IN ('app:runbooks:read', 'app:runbooks:create', 'app:runbooks:update', 'app:runbooks:delete');
