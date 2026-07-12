-- 回滚 0034：把 eip/bandwidth/subnet/peering 子类型重新合并回 'vpc'。
-- 注意：回滚后语义混合与 ID 碰撞风险恢复；若回填后已新增同名 vpc 行，合并会触发
-- idx_asset_resource_cloud_key 唯一冲突，需先人工合并/删除冲突行。down 脚本仅为人工参考，不由 runner 自动执行。

UPDATE asset_resource
SET cloud_resource_type = 'vpc',
    updated_at = NOW()
WHERE cloud_resource_type IN ('eip', 'bandwidth', 'subnet', 'peering')
  AND source = 'cloud_sync'
  AND labels->>'namespace' = 'SYS.VPC';
