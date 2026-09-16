-- 0030 rollback reference: remove asset sync fencing token metadata.

DROP INDEX IF EXISTS idx_asset_sync_batch_fencing_running;

ALTER TABLE asset_sync_batch
    DROP COLUMN IF EXISTS fencing_token;
