-- 0057 回滚参考：移除 reasoning_effort 列（人工执行）。
ALTER TABLE ai_provider DROP COLUMN IF EXISTS reasoning_effort;