-- 0054 回滚参考（人工执行）：删除记忆管理权限。
DELETE FROM iam_role_permission
WHERE permission_id IN (SELECT permission_id FROM iam_permission WHERE code = 'app:ai:memory:manage');

DELETE FROM iam_permission WHERE code = 'app:ai:memory:manage';
