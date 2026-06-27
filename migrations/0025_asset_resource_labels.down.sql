-- 0025 rollback: remove asset_resource.labels.

ALTER TABLE asset_resource
    DROP COLUMN IF EXISTS labels;
