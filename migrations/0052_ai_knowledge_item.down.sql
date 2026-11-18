-- 0052 回滚参考（人工执行）：删除知识条目表与审核权限。
DELETE FROM iam_role_permission
WHERE permission_id IN (SELECT permission_id FROM iam_permission WHERE code = 'app:ai:knowledge:manage');

DELETE FROM iam_permission WHERE code = 'app:ai:knowledge:manage';

DROP TABLE IF EXISTS ai_knowledge_item;
