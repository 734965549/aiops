-- 0006 authentication audit events.

CREATE TABLE IF NOT EXISTS iam_auth_audit (
    id          BIGSERIAL    PRIMARY KEY,
    audit_id    VARCHAR(36)  NOT NULL UNIQUE,
    user_id     VARCHAR(36)  NOT NULL DEFAULT '',
    username    VARCHAR(64)  NOT NULL DEFAULT '',
    provider_id VARCHAR(64)  NOT NULL DEFAULT '',
    event       VARCHAR(32)  NOT NULL,
    method      VARCHAR(32)  NOT NULL,
    result      VARCHAR(32)  NOT NULL,
    ip          VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent  VARCHAR(512) NOT NULL DEFAULT '',
    reason      VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL,
    updated_at  TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_iam_auth_audit_user_id ON iam_auth_audit(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_auth_audit_username ON iam_auth_audit(username);
CREATE INDEX IF NOT EXISTS idx_iam_auth_audit_provider_id ON iam_auth_audit(provider_id);
CREATE INDEX IF NOT EXISTS idx_iam_auth_audit_event ON iam_auth_audit(event);
CREATE INDEX IF NOT EXISTS idx_iam_auth_audit_result ON iam_auth_audit(result);
CREATE INDEX IF NOT EXISTS idx_iam_auth_audit_created_at ON iam_auth_audit(created_at);

COMMENT ON TABLE iam_auth_audit IS 'Identity context authentication audit events';
COMMENT ON COLUMN iam_auth_audit.audit_id IS 'Business audit identifier';
COMMENT ON COLUMN iam_auth_audit.event IS 'login / refresh / logout';
COMMENT ON COLUMN iam_auth_audit.method IS 'local / external / oauth / refresh';
COMMENT ON COLUMN iam_auth_audit.result IS 'success / failure';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000012', 'app:identity.auth_audits:read', 'Read authentication audits', 'identity.auth_audits', 'read', 'Query login, refresh token and logout audit events', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code = 'app:identity.auth_audits:read'
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
