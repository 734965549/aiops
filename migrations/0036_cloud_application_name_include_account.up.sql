-- 0036 update cloud sync application name to include account_id for uniqueness.
-- 背景：ensureCloudApplication 之前用 Name = "<provider>-cloud"，所有同 provider 账号共用同一名称。
-- FindByNameEnv(name, env) 取第一条，多账号场景告警默认匹配存在歧义。
-- 修复：Name 改为 "<provider>-cloud-<account_id>"，保证每个账号的应用名称唯一。
-- 本迁移从 description 提取 account_id，追加到现有 name。
-- 幂等：name NOT LIKE '%-cloud-%' 确保不重复追加。

UPDATE asset_application
SET name    = name || '-' || regexp_replace(description, '^Auto-created cloud sync application for account ', ''),
    updated_at = NOW()
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name NOT LIKE '%-cloud-%';
