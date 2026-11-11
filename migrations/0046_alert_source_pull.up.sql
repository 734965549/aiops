-- 0046 alert source pull-based ingestion support.
-- Contract: ops/huawei-ces-alert-sync-contract.md §4.1 and §9.
-- Timestamps are maintained by the Go application (no DB defaults/triggers).

ALTER TABLE alert_source
    ADD COLUMN IF NOT EXISTS account_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pull_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS last_sync_status VARCHAR(32) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS alert_source_sync_lease (
    id BIGSERIAL PRIMARY KEY,
    source_id VARCHAR(64) NOT NULL UNIQUE,
    owner_id VARCHAR(128) NOT NULL,
    lease_until TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_source_sync_lease_until
    ON alert_source_sync_lease (lease_until);
