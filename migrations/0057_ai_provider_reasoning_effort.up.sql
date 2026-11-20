-- 0057: ai_provider 增加 reasoning_effort（思考型模型推理强度档位）。
-- 空串表示不发送该参数；仅 low/high/max 由应用层校验。
ALTER TABLE ai_provider
    ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(16) NOT NULL DEFAULT '';