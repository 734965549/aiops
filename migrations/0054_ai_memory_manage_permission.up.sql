-- 0054: AI 助手记忆管理权限（阶段 1 管理入口）
-- 记忆检索注入仍按归属用户隔离；本权限仅用于管理端列表与手动停用/启用。
INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000054', 'app:ai:memory:manage', 'Manage AI memory cases', 'ai', 'memory:manage',
     'List AI memory cases and retire or reactivate them manually', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code = 'app:ai:memory:manage'
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
