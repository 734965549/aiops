-- 0050 回滚参考（人工执行）：删除幂等键列与部分唯一索引。
DROP INDEX IF EXISTS uq_ai_session_message_client;

ALTER TABLE ai_session_message
    DROP COLUMN IF EXISTS client_message_id;
