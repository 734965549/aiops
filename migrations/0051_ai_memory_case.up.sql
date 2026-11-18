-- 0051: AI 助手相似案例记忆（阶段 1，RAG-lite）
-- 来源：feedback=valid 且未降级的助手回答；按 source_message_id 幂等沉淀。
-- 约束：id BIGSERIAL 主键 + case_id 业务唯一列；时间戳由 Go 维护；不加外键；不存凭据与告警正文。
CREATE TABLE IF NOT EXISTS ai_memory_case (
    id                BIGSERIAL    PRIMARY KEY,
    case_id           VARCHAR(64)  NOT NULL,
    source_type       VARCHAR(32)  NOT NULL,
    source_message_id VARCHAR(64)  NOT NULL DEFAULT '',
    session_id        VARCHAR(64)  NOT NULL DEFAULT '',
    user_id           VARCHAR(64)  NOT NULL,
    application_id    VARCHAR(64)  NOT NULL DEFAULT '',
    resource_id       VARCHAR(128) NOT NULL DEFAULT '',
    resource_type     VARCHAR(64)  NOT NULL DEFAULT '',
    alert_signature   VARCHAR(255) NOT NULL DEFAULT '',
    keywords          TEXT         NOT NULL DEFAULT '',
    summary           TEXT         NOT NULL DEFAULT '',
    recommendations   TEXT         NOT NULL DEFAULT '',
    risk_level        VARCHAR(16)  NOT NULL DEFAULT '',
    quality_score     INT          NOT NULL DEFAULT 0,
    status            VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at        TIMESTAMPTZ  NOT NULL,
    updated_at        TIMESTAMPTZ  NOT NULL
);

COMMENT ON TABLE ai_memory_case IS 'AI assistant similar-case memory built from valid feedback (stage 1, RAG-lite)';
COMMENT ON COLUMN ai_memory_case.case_id IS 'Business case identifier exposed to API and references';
COMMENT ON COLUMN ai_memory_case.source_type IS 'chat_message / alert_analysis / knowledge_item';
COMMENT ON COLUMN ai_memory_case.user_id IS 'Owner; retrieval is always filtered by the asking user';
COMMENT ON COLUMN ai_memory_case.alert_signature IS 'Alert fingerprint: alert_name + metric + dimension hash';
COMMENT ON COLUMN ai_memory_case.keywords IS 'Whitespace-separated retrieval keywords extracted from summary/recommendations';
COMMENT ON COLUMN ai_memory_case.quality_score IS 'Feedback aggregate score; at or below the retire threshold the case stops matching';
COMMENT ON COLUMN ai_memory_case.status IS 'active / retired';

CREATE UNIQUE INDEX IF NOT EXISTS uk_ai_memory_case_case_id ON ai_memory_case (case_id);

-- 同一来源消息只沉淀一条案例；空串（历史/非反馈来源）不参与唯一约束。
CREATE UNIQUE INDEX IF NOT EXISTS uk_ai_memory_case_source
    ON ai_memory_case (user_id, source_message_id)
    WHERE source_message_id <> '';

CREATE INDEX IF NOT EXISTS idx_ai_memory_case_signature ON ai_memory_case (alert_signature);
CREATE INDEX IF NOT EXISTS idx_ai_memory_case_owner ON ai_memory_case (user_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_memory_case_keywords
    ON ai_memory_case USING gin (to_tsvector('simple', keywords));
