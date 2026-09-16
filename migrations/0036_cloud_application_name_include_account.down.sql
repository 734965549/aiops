-- Revert cloud sync application name from "<provider>-cloud-<account_id>" back to "<provider>-cloud".
-- provider 不含连字符（如 huawei_cloud、aliyun_cloud），取 name 前两段 split_part 即可还原。

UPDATE asset_application
SET name    = split_part(name, '-', 1) || '-' || split_part(name, '-', 2),
    updated_at = NOW()
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND name LIKE '%-cloud-%';
