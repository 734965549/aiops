-- 0064 回滚参考：移除 context_window_tokens 列（人工执行）。
ALTER TABLE ai_provider DROP COLUMN IF EXISTS context_window_tokens;