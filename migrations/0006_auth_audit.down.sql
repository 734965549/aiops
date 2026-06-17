-- 回滚认证审计：先移除管理员权限绑定，再删除权限种子同审计表。
DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code = 'app:identity.auth_audits:read'
);

DELETE FROM iam_permission WHERE code = 'app:identity.auth_audits:read';

DROP TABLE IF EXISTS iam_auth_audit;
