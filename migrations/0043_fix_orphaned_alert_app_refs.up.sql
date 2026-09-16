-- 0043 fix orphaned alert application references.
-- 背景：DeleteApplication 历史上仅检查 asset_resource / asset_match_rule 引用，
-- 未检查 alert_alert.application_id 和 inspection_policy.scope->'application_ids'。
-- 修复后 DeleteApplication 已增加跨上下文引用检查（ApplicationReferenceChecker），
-- 但本地已存在 2 条孤儿告警引用：alert_alert.application_id 指向不存在的应用。
--
-- 本迁移：
-- 1) 将 v_asset_app_ref_integrity 视图中 alert_alert 来源的孤儿引用置空 application_id，
--    使视图返回 0 行（release-checklist §2.6 硬验收要求）。
-- 2) 同步清理 inspection_policy 中可能存在的孤儿 application_ids 元素。
-- 3) 硬验收：修复后 v_asset_app_ref_integrity 必须返回 0 行。
--
-- 幂等：仅更新 application_id 不在 asset_application 中的行，已修复的行不受影响。
-- 兼容仓库自研 SQL splitter：仅普通 DML，无 $$ 美元引用，语句以分号分隔。

-- 1) 清理 alert_alert 中的孤儿 application_id 引用
UPDATE alert_alert al
SET application_id = '',
    application_name = '',
    updated_at = NOW()
WHERE al.application_id <> ''
  AND NOT EXISTS (
    SELECT 1 FROM asset_application aa WHERE aa.application_id = al.application_id
  );

-- 2) 清理 inspection_policy.scope.application_ids 中的孤儿元素
UPDATE inspection_policy ip
SET scope = jsonb_set(
      ip.scope,
      '{application_ids}',
      (
        SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
        FROM jsonb_array_elements_text(
          CASE WHEN jsonb_typeof(ip.scope->'application_ids') = 'array'
               THEN ip.scope->'application_ids'
               ELSE '[]'::jsonb
          END
        ) AS t(elem)
        WHERE EXISTS (
          SELECT 1 FROM asset_application aa WHERE aa.application_id = t.elem
        )
      ),
      true
    ),
    updated_at = NOW()
WHERE ip.deleted = FALSE
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements_text(
      CASE WHEN jsonb_typeof(ip.scope->'application_ids') = 'array'
           THEN ip.scope->'application_ids'
           ELSE '[]'::jsonb
      END
    ) AS t(elem)
    WHERE t.elem <> ''
      AND NOT EXISTS (
        SELECT 1 FROM asset_application aa WHERE aa.application_id = t.elem
      )
  );

-- 3) 硬验收：修复后视图必须返回 0 行
CREATE TEMP TABLE _0043_guard (n int NOT NULL CHECK (n = 0));

INSERT INTO _0043_guard
SELECT count(*) FROM v_asset_app_ref_integrity;

DROP TABLE _0043_guard;
