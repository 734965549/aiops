-- 0049: AI 运维助手会话与消息（AI 助手多轮对话）
-- 会话与消息均以业务 ID（session_id / message_id）对外，不暴露自增 id；
-- created_at / updated_at 由 Go 程序维护，不使用 DB DEFAULT；不建数据库外键，
-- 会话与消息归属由应用层事务 + 唯一索引保证。
CREATE TABLE IF NOT EXISTS ai_session (
    id           BIGSERIAL    PRIMARY KEY,
    session_id   VARCHAR(64)  NOT NULL,
    user_id      VARCHAR(64)  NOT NULL,
    title        VARCHAR(255) NOT NULL DEFAULT '',
    context_json JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status       VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ  NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_session_session_id ON ai_session (session_id);
CREATE INDEX IF NOT EXISTS idx_ai_session_user_updated ON ai_session (user_id, updated_at DESC);

COMMENT ON TABLE ai_session IS 'AI assistant chat sessions owned by platform users';
COMMENT ON COLUMN ai_session.session_id IS 'Business session identifier exposed via API';
COMMENT ON COLUMN ai_session.context_json IS 'Page context captured when the session was created (ids, time range), never credentials';
COMMENT ON COLUMN ai_session.status IS 'active / archived';

CREATE TABLE IF NOT EXISTS ai_session_message (
    id              BIGSERIAL   PRIMARY KEY,
    message_id      VARCHAR(64) NOT NULL,
    session_id      VARCHAR(64) NOT NULL,
    role            VARCHAR(16) NOT NULL,
    content         TEXT        NOT NULL DEFAULT '',
    tool_calls_json JSONB       NOT NULL DEFAULT '[]'::jsonb,
    references_json JSONB       NOT NULL DEFAULT '[]'::jsonb,
    feedback        VARCHAR(16) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_session_message_message_id ON ai_session_message (message_id);
CREATE INDEX IF NOT EXISTS idx_ai_session_message_session_created ON ai_session_message (session_id, created_at ASC);

COMMENT ON TABLE ai_session_message IS 'AI assistant chat messages; assistant content holds the structured answer JSON';
COMMENT ON COLUMN ai_session_message.role IS 'user / assistant';
COMMENT ON COLUMN ai_session_message.tool_calls_json IS 'Tool permission checks and results (no credentials or prompts)';
COMMENT ON COLUMN ai_session_message.references_json IS 'Evidence references surfaced to the user';
COMMENT ON COLUMN ai_session_message.feedback IS 'empty / valid / invalid';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000048', 'app:ai:chat', 'Chat with AI assistant', 'ai', 'chat', 'Create AI assistant sessions and send messages', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000049', 'app:ai:chat:feedback', 'Feedback AI assistant answers', 'ai', 'chat:feedback', 'Mark AI assistant answers as valid or invalid', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN ('app:ai:chat', 'app:ai:chat:feedback')
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
