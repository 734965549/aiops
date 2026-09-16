-- 0040 down: drop the referential integrity diagnostic view.
-- 视图不修改任何业务数据，删除即可回滚。
DROP VIEW IF EXISTS v_asset_app_ref_integrity;
