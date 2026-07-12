-- 0040 create persistent view for application_id referential integrity diagnostics.
-- 背景：0032 破坏性 DELETE 删除旧格式应用后，0039 将 alert_alert / inspection_policy 中
-- 旧格式引用改写为新格式 cloud-<prefix>-<hash>，但不校验新格式应用是否已在 asset_application 中存在。
-- 若重新同步尚未执行，新格式应用不存在，引用成为孤儿。
-- 本迁移创建持久视图 v_asset_app_ref_integrity，暴露 asset_resource / asset_match_rule /
-- alert_alert / inspection_policy 中指向不存在 application_id 的引用，
-- 供运维和发布流水线查询验证。
-- 视图不修改数据，不阻断迁移执行。
-- 验证方式：SELECT * FROM v_asset_app_ref_integrity; 期望 0 行（重新同步后）。
-- 兼容仓库自研 SQL splitter：仅普通 DDL，无 $$ 美元引用，语句以分号分隔。

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
