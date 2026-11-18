-- 0055 回滚参考（人工执行）：删除规则体检工具权限。
DELETE FROM iam_role_ai_tool_permission
WHERE tool_permission_id IN (SELECT tool_permission_id FROM iam_ai_tool_permission WHERE tool_code = 'alert.rules.list');

DELETE FROM iam_ai_tool_permission WHERE tool_code = 'alert.rules.list';
