-- 0023 extend Asset context for cloud resource sync (phase 2).

ALTER TABLE asset_resource
    ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS integration_account_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cloud_resource_id VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cloud_resource_type VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS region VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sync_status VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS sync_batch_id VARCHAR(64) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_resource_cloud_key
    ON asset_resource(integration_account_id, cloud_resource_type, cloud_resource_id)
    WHERE source = 'cloud_sync' AND cloud_resource_id <> '';

CREATE INDEX IF NOT EXISTS idx_asset_resource_sync_account
    ON asset_resource(integration_account_id, sync_status, updated_at DESC);

CREATE TABLE IF NOT EXISTS asset_sync_batch (
    id                      BIGSERIAL    PRIMARY KEY,
    batch_id                VARCHAR(64)  NOT NULL UNIQUE,
    integration_account_id  VARCHAR(64)  NOT NULL,
    provider                VARCHAR(64)  NOT NULL DEFAULT '',
    status                  VARCHAR(32)  NOT NULL DEFAULT 'running',
    created_count           INT          NOT NULL DEFAULT 0,
    updated_count           INT          NOT NULL DEFAULT 0,
    stale_count             INT          NOT NULL DEFAULT 0,
    failed_count            INT          NOT NULL DEFAULT 0,
    message                 VARCHAR(512) NOT NULL DEFAULT '',
    started_at              TIMESTAMPTZ  NOT NULL,
    finished_at             TIMESTAMPTZ  NULL,
    created_at              TIMESTAMPTZ  NOT NULL,
    updated_at              TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_sync_batch_account
    ON asset_sync_batch(integration_account_id, created_at DESC);

COMMENT ON TABLE asset_sync_batch IS 'Asset context cloud sync batch records';
COMMENT ON COLUMN asset_resource.source IS 'manual or cloud_sync';
COMMENT ON COLUMN asset_resource.sync_status IS 'active or stale for cloud_sync resources';
