-- 回滚 0029：把本迁移写入的 native 配置还原为 {}，使历史华为账号回到 0024 后的空配置状态。
-- 注意：回滚后这些账号会再次被解析为 ces（见 sync_mode_config.go），仅在确需回到 0024 状态时执行。
UPDATE integration_account
SET extra_config = '{}'::jsonb,
    updated_at   = NOW()
WHERE provider = 'huawei_cloud'
  AND extra_config = '{"sync_mode":"native"}'::jsonb;
