-- 0050: AI 助手消息幂等键（client_message_id）
-- 目的：客户端超时重试时，同一会话内同一 client_message_id 只落一条用户消息，
-- 避免重复消息与重复模型调用。列与索引均为幂等补齐，不改写已发布的 0049。
ALTER TABLE ai_session_message
    ADD COLUMN IF NOT EXISTS client_message_id VARCHAR(64) NOT NULL DEFAULT '';

COMMENT ON COLUMN ai_session_message.client_message_id IS
    'Client idempotency key for user messages; empty for assistant messages and legacy rows';

-- 部分唯一索引：空串（assistant 消息与历史数据）不参与唯一约束。
CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_session_message_client
    ON ai_session_message (session_id, client_message_id)
    WHERE client_message_id <> '';
