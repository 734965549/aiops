-- 0009 initialize Audit context.

CREATE TABLE IF NOT EXISTS audit_operation (
    id              BIGSERIAL    PRIMARY KEY,
    audit_id        VARCHAR(36)  NOT NULL UNIQUE,
    user_id         VARCHAR(36)  NOT NULL DEFAULT '',
    resource_type   VARCHAR(64)  NOT NULL,
    resource_id     VARCHAR(128) NOT NULL,
    action          VARCHAR(64)  NOT NULL,
    payload         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    ip              VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent      VARCHAR(512) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_operation_resource ON audit_operation(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_operation_user_id ON audit_operation(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_operation_action ON audit_operation(action);
CREATE INDEX IF NOT EXISTS idx_audit_operation_created_at ON audit_operation(created_at);

COMMENT ON TABLE audit_operation IS 'Audit context business operation audit table';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000022', 'app:audits:read', 'Read operation audits', 'audits', 'read', 'Query platform business operation audit logs', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code = 'app:audits:read'
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
