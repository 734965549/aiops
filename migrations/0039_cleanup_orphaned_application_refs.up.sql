-- 0039 cleanup orphaned application_id references left by 0032 DELETE.
-- 背景：0032 是破坏性清理脚本，删除旧格式 cloud-<account_id> 应用及其关联的
-- asset_resource / asset_match_rule，但不处理 alert_alert.application_id 和
-- inspection_policy.scope.application_ids 中的旧格式引用。
-- 0037 修复路径依赖 has_old（旧应用仍在 asset_application 中），若 0032 已删除
-- 旧应用，则 has_old=false，0037 跳过这两个表，留下孤儿引用。
-- 本迁移补全：按 integration_account 计算 old->new 映射，将 alert_alert 和
-- inspection_policy 中仍残留的旧格式 application_id 改写为新格式。
-- 幂等：已经是新格式或已由 0037 处理过的行不受影响（WHERE 不匹配）。
-- 依赖 pgcrypto digest()；CREATE EXTENSION IF NOT EXISTS 幂等。
-- 兼容仓库自研 SQL splitter：仅普通 DML，无 $$ 美元引用，语句以分号分隔。

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TEMP TABLE _0039_account AS
SELECT ia.account_id,
       'cloud-' || trim(ia.account_id) AS old_app_id,
       'cloud-' || left(trim(ia.account_id), 17) || '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12) AS new_app_id
FROM integration_account ia;

-- alert_alert: 旧格式孤儿引用改写为新格式。
UPDATE alert_alert al
SET application_id = m.new_app_id,
    updated_at = NOW()
FROM _0039_account m
WHERE al.application_id = m.old_app_id;

-- inspection_policy.scope.application_ids: 旧格式元素替换为新格式，DISTINCT 去重。
UPDATE inspection_policy ip
SET scope = jsonb_set(
      ip.scope,
      '{application_ids}',
      (
        SELECT COALESCE(jsonb_agg(x.elem ORDER BY x.elem), '[]'::jsonb)
        FROM (
          SELECT DISTINCT COALESCE(m.new_app_id, elem) AS elem
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
          LEFT JOIN _0039_account m ON m.old_app_id = elem
        ) AS x
      )
    ),
    updated_at = NOW()
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
        JOIN _0039_account m ON m.old_app_id = elem
      );

DROP TABLE _0039_account;
