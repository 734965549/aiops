-- CMDB 手工资源关系；应用与资源归属仍由 asset_resource.application_id 派生。
CREATE TABLE IF NOT EXISTS asset_resource_relation (
  id BIGSERIAL PRIMARY KEY,
  relation_id VARCHAR(36) NOT NULL,
  source_resource_id VARCHAR(36) NOT NULL,
  target_resource_id VARCHAR(36) NOT NULL,
  relation_type VARCHAR(32) NOT NULL,
  origin VARCHAR(32) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_asset_resource_relation_id ON asset_resource_relation(relation_id);
CREATE UNIQUE INDEX IF NOT EXISTS uk_asset_resource_relation_pair ON asset_resource_relation(source_resource_id, target_resource_id, relation_type);
CREATE INDEX IF NOT EXISTS idx_asset_resource_relation_target ON asset_resource_relation(target_resource_id);
CREATE INDEX IF NOT EXISTS idx_asset_resource_relation_origin ON asset_resource_relation(origin);

COMMENT ON TABLE asset_resource_relation IS 'CMDB resource dependency and attachment relations';
