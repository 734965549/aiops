-- 0045 align soft-deleted inspection policies with referential integrity checks.
-- 背景：SoftDelete 仅设置 deleted=true，scope.application_ids 仍保留；ApplicationReferenceChecker
-- 已排除 deleted 策略，DeleteApplication 不再被阻塞，但 v_asset_app_ref_integrity 仍报告孤儿引用。
-- 本迁移：
-- 1) 回填：清空已软删除策略的 scope.application_ids；
-- 2) 重建视图：仅检查 deleted=false 的 inspection_policy。
-- 运行时 SoftDelete 也会清空 application_ids，与 0043 告警解除引用模式一致。

UPDATE inspection_policy
SET scope = jsonb_set(scope, '{application_ids}', '[]'::jsonb, true),
    updated_at = NOW()
WHERE deleted = TRUE
  AND jsonb_typeof(scope->'application_ids') = 'array'
  AND jsonb_array_length(scope->'application_ids') > 0;

CREATE OR REPLACE VIEW v_asset_app_ref_integrity AS
SELECT 'asset_resource' AS source_table,
       ar.resource_id AS ref_id,
       ar.application_id AS orphaned_application_id
FROM asset_resource ar
WHERE ar.application_id <> ''
  AND NOT EXISTS (
        SELECT 1 FROM asset_application aa
        WHERE aa.application_id = ar.application_id
      )
UNION ALL
SELECT 'asset_match_rule' AS source_table,
       am.rule_id AS ref_id,
       am.application_id AS orphaned_application_id
FROM asset_match_rule am
WHERE am.application_id <> ''
  AND NOT EXISTS (
        SELECT 1 FROM asset_application aa
        WHERE aa.application_id = am.application_id
      )
UNION ALL
SELECT 'alert_alert' AS source_table,
       al.alert_id AS ref_id,
       al.application_id AS orphaned_application_id
FROM alert_alert al
WHERE al.application_id <> ''
  AND NOT EXISTS (
        SELECT 1 FROM asset_application aa
        WHERE aa.application_id = al.application_id
      )
UNION ALL
SELECT 'inspection_policy' AS source_table,
       ip.policy_id AS ref_id,
       t.elem AS orphaned_application_id
FROM inspection_policy ip
CROSS JOIN LATERAL jsonb_array_elements_text(
     CASE WHEN jsonb_typeof(ip.scope->'application_ids') = 'array'
          THEN ip.scope->'application_ids'
          ELSE '[]'::jsonb
     END
) AS t(elem)
WHERE ip.deleted = FALSE
  AND t.elem <> ''
  AND NOT EXISTS (
        SELECT 1 FROM asset_application aa
        WHERE aa.application_id = t.elem
      );
