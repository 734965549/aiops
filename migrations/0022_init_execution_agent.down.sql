DROP TABLE IF EXISTS exec_log_stream;
DROP TABLE IF EXISTS exec_lease;
DROP TABLE IF EXISTS exec_command_spec;
DROP TABLE IF EXISTS exec_agent;
DROP TABLE IF EXISTS exec_medium;

ALTER TABLE exec_step DROP COLUMN IF EXISTS requires_tty;
ALTER TABLE exec_step DROP COLUMN IF EXISTS working_dir;
ALTER TABLE exec_step DROP COLUMN IF EXISTS output_redaction;
ALTER TABLE exec_step DROP COLUMN IF EXISTS arguments;
ALTER TABLE exec_step DROP COLUMN IF EXISTS command_template;
ALTER TABLE exec_step DROP COLUMN IF EXISTS command_spec_id;

ALTER TABLE exec_task DROP COLUMN IF EXISTS command_spec_id;
ALTER TABLE exec_task DROP COLUMN IF EXISTS lease_id;
ALTER TABLE exec_task DROP COLUMN IF EXISTS dispatch_status;
ALTER TABLE exec_task DROP COLUMN IF EXISTS agent_id;
ALTER TABLE exec_task DROP COLUMN IF EXISTS medium_id;
ALTER TABLE exec_task DROP COLUMN IF EXISTS execution_mode;

DELETE FROM iam_role_ai_tool_permission
WHERE tool_permission_id IN (
    SELECT tool_permission_id FROM iam_ai_tool_permission
    WHERE tool_code IN ('execution.media.list', 'execution.tasks.propose', 'execution.tasks.dispatch')
);

DELETE FROM iam_ai_tool_permission
WHERE tool_code IN ('execution.media.list', 'execution.tasks.propose', 'execution.tasks.dispatch');

DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission
    WHERE code LIKE 'app:executions:media:%'
       OR code LIKE 'app:executions:command_specs:%'
       OR code = 'app:executions:agents:manage'
);

DELETE FROM iam_permission
WHERE code LIKE 'app:executions:media:%'
   OR code LIKE 'app:executions:command_specs:%'
   OR code = 'app:executions:agents:manage';
