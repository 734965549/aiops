-- AI 运维助手会话只读分享（契约 ops/ai-contract.md §10.18）。
-- 只存 token 的 SHA-256，明文 token 仅在创建响应中返回一次；时间戳由 Go 维护，无外键。
CREATE TABLE IF NOT EXISTS ai_session_share (
  id BIGSERIAL PRIMARY KEY,
  share_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  owner_user_id TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_ai_session_share_share_id ON ai_session_share(share_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ai_session_share_token_hash ON ai_session_share(token_hash);
CREATE INDEX IF NOT EXISTS idx_ai_session_share_session ON ai_session_share(session_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_session_share_expires ON ai_session_share(status, expires_at);
