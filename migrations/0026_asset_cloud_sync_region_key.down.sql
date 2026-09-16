-- 回滚 0026：恢复不含 region 的唯一索引（多区域同类型同 ID 将再次互相覆盖）。
-- 注意：若 0026 应用后已产生跨 region 相同 cloud_resource_id 的数据，重建旧唯一索引会因
-- 重复键失败。此时不得直接回滚，需先人工合并/删除冲突行（保留每个 account+type+id 的最新记录），
-- 或放弃回滚保持 0026 的 region-aware 索引。down 脚本仅为人工参考，不由 runner 自动执行。

DROP INDEX IF EXISTS idx_asset_resource_cloud_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_resource_cloud_key
    ON asset_resource(integration_account_id, cloud_resource_type, cloud_resource_id)
    WHERE source = 'cloud_sync' AND cloud_resource_id <> '';
