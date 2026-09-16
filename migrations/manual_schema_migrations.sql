-- =============================================================================
-- !! 已废弃（DEPRECATED）— 禁止手工执行 !!
--
-- 本文件为历史遗留兜底脚本，schema 已与 runner 不一致（缺 checksum 列、版本仅至 0029）。
-- 生产 / 预发 / dev / test 一律使用自研 runner（cmd/migrate 或 make migrate），
-- 禁止手工 psql -f 本文件或手工写入 schema_migrations。
-- 详见 ops/migration-contract.md 与 migrations/README.md。
-- =============================================================================
--
-- 只供 DBA 手工执行模式使用。（已废弃，见上方说明）
--
-- 先按 migrations/README.md 列出嘅顺序成功执行所有 migrations/*.up.sql，
-- 再执行呢个文件补齐 schema_migrations 账本。
--
-- 唔好用呢个文件掩盖未执行嘅建表或种子数据；佢只记录 DBA 已经套用对应版本 SQL。

CREATE TABLE IF NOT EXISTS public.schema_migrations (
  version    VARCHAR(64) PRIMARY KEY,
  name       VARCHAR(255) NOT NULL,
  applied_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

INSERT INTO public.schema_migrations (version, name, applied_at) VALUES
('0001', 'init_identity', NOW()),
('0002', 'seed_admin_permissions', NOW()),
('0003', 'external_identity', NOW()),
('0004', 'user_provisioning_permissions', NOW()),
('0005', 'user_role_source', NOW()),
('0006', 'auth_audit', NOW()),
('0007', 'init_alert', NOW()),
('0008', 'init_asset', NOW()),
('0009', 'init_audit', NOW()),
('0010', 'ai_analyze_permission', NOW()),
('0011', 'init_execution', NOW()),
('0012', 'init_runbook', NOW()),
('0013', 'dashboard_permission', NOW()),
('0014', 'init_asset_match_rule', NOW()),
('0015', 'identity_access_control_management', NOW()),
('0016', 'seed_default_admin_user', NOW()),
('0017', 'repair_default_admin_superset', NOW()),
('0018', 'init_integration', NOW()),
('0019', 'init_observability', NOW()),
('0020', 'init_inspection', NOW()),
('0022', 'init_execution_agent', NOW()),
('0023', 'asset_cloud_sync', NOW()),
('0024', 'integration_account_extra_config', NOW()),
('0025', 'asset_resource_labels', NOW()),
('0026', 'asset_cloud_sync_region_key', NOW()),
('0027', 'asset_sync_batch_message_text', NOW()),
('0028', 'asset_sync_batch_running_mutex', NOW()),
('0029', 'huawei_legacy_accounts_native_sync_mode', NOW())
ON CONFLICT (version) DO NOTHING;
