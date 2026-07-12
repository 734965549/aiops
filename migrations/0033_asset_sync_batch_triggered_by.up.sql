-- 0033 asset sync batch triggered by.
-- 背景：审计此前只在终态写入，进程崩溃后只留 running 批次，缺少触发人审计；
-- 下一次同步 reap 崩溃批次时审计 actor 会落到当次请求用户，无法还原原批次操作者。
-- 解决：为 asset_sync_batch 增加 triggered_by（触发用户 user_id），TriggerSync 创建 running 批次时写入；
-- reap 崩溃批次时 sync_reaped 审计 actor 取该字段，避免归因到当次请求用户。

ALTER TABLE asset_sync_batch
    ADD COLUMN IF NOT EXISTS triggered_by VARCHAR(64) NOT NULL DEFAULT '';

COMMENT ON COLUMN asset_sync_batch.triggered_by IS '同步批次触发用户 user_id；reap 崩溃批次时 sync_reaped 审计 actor 取该字段还原原操作者';
