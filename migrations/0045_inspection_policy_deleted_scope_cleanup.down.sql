-- 0045 down: restore 0040 view definition (without deleted filter).
-- 已清空的 deleted 策略 scope.application_ids 无法从 down 恢复，需从备份还原。

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
WHERE t.elem <> ''
  AND NOT EXISTS (
        SELECT 1 FROM asset_application aa
        WHERE aa.application_id = t.elem
      );

DELETE FROM schema_migrations WHERE version = '0045';
