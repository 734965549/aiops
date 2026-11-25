-- 0063 down: 移除 AI RDS 备份查询工具权限。
DELETE FROM iam_role_ai_tool_permission
WHERE tool_permission_id = '00000000-0000-0000-0003-000000000016';

DELETE FROM iam_ai_tool_permission
WHERE tool_code = 'rds.backup.query';