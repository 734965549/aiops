-- 0061 down：人工回滚参考，不由 runner 自动执行。
DELETE FROM iam_role_ai_tool_permission
WHERE tool_permission_id IN (
    SELECT tool_permission_id FROM iam_ai_tool_permission WHERE tool_code = 'postgres.activity.query'
);
DELETE FROM iam_ai_tool_permission WHERE tool_code = 'postgres.activity.query';
