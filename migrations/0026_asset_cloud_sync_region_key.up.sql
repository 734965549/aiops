-- 0026 make cloud resource unique key region-aware, see ops/huawei-ces-sync-contract.md §9.1.
-- 旧唯一索引 (integration_account_id, cloud_resource_type, cloud_resource_id) 不含 region，
-- 多区域同类型同云 ID 资源会互相覆盖；重建为含 region 的部分唯一索引。
-- 既有数据按旧索引去重，每个 (account, type, id) 仅一行，重建不会触发唯一冲突。

DROP INDEX IF EXISTS idx_asset_resource_cloud_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_resource_cloud_key
    ON asset_resource(integration_account_id, cloud_resource_type, cloud_resource_id, region)
    WHERE source = 'cloud_sync' AND cloud_resource_id <> '';
