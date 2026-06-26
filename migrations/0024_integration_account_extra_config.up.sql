-- 0024 add integration_account.extra_config for provider-specific sync settings (e.g. huawei sync_mode).

ALTER TABLE integration_account
    ADD COLUMN IF NOT EXISTS extra_config JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN integration_account.extra_config IS 'Provider-specific extension config JSON (huawei sync_mode/resource_group_name/max_resources); never store secrets';
