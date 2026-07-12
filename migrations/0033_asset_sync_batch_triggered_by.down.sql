-- 0033 rollback reference: remove asset sync batch triggered_by.

ALTER TABLE asset_sync_batch
    DROP COLUMN IF EXISTS triggered_by;
