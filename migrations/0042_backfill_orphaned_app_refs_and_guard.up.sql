-- 0042 backfill orphaned cloud application references and enforce referential integrity.
-- 背景：0039 将 alert_alert / inspection_policy 中旧格式 application_id 改写为新格式
-- cloud-<prefix>-<hash>，但不创建对应的 asset_application 记录。0032 已删除旧格式应用，
-- 新格式应用仅在云同步（ensureCloudApplication）时创建。迁移完成到人工重新同步之间，
-- alert_alert / inspection_policy 中的 application_id 引用成为孤儿，违反"应用层保证引用完整性"约定。
-- 0040 视图只诊断不修复，0041 只阻断旧格式应用，均不覆盖此场景。
--
-- 本迁移：
-- 1) 为仍被引用但不存在于 asset_application 的新格式 cloud application ID 补建记录，
--    字段与 ensureCloudApplication（internal/asset/application/sync_dto.go）保持一致。
-- 2) 将 v_asset_app_ref_integrity 作为硬验收：补建后若视图仍有孤儿行，迁移失败（CHECK n=0）。
--
-- 幂等：补建使用 INSERT ... ON CONFLICT (application_id) DO NOTHING，已存在的应用不受影响。
-- 兼容仓库自研 SQL splitter：仅普通 DML，无 $$ 美元引用，语句以分号分隔。
-- 依赖 pgcrypto digest()（0039 已 CREATE EXTENSION IF NOT EXISTS）。

CREATE TEMP TABLE _0042_account AS
SELECT ia.account_id,
       ia.provider,
       'cloud-' || left(trim(ia.account_id), 17) || '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12) AS new_app_id
FROM integration_account ia;

INSERT INTO asset_application (application_id, name, environment, namespace, description, created_at, updated_at)
SELECT m.new_app_id,
       m.provider || '-cloud-' || m.account_id,
       'cloud',
       '',
       'Auto-created cloud sync application for account ' || m.account_id,
       NOW(),
       NOW()
FROM _0042_account m
WHERE m.new_app_id <> ''
  AND NOT EXISTS (SELECT 1 FROM asset_application aa WHERE aa.application_id = m.new_app_id)
  AND (
    EXISTS (SELECT 1 FROM alert_alert al WHERE al.application_id = m.new_app_id)
    OR EXISTS (
      SELECT 1
      FROM inspection_policy ip
      CROSS JOIN LATERAL jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(ip.scope->'application_ids') = 'array'
             THEN ip.scope->'application_ids'
             ELSE '[]'::jsonb
        END
      ) AS t(elem)
      WHERE t.elem = m.new_app_id
    )
    OR EXISTS (SELECT 1 FROM asset_resource ar WHERE ar.application_id = m.new_app_id)
    OR EXISTS (SELECT 1 FROM asset_match_rule am WHERE am.application_id = m.new_app_id)
  )
ON CONFLICT (application_id) DO NOTHING;

CREATE TEMP TABLE _0042_guard (n int NOT NULL CHECK (n = 0));

INSERT INTO _0042_guard
SELECT count(*) FROM v_asset_app_ref_integrity;

DROP TABLE _0042_guard;
DROP TABLE _0042_account;
