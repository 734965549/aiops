-- 0032 cleanup legacy cloud application IDs.
-- 背景：cloudApplicationID 从旧格式 cloud-<account_id> 改为新格式 cloud-<account_prefix>-<hash>，
-- 旧格式与新格式不兼容，会导致同一账号的旧资源与新资源被拆散到不同应用，破坏资产匹配闭环。
-- 策略：开发/未上线阶段允许清空重来，本迁移删除所有旧格式云同步应用及其关联资源、匹配规则。
-- 保留 integration_account，升级后需重新触发云同步（无需重新录入账号）。
--
-- 识别方式：按 application_id = 'cloud-' || trim(account_id) 精确关联 integration_account，
-- 覆盖 account_id 含连字符（如 acc-<uuid>）与不含连字符（如华为云纯数字账号）的所有情况。
-- 新格式 application_id = cloud-<left(account_id,17)>-<sha1_12hex>，比旧格式多一个 '-<hash>' 后缀，
-- 精确匹配不会误删新格式应用。
-- 注意：本迁移不处理 alert_alert / inspection_policy 中的旧格式引用，由 0039 补全清理。
-- 为兼容仓库自研 SQL splitter（不支持 $$ 美元引用块），本脚本使用普通 DML 完成清理。

DELETE FROM asset_match_rule
WHERE application_id IN (
    SELECT 'cloud-' || trim(ia.account_id) AS old_app_id
    FROM integration_account ia
);

DELETE FROM asset_resource
WHERE application_id IN (
    SELECT 'cloud-' || trim(ia.account_id) AS old_app_id
    FROM integration_account ia
);

DELETE FROM asset_application
WHERE application_id IN (
    SELECT 'cloud-' || trim(ia.account_id) AS old_app_id
    FROM integration_account ia
);
