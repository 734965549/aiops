-- 0052: AI 助手知识沉淀（阶段 2）
-- 反馈聚合或人工录入的 draft 条目，必须经管理员激活后才注入回答；
-- 条目只存摘要与业务 ID，禁止凭据与告警正文原文。
CREATE TABLE IF NOT EXISTS ai_knowledge_item (
    id           BIGSERIAL    PRIMARY KEY,
    item_id      VARCHAR(64)  NOT NULL,
    scope        VARCHAR(32)  NOT NULL,
    scope_ref    VARCHAR(128) NOT NULL DEFAULT '',
    title        VARCHAR(255) NOT NULL,
    content      TEXT         NOT NULL DEFAULT '',
    origin       VARCHAR(32)  NOT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'draft',
    hit_count    INT          NOT NULL DEFAULT 0,
    useful_count INT          NOT NULL DEFAULT 0,
    reviewed_by  VARCHAR(64)  NOT NULL DEFAULT '',
    reviewed_at  TIMESTAMPTZ,
    created_by   VARCHAR(64)  NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL
);

COMMENT ON TABLE ai_knowledge_item IS 'AI assistant knowledge items reviewed by admins (stage 2)';
COMMENT ON COLUMN ai_knowledge_item.item_id IS 'Business identifier exposed to API and references';
COMMENT ON COLUMN ai_knowledge_item.scope IS 'global / application / resource_type';
COMMENT ON COLUMN ai_knowledge_item.scope_ref IS 'Scope value (application_id or resource type); empty for global';
COMMENT ON COLUMN ai_knowledge_item.origin IS 'feedback_aggregate / manual';
COMMENT ON COLUMN ai_knowledge_item.status IS 'draft / active / disabled; only active items are injected';
COMMENT ON COLUMN ai_knowledge_item.hit_count IS 'Injection count used for efficiency evaluation';
COMMENT ON COLUMN ai_knowledge_item.useful_count IS 'Feedback=valid count on answers that injected this item';
COMMENT ON COLUMN ai_knowledge_item.reviewed_by IS 'Admin user_id that last reviewed the item';

CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_knowledge_item_item_id ON ai_knowledge_item (item_id);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_item_status_scope
    ON ai_knowledge_item (status, scope, scope_ref, updated_at DESC);

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000052', 'app:ai:knowledge:manage', 'Manage AI knowledge items', 'ai', 'knowledge:manage',
     'Create, review, activate and disable AI knowledge items', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code = 'app:ai:knowledge:manage'
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
