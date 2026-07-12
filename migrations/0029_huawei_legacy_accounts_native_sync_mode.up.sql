-- 0029 backfill legacy huawei accounts to sync_mode=native.
-- 背景：0024 给所有账号写入 {}，而配置解析（sync_mode_config.go）把空配置解释为 ces，
-- 导致历史华为账号升级后立即从 native 切到 CES，违反 ops/huawei-ces-sync-contract.md §11/§18 灰度策略
-- （已有账号应保持 native，灰度切换；新账号才默认 ces）。
-- 本迁移把仍为空配置的华为账号显式置为 native，保持升级前后行为不变；
-- 新账号由创建逻辑 encodeExtraConfigInput 显式写 ces，不再依赖解析器默认值。
-- 仅命中 extra_config 仍为 {} 的华为账号；已显式配置过其它 sync_mode 的账号不受影响。
UPDATE integration_account
SET extra_config = '{"sync_mode":"native"}'::jsonb,
    updated_at   = NOW()
WHERE provider = 'huawei_cloud'
  AND extra_config = '{}'::jsonb;
