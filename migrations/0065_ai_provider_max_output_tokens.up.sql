-- 0065: ai_provider 增加 max_output_tokens（单次模型输出上限，单位 token）。
-- 0 表示未配置，对话/分析请求按 ai.chat_max_output_tokens 全局默认下发 max_tokens；
-- 配置窗口时输出上限不超过窗口的一半，剩余空间留给输入 prompt。
ALTER TABLE ai_provider
    ADD COLUMN IF NOT EXISTS max_output_tokens INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN ai_provider.max_output_tokens IS 'Model output token limit sent as max_tokens; 0 = unset (use global chat max output tokens)';