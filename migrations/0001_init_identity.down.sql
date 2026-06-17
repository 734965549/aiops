DROP TABLE IF EXISTS iam_role_ai_tool_permission;
DROP TABLE IF EXISTS iam_ai_tool_permission;
DROP TABLE IF EXISTS iam_role_data_scope;
DROP TABLE IF EXISTS iam_data_scope;
DROP TABLE IF EXISTS iam_role_permission;
DROP TABLE IF EXISTS iam_user_role;
DROP TABLE IF EXISTS iam_permission;
DROP TABLE IF EXISTS iam_role;
DROP TABLE IF EXISTS iam_user;

-- 清除自研 runner 版本记录，便于 dev 环境完整 down 后重新 up。
DELETE FROM schema_migrations WHERE version IN ('0001', '0002');
