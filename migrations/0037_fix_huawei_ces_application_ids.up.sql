-- 0037 fix Huawei CES application ID merge semantics.
-- 0032 已发布迁移不可改写；本迁移仅修复历史数据迁移路径中的安全性问题。
-- 目标：当 legacy cloud-<account_id> 与 new cloud-<prefix>-<hash> 应用并存时，不再直接改写业务 ID，
-- 而是先把旧应用引用迁移到新应用、去重子表引用，再删除旧应用；
-- 仅存在旧应用时，允许把旧应用安全重命名为新应用；仅存在新应用时保持幂等。
-- 覆盖：
--   1) only legacy app
--   2) only new app
--   3) legacy + new app both exist
--   4) 子表引用重复去重（resource / match_rule / alert / inspection_policy.scope.application_ids）
-- 依赖 pgcrypto digest()；runner 会逐语句执行。

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TEMP TABLE _0037_account AS
SELECT ia.account_id,
       'cloud-' || trim(ia.account_id) AS old_app_id,
       'cloud-' || left(trim(ia.account_id), 17) || '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12) AS new_app_id
FROM integration_account ia;

CREATE TEMP TABLE _0037_state AS
SELECT a.account_id,
       a.old_app_id,
       a.new_app_id,
       EXISTS (SELECT 1 FROM asset_application aa WHERE aa.application_id = a.old_app_id) AS has_old,
       EXISTS (SELECT 1 FROM asset_application aa WHERE aa.application_id = a.new_app_id) AS has_new
FROM _0037_account a;

-- 1) legacy + new 同时存在：先迁移引用到 new，再删除 old。
UPDATE asset_resource ar
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND s.has_new AND ar.application_id = s.old_app_id;

UPDATE asset_match_rule am
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND s.has_new AND am.application_id = s.old_app_id;

UPDATE alert_alert al
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND s.has_new AND al.application_id = s.old_app_id;

UPDATE inspection_policy ip
SET scope = jsonb_set(
      ip.scope,
      '{application_ids}',
      (
        SELECT COALESCE(jsonb_agg(x.elem ORDER BY x.elem), '[]'::jsonb)
        FROM (
          SELECT DISTINCT COALESCE(s2.new_app_id, elem) AS elem
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
          LEFT JOIN _0037_state s2 ON s2.old_app_id = elem
        ) AS x
      )
    ),
    updated_at = NOW()
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
        JOIN _0037_state s2 ON s2.old_app_id = elem
        WHERE s2.has_old AND s2.has_new
      );

DELETE FROM asset_application aa
USING _0037_state s
WHERE s.has_old AND s.has_new AND aa.application_id = s.old_app_id;

-- 2) only legacy 存在：安全重命名为 new。
UPDATE asset_resource ar
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND NOT s.has_new AND ar.application_id = s.old_app_id;

UPDATE asset_match_rule am
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND NOT s.has_new AND am.application_id = s.old_app_id;

UPDATE alert_alert al
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND NOT s.has_new AND al.application_id = s.old_app_id;

UPDATE inspection_policy ip
SET scope = jsonb_set(
      ip.scope,
      '{application_ids}',
      (
        SELECT COALESCE(jsonb_agg(x.elem ORDER BY x.elem), '[]'::jsonb)
        FROM (
          SELECT DISTINCT COALESCE(s2.new_app_id, elem) AS elem
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
          LEFT JOIN _0037_state s2 ON s2.old_app_id = elem
        ) AS x
      )
    ),
    updated_at = NOW()
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
        JOIN _0037_state s2 ON s2.old_app_id = elem
        WHERE s2.has_old AND NOT s2.has_new
      );

UPDATE asset_application aa
SET application_id = s.new_app_id, updated_at = NOW()
FROM _0037_state s
WHERE s.has_old AND NOT s.has_new AND aa.application_id = s.old_app_id;

DROP TABLE _0037_state;
DROP TABLE _0037_account;
