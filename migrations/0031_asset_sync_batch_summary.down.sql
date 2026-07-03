-- 0031 rollback reference: remove asset sync batch structured summary.

ALTER TABLE asset_sync_batch
    DROP COLUMN IF EXISTS summary;
