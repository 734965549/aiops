-- 0028 account-level sync mutex via partial unique index + lease, see docs/huawei-ces-asset-sync-plan.md §P1.
-- 问题：TriggerSync 创建 running 批次无账号级互斥，同一账号并发批次交错时，
-- A 会把 B 刚 upsert 的资源（sync_batch_id=B）标记为 stale，产生错误资产状态。
-- 解决：每个账号同一时刻只允许一个 running 批次；崩溃批次靠 lease_expires_at 超时自愈。
-- 依赖：仅 PostgreSQL 部分唯一索引，不依赖 Redis，与 redis.required=false 部署姿态一致。

-- 1. 新增租约过期时间列；running 批次写入 now+TTL，终态批次由应用层清空。
ALTER TABLE asset_sync_batch
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ NULL;

-- 2. 建索引前先清理历史残留 running 批次，避免每个账号存在多个 running 行导致建索引失败。
--    这些批次多半是进程崩溃遗留，无法继续推进，统一标记 failed 释放槽位。
UPDATE asset_sync_batch
SET status        = 'failed',
    finished_at   = COALESCE(finished_at, NOW()),
    message       = LEFT(COALESCE(NULLIF(message, ''), '') || ' [batch interrupted before lease enforcement]', 512),
    updated_at    = NOW()
WHERE status = 'running';

-- 3. 部分唯一索引：同一账号仅允许一个 running 批次。
--    Create 冲突由应用层 MapUniqueViolation 映射为 ErrAlreadyExists -> HTTP 409。
CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_sync_batch_running_account
    ON asset_sync_batch(integration_account_id)
    WHERE status = 'running';

COMMENT ON COLUMN asset_sync_batch.lease_expires_at IS 'running 批次租约过期时间；超时后由下一次同步 reap 为 failed；终态批次为 NULL';
