-- 0018 initialize Integration context (cloud / observability account onboarding).

CREATE TABLE IF NOT EXISTS integration_account (
    id                BIGSERIAL    PRIMARY KEY,
    account_id        VARCHAR(64)  NOT NULL UNIQUE,
    name              VARCHAR(128) NOT NULL,
    provider          VARCHAR(64)  NOT NULL,
    auth_type         VARCHAR(32)  NOT NULL,
    regions           JSONB        NOT NULL DEFAULT '[]'::jsonb,
    project_id        VARCHAR(128) NOT NULL DEFAULT '',
    credential_ref_id VARCHAR(64)  NOT NULL DEFAULT '',
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    deleted           BOOLEAN      NOT NULL DEFAULT FALSE,
    owner_team        VARCHAR(128) NOT NULL DEFAULT '',
    description       VARCHAR(512) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ  NOT NULL,
    updated_at        TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_integration_account_provider_enabled
    ON integration_account(provider, enabled)
    WHERE deleted = FALSE;

COMMENT ON TABLE integration_account IS 'Integration context external provider account registry (cloud / observability onboarding)';
COMMENT ON COLUMN integration_account.account_id IS 'Business account identifier exposed via API';
COMMENT ON COLUMN integration_account.credential_ref_id IS 'Reference to integration_credential_ref, never plaintext secret';
COMMENT ON COLUMN integration_account.regions IS 'Provider region list JSON array';

CREATE TABLE IF NOT EXISTS integration_credential_ref (
    id                BIGSERIAL    PRIMARY KEY,
    credential_ref_id VARCHAR(64)  NOT NULL UNIQUE,
    account_id        VARCHAR(64)  NOT NULL,
    store_type        VARCHAR(32)  NOT NULL DEFAULT 'local_encrypted',
    ciphertext        BYTEA        NOT NULL DEFAULT ''::bytea,
    external_ref      VARCHAR(512) NOT NULL DEFAULT '',
    fingerprint       VARCHAR(64)  NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ  NOT NULL,
    updated_at        TIMESTAMPTZ  NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_credential_account
    ON integration_credential_ref(account_id);

COMMENT ON TABLE integration_credential_ref IS 'Encrypted credential blob or external secret reference';
COMMENT ON COLUMN integration_credential_ref.ciphertext IS 'AES-GCM encrypted credential JSON, never returned via API';
COMMENT ON COLUMN integration_credential_ref.external_ref IS 'Optional external secret manager reference';

CREATE TABLE IF NOT EXISTS integration_capability (
    id          BIGSERIAL    PRIMARY KEY,
    account_id  VARCHAR(64)  NOT NULL,
    capability  VARCHAR(32)  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL,
    updated_at  TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_integration_capability UNIQUE (account_id, capability)
);

CREATE INDEX IF NOT EXISTS idx_integration_capability_account
    ON integration_capability(account_id);

COMMENT ON TABLE integration_capability IS 'Declared provider capabilities per integration account';

CREATE TABLE IF NOT EXISTS integration_check_result (
    id           BIGSERIAL    PRIMARY KEY,
    check_id     VARCHAR(64)  NOT NULL UNIQUE,
    account_id   VARCHAR(64)  NOT NULL,
    status       VARCHAR(16)  NOT NULL,
    message      VARCHAR(512) NOT NULL DEFAULT '',
    capabilities JSONB        NOT NULL DEFAULT '[]'::jsonb,
    checked_at   TIMESTAMPTZ  NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_integration_check_account_checked
    ON integration_check_result(account_id, checked_at DESC);

COMMENT ON TABLE integration_check_result IS 'Connectivity check history for integration accounts';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000040', 'app:integrations:read', 'Read integration accounts', 'integrations', 'read', 'View integration accounts, capabilities and connectivity', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000041', 'app:integrations:create', 'Create integration accounts', 'integrations', 'create', 'Create integration provider accounts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000043', 'app:integrations:update', 'Update integration accounts', 'integrations', 'update', 'Update integration provider accounts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000044', 'app:integrations:delete', 'Delete integration accounts', 'integrations', 'delete', 'Disable or delete integration provider accounts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000045', 'app:integrations:check', 'Check integration connectivity', 'integrations', 'check', 'Run connectivity checks for integration provider accounts', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN (
    'app:integrations:read',
    'app:integrations:create',
    'app:integrations:update',
    'app:integrations:delete',
    'app:integrations:check'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

-- Migrate holders of legacy app:integrations:write to granular permissions before removal.
INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT rp.role_id, p.permission_id, NOW(), NOW()
FROM iam_role_permission rp
JOIN iam_permission old_p ON old_p.permission_id = rp.permission_id AND old_p.code = 'app:integrations:write'
JOIN iam_permission p ON p.code IN (
    'app:integrations:read',
    'app:integrations:create',
    'app:integrations:update',
    'app:integrations:delete',
    'app:integrations:check'
)
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code = 'app:integrations:write'
);

DELETE FROM iam_permission WHERE code = 'app:integrations:write';
