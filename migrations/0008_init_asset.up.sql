-- 0008 initialize Asset context.

CREATE TABLE IF NOT EXISTS asset_application (
    id              BIGSERIAL    PRIMARY KEY,
    application_id  VARCHAR(36)  NOT NULL UNIQUE,
    name            VARCHAR(128) NOT NULL,
    environment     VARCHAR(64)  NOT NULL DEFAULT '',
    namespace       VARCHAR(128) NOT NULL DEFAULT '',
    description     VARCHAR(255) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_application_name_env ON asset_application(name, environment);

CREATE TABLE IF NOT EXISTS asset_resource (
    id              BIGSERIAL    PRIMARY KEY,
    resource_id     VARCHAR(36)  NOT NULL UNIQUE,
    application_id  VARCHAR(36)  NOT NULL,
    name            VARCHAR(255) NOT NULL DEFAULT '',
    resource_type   VARCHAR(64)  NOT NULL DEFAULT '',
    namespace       VARCHAR(128) NOT NULL DEFAULT '',
    pod             VARCHAR(255) NOT NULL DEFAULT '',
    node            VARCHAR(255) NOT NULL DEFAULT '',
    instance        VARCHAR(255) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_resource_app_id ON asset_resource(application_id);
CREATE INDEX IF NOT EXISTS idx_asset_resource_match ON asset_resource(application_id, namespace, pod, node, instance);

COMMENT ON TABLE asset_application IS 'Asset context application/service registry table';
COMMENT ON TABLE asset_resource IS 'Asset context pod/node/instance resource registry table';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000020', 'app:assets:read', 'Read assets', 'assets', 'read', 'Read application and resource registry data', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000021', 'app:assets:write', 'Manage assets', 'assets', 'write', 'Create and update application/resource registry data', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN ('app:assets:read', 'app:assets:write')
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
