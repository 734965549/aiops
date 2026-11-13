-- 0048: AI provider 持久化配置（平台直连模型）
-- 业务标识 provider_id 唯一；api_key 仅保存 AES-GCM 密文，明文只在运行时解密；
-- created_at / updated_at 由 Go 程序维护，不使用 DB DEFAULT；不建数据库外键。
CREATE TABLE IF NOT EXISTS ai_provider (
    id BIGSERIAL PRIMARY KEY,
    provider_id VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(16) NOT NULL,
    base_url VARCHAR(512) NOT NULL,
    model VARCHAR(128) NOT NULL DEFAULT '',
    api_key_encrypted BYTEA,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    timeout_ms BIGINT NOT NULL DEFAULT 30000,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    description VARCHAR(512) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ai_provider_provider_id ON ai_provider (provider_id);
