-- 0064: ai_provider 增加 context_window_tokens（模型上下文窗口，单位 token）。
-- 0 表示未配置，对话按 ai.chat_prompt_token_budget 全局预算裁剪；
-- 配置后按 min(窗口 - 输出预留, 全局预算) 约束单次 prompt。
ALTER TABLE ai_provider
    ADD COLUMN IF NOT EXISTS context_window_tokens INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN ai_provider.context_window_tokens IS 'Model context window in tokens; 0 = unset (use global chat prompt token budget)';