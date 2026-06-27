-- 回滚 0028：移除账号级 running 互斥。注意：回滚后同账号并发同步会再次出现互相标记 stale 的竞态。

DROP INDEX IF EXISTS idx_asset_sync_batch_running_account;

ALTER TABLE asset_sync_batch
    DROP COLUMN IF EXISTS lease_expires_at;
