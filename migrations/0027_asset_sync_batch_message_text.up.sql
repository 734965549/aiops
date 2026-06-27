-- 0027 enlarge asset_sync_batch.message to TEXT, see docs/huawei-ces-asset-sync-plan.md §P0.
-- 0023 建表时 message 为 VARCHAR(512)，但应用层 sync_service.go 截断到 2000 rune，
-- 多区域多失败场景下最终 UPDATE 会因列长度溢出失败，导致资源已写入但批次卡 running、审计未写。
-- 改为 TEXT 以容纳完整诊断摘要（ces_total/discovered/failed_scopes/enriched/enrichment_failed）。

ALTER TABLE asset_sync_batch
    ALTER COLUMN message TYPE TEXT;
