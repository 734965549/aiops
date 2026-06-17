-- 0003 external identity bindings for LDAP / AD / OAuth2 / OIDC / SSO.

CREATE TABLE IF NOT EXISTS iam_external_identity (
    id                     BIGSERIAL    PRIMARY KEY,
    external_identity_id   VARCHAR(36)  NOT NULL UNIQUE,
    user_id                VARCHAR(36)  NOT NULL,
    provider_id            VARCHAR(64)  NOT NULL,
    external_subject       VARCHAR(512) NOT NULL,
    external_username      VARCHAR(128) NOT NULL DEFAULT '',
    external_email         VARCHAR(128) NOT NULL DEFAULT '',
    external_groups        JSONB        NOT NULL DEFAULT '[]'::jsonb,
    last_login_at          TIMESTAMPTZ,
    created_at             TIMESTAMPTZ  NOT NULL,
    updated_at             TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_iam_external_identity_provider_subject UNIQUE (provider_id, external_subject)
);

CREATE INDEX IF NOT EXISTS idx_iam_external_identity_user_id ON iam_external_identity(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_external_identity_provider_id ON iam_external_identity(provider_id);

COMMENT ON TABLE iam_external_identity IS 'Identity context external identity binding table';
COMMENT ON COLUMN iam_external_identity.external_subject IS 'LDAP DN / OIDC sub or another provider-unique subject';
COMMENT ON COLUMN iam_external_identity.external_groups IS 'External groups synced at last login, stored as JSON array';
