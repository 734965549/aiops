-- Revert 0038: 把 0038 改写过的行从契约格式 "<provider>-cloud-<account_id>"
-- 还原为反向格式 "<provider>-<account_id>-cloud"。
-- 注意：0038 与 0036 产出相同的契约格式，仅凭当前数据无法区分哪些行是 0038 改写、
-- 哪些是 0036 改写。因此本脚本仅提供转换模板；人工回滚时必须对照 0038 执行前备份，
-- 仅挑选 0038 实际改写的行执行，否则会把 0036 归一化的行也错误还原为反向格式。
-- 不由 runner 自动执行；生产回滚由 DBA 按审批流程显式执行。

UPDATE asset_application
SET name    = split_part(name, '-', 1) || '-' || regexp_replace(description, '^Auto-created cloud sync application for account ', '') || '-cloud',
    updated_at = NOW()
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name LIKE '%-cloud-%'
  AND split_part(name, '-', 2) = 'cloud';
