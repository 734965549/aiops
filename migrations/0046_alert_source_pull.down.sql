-- 0046 alert source pull-based ingestion support (manual rollback reference).

DROP TABLE IF EXISTS alert_source_sync_lease;

ALTER TABLE alert_source
    DROP COLUMN IF EXISTS last_sync_status,
    DROP COLUMN IF EXISTS last_sync_at,
    DROP COLUMN IF EXISTS pull_config,
    DROP COLUMN IF EXISTS account_id;
