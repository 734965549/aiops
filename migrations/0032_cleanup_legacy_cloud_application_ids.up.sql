-- 0032 cleanup legacy cloud application IDs.
-- 背景：cloudApplicationID 从旧格式 cloud-<account_id> 改为新格式 cloud-<account_prefix>-<hash>，
-- 旧格式与新格式不兼容，会导致同一账号的旧资源与新资源被拆散到不同应用，破坏资产匹配闭环。
-- 策略：开发/未上线阶段允许清空重来，本迁移删除所有旧格式云同步应用及其关联资源、匹配规则。
-- 升级后需要重新录入华为云集成账号并执行同步。
-- 新格式特征：cloud-<最多17位账号前缀>-<12位sha1哈希>，至少包含两个'-'。
-- 旧格式特征：cloud-<完整账号>，通常只包含一个'-'（账号本身不含'-'）。
-- 注意：若历史账号本身包含'-'，旧格式 cloud-<账号> 会与新格式无法区分；现实中华为云账号ID为
-- 纯数字/字母，不会出现此歧义。本迁移按"cloud- 开头且不含第二个 '-'"识别旧格式。
-- 为兼容仓库自研 SQL splitter（不支持 $$ 美元引用块），本脚本使用普通 DML 完成清理。

DELETE FROM asset_match_rule
WHERE application_id IN (
    SELECT application_id FROM asset_application
    WHERE application_id LIKE 'cloud-%'
      AND application_id NOT LIKE 'cloud-%-%'
);

DELETE FROM asset_resource
WHERE application_id IN (
    SELECT application_id FROM asset_application
    WHERE application_id LIKE 'cloud-%'
      AND application_id NOT LIKE 'cloud-%-%'
);

DELETE FROM asset_application
WHERE application_id LIKE 'cloud-%'
  AND application_id NOT LIKE 'cloud-%-%';
