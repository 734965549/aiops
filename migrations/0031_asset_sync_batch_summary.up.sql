-- 0031 asset sync batch structured summary.
-- 背景：批次详情页不应继续把 message 当作半结构化协议解析。
-- 解决：为 asset_sync_batch 增加 summary JSONB，保存同步摘要 DTO；message 仅保留人类可读排查说明。

ALTER TABLE asset_sync_batch
    ADD COLUMN IF NOT EXISTS summary JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN asset_sync_batch.summary IS '同步批次结构化摘要 DTO；message 仅作为人类可读排查说明';
