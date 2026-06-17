-- 0002 回滚：按 code / tool_code 删除种子数据，保留 0001 表结构。
-- 生产回滚须 DBA 审批；若 admin 角色仍被用户引用，请先解除 iam_user_role 关联。

DELETE FROM iam_role_ai_tool_permission
WHERE role_id IN (SELECT role_id FROM iam_role WHERE code = 'admin');

DELETE FROM iam_role_data_scope
WHERE role_id IN (SELECT role_id FROM iam_role WHERE code = 'admin');

DELETE FROM iam_role_permission
WHERE role_id IN (SELECT role_id FROM iam_role WHERE code = 'admin');

DELETE FROM iam_ai_tool_permission
WHERE tool_code IN ('alarm.analyze', 'metrics.query', 'logs.search', 'execution.runbook');

DELETE FROM iam_data_scope
WHERE code = 'all-data';

DELETE FROM iam_permission
WHERE code IN (
    'app:identity.roles:read',
    'app:identity.permissions:read',
    'app:identity.profile:read',
    'app:identity.profile.roles:read',
    'app:identity.authorization:execute',
    'app:ai.providers:read',
    'app:ai.providers:write',
    'app:ai.providers:delete',
    'app:ai.tools:invoke'
);

DELETE FROM iam_role
WHERE code = 'admin';

DELETE FROM schema_migrations WHERE version = '0002';
