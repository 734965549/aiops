-- 0049 down: 人工回滚参考（runner 不自动执行）。
DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code IN ('app:ai:chat', 'app:ai:chat:feedback')
);
DELETE FROM iam_permission WHERE code IN ('app:ai:chat', 'app:ai:chat:feedback');

DROP TABLE IF EXISTS ai_session_message;
DROP TABLE IF EXISTS ai_session;
