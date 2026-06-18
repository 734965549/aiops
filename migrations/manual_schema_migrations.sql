-- DBA manual mode only.
--
-- Run this file after every migrations/*.up.sql file listed in migrations/README.md
-- has been executed successfully against the target database.
--
-- Do not use this file to hide missing table/seed changes. It only records that
-- DBA has already applied the corresponding versioned SQL files.

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
('0016', 'seed_default_admin_user', NOW())
ON CONFLICT (version) DO NOTHING;
