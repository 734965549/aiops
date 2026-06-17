-- AI ops platform - 0001 initialize Identity context core tables.
-- Business timestamps are maintained by Go code; no database defaults or triggers.

CREATE TABLE IF NOT EXISTS iam_user (
    id             BIGSERIAL    PRIMARY KEY,
    user_id        VARCHAR(36)  NOT NULL UNIQUE,
    username       VARCHAR(64)  NOT NULL UNIQUE,
    display_name   VARCHAR(128) NOT NULL DEFAULT '',
    email          VARCHAR(128) NOT NULL DEFAULT '',
    password_hash  VARCHAR(255) NOT NULL DEFAULT '',
    status         VARCHAR(32)  NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ  NOT NULL,
    updated_at     TIMESTAMPTZ  NOT NULL
);

CREATE TABLE IF NOT EXISTS iam_role (
    id            BIGSERIAL    PRIMARY KEY,
    role_id       VARCHAR(36)  NOT NULL UNIQUE,
    code          VARCHAR(64)  NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    description   VARCHAR(255) NOT NULL DEFAULT '',
    status        VARCHAR(32)  NOT NULL DEFAULT 'active',
    is_system     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ  NOT NULL,
    updated_at    TIMESTAMPTZ  NOT NULL
);

CREATE TABLE IF NOT EXISTS iam_permission (
    id            BIGSERIAL    PRIMARY KEY,
    permission_id VARCHAR(36)  NOT NULL UNIQUE,
    code          VARCHAR(128) NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    resource      VARCHAR(128) NOT NULL,
    action        VARCHAR(64)  NOT NULL,
    description   VARCHAR(255) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL,
    updated_at    TIMESTAMPTZ  NOT NULL
);

CREATE TABLE IF NOT EXISTS iam_user_role (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     VARCHAR(36) NOT NULL,
    role_id     VARCHAR(36) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_iam_user_role UNIQUE (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS iam_role_permission (
    id            BIGSERIAL   PRIMARY KEY,
    role_id       VARCHAR(36) NOT NULL,
    permission_id VARCHAR(36) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_iam_role_permission UNIQUE (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS iam_data_scope (
    id            BIGSERIAL    PRIMARY KEY,
    data_scope_id VARCHAR(36)  NOT NULL UNIQUE,
    code          VARCHAR(64)  NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    scope_type    VARCHAR(32)  NOT NULL,
    scope_config  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    description   VARCHAR(255) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL,
    updated_at    TIMESTAMPTZ  NOT NULL
);

CREATE TABLE IF NOT EXISTS iam_role_data_scope (
    id            BIGSERIAL   PRIMARY KEY,
    role_id       VARCHAR(36) NOT NULL,
    data_scope_id VARCHAR(36) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_iam_role_data_scope UNIQUE (role_id, data_scope_id)
);

CREATE TABLE IF NOT EXISTS iam_ai_tool_permission (
    id                 BIGSERIAL    PRIMARY KEY,
    tool_permission_id VARCHAR(36)  NOT NULL UNIQUE,
    tool_code          VARCHAR(128) NOT NULL UNIQUE,
    tool_name          VARCHAR(128) NOT NULL,
    permission_mode    VARCHAR(32)  NOT NULL,
    allow_confirm      BOOLEAN      NOT NULL DEFAULT FALSE,
    description        VARCHAR(255) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ  NOT NULL,
    updated_at         TIMESTAMPTZ  NOT NULL
);

CREATE TABLE IF NOT EXISTS iam_role_ai_tool_permission (
    id                 BIGSERIAL   PRIMARY KEY,
    role_id            VARCHAR(36) NOT NULL,
    tool_permission_id VARCHAR(36) NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_iam_role_ai_tool_permission UNIQUE (role_id, tool_permission_id)
);

CREATE INDEX IF NOT EXISTS idx_iam_user_email ON iam_user(email);
CREATE INDEX IF NOT EXISTS idx_iam_user_status ON iam_user(status);
CREATE INDEX IF NOT EXISTS idx_iam_role_status ON iam_role(status);
CREATE INDEX IF NOT EXISTS idx_iam_permission_resource ON iam_permission(resource);
CREATE INDEX IF NOT EXISTS idx_iam_permission_action ON iam_permission(action);
CREATE INDEX IF NOT EXISTS idx_iam_user_role_user_id ON iam_user_role(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_user_role_role_id ON iam_user_role(role_id);
CREATE INDEX IF NOT EXISTS idx_iam_role_permission_role_id ON iam_role_permission(role_id);
CREATE INDEX IF NOT EXISTS idx_iam_role_permission_permission_id ON iam_role_permission(permission_id);
CREATE INDEX IF NOT EXISTS idx_iam_role_data_scope_role_id ON iam_role_data_scope(role_id);
CREATE INDEX IF NOT EXISTS idx_iam_role_data_scope_data_scope_id ON iam_role_data_scope(data_scope_id);
CREATE INDEX IF NOT EXISTS idx_iam_role_ai_tool_permission_role_id ON iam_role_ai_tool_permission(role_id);
CREATE INDEX IF NOT EXISTS idx_iam_role_ai_tool_permission_tool_permission_id ON iam_role_ai_tool_permission(tool_permission_id);

COMMENT ON TABLE iam_user IS 'Identity context user table';
COMMENT ON COLUMN iam_user.id IS 'Database surrogate key, not exposed by API';
COMMENT ON COLUMN iam_user.user_id IS 'Business user identifier for API and cross-context references';
COMMENT ON COLUMN iam_user.status IS 'active / disabled / locked';
COMMENT ON COLUMN iam_user.created_at IS 'Written by Go code, no database default';
COMMENT ON COLUMN iam_user.updated_at IS 'Written by Go code on insert/update, no database trigger';

COMMENT ON TABLE iam_role IS 'Identity context role table';
COMMENT ON COLUMN iam_role.role_id IS 'Business role identifier';
COMMENT ON COLUMN iam_role.code IS 'Role code, for example admin / operator / viewer';
COMMENT ON COLUMN iam_role.status IS 'active / disabled';
COMMENT ON COLUMN iam_role.is_system IS 'System built-in role flag';

COMMENT ON TABLE iam_permission IS 'Identity context permission table';
COMMENT ON COLUMN iam_permission.code IS 'Permission code, for example app:user:read';
COMMENT ON COLUMN iam_permission.resource IS 'Permission resource domain';
COMMENT ON COLUMN iam_permission.action IS 'Permission action';

COMMENT ON TABLE iam_user_role IS 'Identity context user-role relation table';
COMMENT ON TABLE iam_role_permission IS 'Identity context role-permission relation table';
COMMENT ON TABLE iam_data_scope IS 'Identity context data scope table';
COMMENT ON COLUMN iam_data_scope.scope_type IS 'all / department / team / region / tag / custom';
COMMENT ON COLUMN iam_data_scope.scope_config IS 'Data scope config stored as JSON';
COMMENT ON TABLE iam_role_data_scope IS 'Identity context role-data-scope relation table';
COMMENT ON TABLE iam_ai_tool_permission IS 'Identity context AI tool permission table';
COMMENT ON COLUMN iam_ai_tool_permission.tool_code IS 'Tool code, for example metrics.query / logs.search';
COMMENT ON COLUMN iam_ai_tool_permission.permission_mode IS 'read_only / require_confirm / deny';
COMMENT ON COLUMN iam_ai_tool_permission.allow_confirm IS 'Whether manual confirmation can allow high-risk tool execution';
COMMENT ON TABLE iam_role_ai_tool_permission IS 'Identity context role-AI-tool-permission relation table';
