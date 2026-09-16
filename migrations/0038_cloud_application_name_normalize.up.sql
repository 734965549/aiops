-- 0038 normalize cloud sync application name from reverse format to contract format.
-- 背景：ensureCloudApplication 代码曾以 Name = "<provider>-<account_id>-cloud"（反向）新建云同步应用，
-- 与契约 / 0036 迁移约定的 "<provider>-cloud-<account_id>" 格式不一致，
-- 导致 0036 归一化后的存量应用名与反向格式代码新建的应用名分裂。
-- 0036 只归一化了“迁移前已存在”的应用；本迁移收敛 0036 之后由反向格式代码新建的应用。
-- 修复：Name 由 "<provider>-<account_id>-cloud" 改写为 "<provider>-cloud-<account_id>"。
-- account_id 从 description 提取（沿用 0036 思路），保证 account_id 含连字符也能正确还原。
-- 仅改 asset_application.name，不改 application_id，不影响匹配键与引用关系。
-- 匹配策略：从 description 提取 account_id，按反向格式精确比对
--   name = split_part(name,'-',1) || '-' || account_id || '-cloud'
-- 不使用 LIKE 模糊排除：account_id 含 '-cloud-' 或以 '-cloud' 结尾时，
-- 反向格式 name 同样会命中 '-cloud-' 子串，NOT LIKE '%-cloud-%' 会错误排除。
-- 幂等：
--   * 契约格式 "<provider>-cloud-<account_id>" 不等于反向格式 "<provider>-<account_id>-cloud"，
--     重复执行不命中（仅 account_id='cloud' 时两者相同，重写为同值，无副作用）。
--   * 旧格式 "<provider>-cloud" 缺少 account_id 段，不等于反向格式，不命中。
--   * 仅反向格式 "<provider>-<account_id>-cloud" 精确命中。

UPDATE asset_application
SET name    = split_part(name, '-', 1) || '-cloud-' || regexp_replace(description, '^Auto-created cloud sync application for account ', ''),
    updated_at = NOW()
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name = split_part(name, '-', 1) || '-' || regexp_replace(description, '^Auto-created cloud sync application for account ', '') || '-cloud';
