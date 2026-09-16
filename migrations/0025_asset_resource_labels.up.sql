-- 0025 add asset_resource.labels for cloud sync enrichment details (private_ip/flavor/vpc_id/az etc).

ALTER TABLE asset_resource
    ADD COLUMN IF NOT EXISTS labels JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN asset_resource.labels IS 'Cloud sync labels (CES namespace/dim_name + native API enrichment: private_ip/flavor/vpc_id/az); overwritten each sync';
