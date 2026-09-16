DROP TABLE IF EXISTS asset_sync_batch;

DROP INDEX IF EXISTS idx_asset_resource_sync_account;
DROP INDEX IF EXISTS idx_asset_resource_cloud_key;

ALTER TABLE asset_resource
    DROP COLUMN IF EXISTS sync_batch_id,
    DROP COLUMN IF EXISTS last_synced_at,
    DROP COLUMN IF EXISTS sync_status,
    DROP COLUMN IF EXISTS region,
    DROP COLUMN IF EXISTS cloud_resource_type,
    DROP COLUMN IF EXISTS cloud_resource_id,
    DROP COLUMN IF EXISTS integration_account_id,
    DROP COLUMN IF EXISTS source;
