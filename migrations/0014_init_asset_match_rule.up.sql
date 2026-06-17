-- 0014 initialize Asset alert label match rules.

CREATE TABLE IF NOT EXISTS asset_match_rule (
    id                   BIGSERIAL    PRIMARY KEY,
    rule_id              VARCHAR(36)  NOT NULL UNIQUE,
    name                 VARCHAR(128) NOT NULL,
    enabled              BOOLEAN      NOT NULL DEFAULT TRUE,
    priority             INT          NOT NULL DEFAULT 0,
    target_type          VARCHAR(32)  NOT NULL,
    source_type          VARCHAR(64)  NOT NULL DEFAULT 'all',
    label_key            VARCHAR(128) NOT NULL,
    label_value_pattern  VARCHAR(255) NOT NULL,
    application_id       VARCHAR(36)  NOT NULL,
    resource_id          VARCHAR(36)  NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ  NOT NULL,
    updated_at           TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_match_rule_enabled_priority
    ON asset_match_rule(enabled, priority DESC, created_at ASC);

COMMENT ON TABLE asset_match_rule IS 'Asset context alert label match rule table';
