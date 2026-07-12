-- 0035 rewrite byte-truncated cloud application IDs to rune-based form for non-ASCII accounts.
-- 背景：cloudApplicationID 旧实现按字节截取 account_id 前 17 字节（Go id[:17]），而迁移 0032 与新格式
-- 契约使用 PostgreSQL left(...,17) 按字符截取。对含多字节字符（中文等）且字节长度 > 17 的账号：
--   - 旧字节版可能截断 UTF-8 字符中部 -> 非法 UTF-8，UTF8 编码库会拒绝写入（此类行不存在）；
--   - 当 17 字节边界恰好落在字符边界时，字节版产出有效但字符数 < 17 的前缀，可写入但与 left() 的
--     17 字符前缀不一致。两种版本共享同一 sha1 后缀，但 application_id 不同 -> 同账号云资源被拆散到
--     两个应用，破坏资产匹配闭环。
-- 修复后 Go 端改按 rune 截取（见 internal/asset/application/sync_service.go:cloudApplicationID），
-- 本迁移把存量字节版 application_id 无损改写为 rune 版。
--
-- 策略（无损精确改写，生产/预发/开发统一适用）：按 application_id 的 sha1 后缀
-- （恒为 '-' || substr(sha1hex(trim(account_id)),1,12)，字节版与 rune 版一致）关联 integration_account
-- 识别受影响应用；对每个账号计算 rune 版新 ID（'cloud-' || left(trim(account_id),17) || '-' || 后缀），
-- 把旧字节版 application_id 及 asset_resource / asset_match_rule / alert_alert /
-- inspection_policy.scope.application_ids 引用改写为新 ID；新旧应用并存时按云资源唯一键合并资源/规则到
-- 新应用并删除空的旧应用（避免撞 asset_application.application_id 唯一约束）。
-- 仅影响 cloud- 前缀且后缀匹配的应用；纯 ASCII 账号字节版=rune 版，无改写。
-- 依赖 pgcrypto 提供 digest()；CREATE EXTENSION IF NOT EXISTS 幂等，生产由 DBA 执行。
-- 兼容仓库自研 SQL splitter：仅普通 DML，无 $$ 美元引用，语句以分号分隔。

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 受影响账号的 sha1 后缀与 rune 版新 ID。
CREATE TEMP TABLE _0035_account AS
SELECT ia.account_id,
       substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12) AS hash_suffix,
       'cloud-' || left(trim(ia.account_id), 17) || '-' ||
         substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12) AS new_app_id
FROM integration_account ia;

-- 可改写集合：旧字节版应用存在且 rune 版新应用不存在（任意写操作前快照，判定稳定）。
-- 旧应用 = cloud- 前缀 + 后缀匹配，且不等于 rune 版新 ID，且不等于 legacy 'cloud-' || account_id（0032 已处理，此处防御）。
CREATE TEMP TABLE _0035_rewrite AS
SELECT aa.application_id AS old_app_id, m.new_app_id
FROM asset_application aa
JOIN _0035_account m ON right(aa.application_id, 13) = '-' || m.hash_suffix
WHERE aa.application_id LIKE 'cloud-%'
  AND aa.application_id <> m.new_app_id
  AND aa.application_id <> 'cloud-' || m.account_id
  AND NOT EXISTS (
        SELECT 1 FROM asset_application aa2
        WHERE aa2.application_id = m.new_app_id
      );

-- 先改写引用表（asset_application 改写前，新应用仍不存在，NOT EXISTS 判定有效）。
UPDATE asset_match_rule am
SET application_id = r.new_app_id, updated_at = NOW()
FROM _0035_rewrite r
WHERE am.application_id = r.old_app_id;

UPDATE asset_resource ar
SET application_id = r.new_app_id, updated_at = NOW()
FROM _0035_rewrite r
WHERE ar.application_id = r.old_app_id;

UPDATE alert_alert al
SET application_id = r.new_app_id, updated_at = NOW()
FROM _0035_rewrite r
WHERE al.application_id = r.old_app_id;

-- 改写应用本身。
UPDATE asset_application aa
SET application_id = r.new_app_id, updated_at = NOW()
FROM _0035_rewrite r
WHERE aa.application_id = r.old_app_id;

-- inspection_policy.scope.application_ids JSONB 改写：把数组中旧格式元素替换为新格式，
-- DISTINCT 去重避免旧字节版与新 rune 版同时存在时替换后产生重复 ID（与 0032/0037 一致）。
UPDATE inspection_policy ip
SET scope = jsonb_set(
      ip.scope,
      '{application_ids}',
      (
        SELECT COALESCE(jsonb_agg(x.elem ORDER BY x.elem), '[]'::jsonb)
        FROM (
          SELECT DISTINCT COALESCE(r.new_app_id, elem) AS elem
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
          LEFT JOIN _0035_rewrite r ON r.old_app_id = elem
        ) AS x
      )
    ),
    updated_at = NOW()
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
        JOIN _0035_rewrite r ON r.old_app_id = elem
      );

-- 合并集合：旧字节版应用存在且 rune 版新应用已存在 -> 将旧应用的资源/规则改写指向新应用，再删除空的旧应用。
-- asset_resource 云资源唯一键 (integration_account_id, cloud_resource_type, cloud_resource_id, region) 不含
-- application_id；asset_match_rule 唯一约束仅在 rule_id；二者改写不会触发唯一冲突。
CREATE TEMP TABLE _0035_merge AS
SELECT aa.application_id AS old_app_id, m.new_app_id
FROM asset_application aa
JOIN _0035_account m ON right(aa.application_id, 13) = '-' || m.hash_suffix
WHERE aa.application_id LIKE 'cloud-%'
  AND aa.application_id <> m.new_app_id
  AND aa.application_id <> 'cloud-' || m.account_id
  AND EXISTS (
        SELECT 1 FROM asset_application aa2
        WHERE aa2.application_id = m.new_app_id
      );

-- 匹配规则迁移到新应用（rule_id 不变，不会冲突）。
UPDATE asset_match_rule am
SET application_id = mg.new_app_id, updated_at = NOW()
FROM _0035_merge mg
WHERE am.application_id = mg.old_app_id;

-- 资源合并到新应用（resource_id 不变，cloud_key 唯一索引不含 application_id，不会冲突）。
UPDATE asset_resource ar
SET application_id = mg.new_app_id, updated_at = NOW()
FROM _0035_merge mg
WHERE ar.application_id = mg.old_app_id;

-- 告警引用统一指向新应用，避免随旧应用删除而孤立。
UPDATE alert_alert al
SET application_id = mg.new_app_id, updated_at = NOW()
FROM _0035_merge mg
WHERE al.application_id = mg.old_app_id;

-- 资源和规则已迁移，删除空的旧应用。
DELETE FROM asset_application aa USING _0035_merge mg WHERE aa.application_id = mg.old_app_id;

-- inspection_policy.scope.application_ids JSONB 合并改写（旧 -> 新），
-- DISTINCT 去重避免合并后产生重复 ID（与 0032/0037 一致）。
UPDATE inspection_policy ip
SET scope = jsonb_set(
      ip.scope,
      '{application_ids}',
      (
        SELECT COALESCE(jsonb_agg(x.elem ORDER BY x.elem), '[]'::jsonb)
        FROM (
          SELECT DISTINCT COALESCE(mg.new_app_id, elem) AS elem
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
          LEFT JOIN _0035_merge mg ON mg.old_app_id = elem
        ) AS x
      )
    ),
    updated_at = NOW()
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
        JOIN _0035_merge mg ON mg.old_app_id = elem
      );

DROP TABLE _0035_rewrite;
DROP TABLE _0035_merge;
DROP TABLE _0035_account;
