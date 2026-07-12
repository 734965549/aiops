-- 0034 split SYS.VPC cloud_sync resources into sub-types by dim_name,
-- see ops/huawei-ces-sync-contract.md §9.3 与 docs/huawei-ces-sync-backlog.md 缺口4。
-- CES SYS.VPC 是网络资源聚合 namespace，其主维度为 publicip_id/bandwidth_id/subnet_id/peering_id
-- （参考 https://support.huaweicloud.com/eu/usermanual-ces/en-us_topic_0202622212.html）。
-- 此前这些子资源统一入库为 cloud_resource_type='vpc'，存在语义混合与 ID 碰撞风险。
-- 本迁移按 labels->>'dim_name' 把存量 'vpc' 行回填为 eip/bandwidth/subnet/peering，未知 dim 保留 vpc。
-- 仅处理 source='cloud_sync' 且 labels.namespace='SYS.VPC' 的行；native VPC 实体（无 namespace label）不受影响。
-- 不同 dim_name 的 cloud_resource_id（主维度 UUID）不同，回填不会触发 idx_asset_resource_cloud_key 唯一冲突。

UPDATE asset_resource
SET cloud_resource_type = CASE labels->>'dim_name'
        WHEN 'publicip_id'  THEN 'eip'
        WHEN 'bandwidth_id' THEN 'bandwidth'
        WHEN 'subnet_id'    THEN 'subnet'
        WHEN 'peering_id'   THEN 'peering'
        WHEN 'vpc_id'       THEN 'vpc'
        ELSE cloud_resource_type
    END,
    updated_at = NOW()
WHERE cloud_resource_type = 'vpc'
  AND source = 'cloud_sync'
  AND labels->>'namespace' = 'SYS.VPC';
