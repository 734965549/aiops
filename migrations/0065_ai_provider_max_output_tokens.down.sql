-- 0065 回滚参考：移除 max_output_tokens 列（人工执行）。
ALTER TABLE ai_provider DROP COLUMN IF EXISTS max_output_tokens;