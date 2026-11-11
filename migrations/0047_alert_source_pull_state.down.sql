-- 手动回滚参考，不由迁移 runner 自动执行。
ALTER TABLE alert_source
    DROP COLUMN IF EXISTS sync_state,
    DROP COLUMN IF EXISTS delivery_mode;
