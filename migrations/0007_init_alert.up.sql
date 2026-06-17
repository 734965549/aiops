-- 0007 initialize Alert context.

CREATE TABLE IF NOT EXISTS alert_alert (
    id                BIGSERIAL    PRIMARY KEY,
    alert_id          VARCHAR(36)  NOT NULL UNIQUE,
    external_id       VARCHAR(128) NOT NULL DEFAULT '',
    source            VARCHAR(64)  NOT NULL,
    source_id         VARCHAR(64)  NOT NULL DEFAULT '',
    source_name       VARCHAR(128) NOT NULL DEFAULT '',
    fingerprint       VARCHAR(128) NOT NULL DEFAULT '',
    dedup_key         VARCHAR(128) NOT NULL,
    lifecycle_seq     INT          NOT NULL DEFAULT 1,
    name              VARCHAR(255) NOT NULL,
    summary           VARCHAR(512) NOT NULL DEFAULT '',
    description       TEXT         NOT NULL DEFAULT '',
    severity          VARCHAR(16)  NOT NULL,
    status            VARCHAR(32)  NOT NULL,
    rule_id           VARCHAR(128) NOT NULL DEFAULT '',
    rule_name         VARCHAR(255) NOT NULL DEFAULT '',
    business_line     VARCHAR(128) NOT NULL DEFAULT '',
    environment       VARCHAR(64)  NOT NULL DEFAULT '',
    application_id    VARCHAR(36)  NOT NULL DEFAULT '',
    application_name  VARCHAR(128) NOT NULL DEFAULT '',
    resource_id       VARCHAR(36)  NOT NULL DEFAULT '',
    resource_type     VARCHAR(64)  NOT NULL DEFAULT '',
    resource_name     VARCHAR(255) NOT NULL DEFAULT '',
    owner_user_id     VARCHAR(36)  NOT NULL DEFAULT '',
    assignee_user_id  VARCHAR(36)  NOT NULL DEFAULT '',
    labels            JSONB        NOT NULL DEFAULT '{}'::jsonb,
    annotations       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    occurrence_count  INT          NOT NULL DEFAULT 1,
    first_seen_at     TIMESTAMPTZ  NOT NULL,
    last_seen_at      TIMESTAMPTZ  NOT NULL,
    recovered_at      TIMESTAMPTZ,
    acknowledged_at   TIMESTAMPTZ,
    closed_at         TIMESTAMPTZ,
    silenced_until    TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL,
    updated_at        TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_alert_dedup_lifecycle UNIQUE (dedup_key, lifecycle_seq)
);

CREATE INDEX IF NOT EXISTS idx_alert_status_severity_last_seen ON alert_alert(status, severity, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_source_external ON alert_alert(source_id, external_id);
CREATE INDEX IF NOT EXISTS idx_alert_source_dedup ON alert_alert(source_id, dedup_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_active_dedup ON alert_alert(source_id, dedup_key) WHERE status <> 'closed';

COMMENT ON TABLE alert_alert IS 'Alert context main alert lifecycle table';
COMMENT ON COLUMN alert_alert.alert_id IS 'Business alert identifier';
COMMENT ON COLUMN alert_alert.external_id IS 'External alert identifier or fingerprint';
COMMENT ON COLUMN alert_alert.source IS 'Source type, for example prometheus_alertmanager or custom_webhook';
COMMENT ON COLUMN alert_alert.source_id IS 'Platform alert source identifier';
COMMENT ON COLUMN alert_alert.dedup_key IS 'Platform deduplication key';
COMMENT ON COLUMN alert_alert.lifecycle_seq IS 'Lifecycle sequence, increments after closed refire';
COMMENT ON COLUMN alert_alert.severity IS 'p0 / p1 / p2 / p3 / info';
COMMENT ON COLUMN alert_alert.status IS 'new / acknowledged / processing / recovered / closed / silenced';
COMMENT ON COLUMN alert_alert.labels IS 'Normalized labels JSONB';
COMMENT ON COLUMN alert_alert.annotations IS 'Normalized annotations JSONB';
COMMENT ON COLUMN alert_alert.occurrence_count IS 'Repeated firing count for the active alert';
COMMENT ON COLUMN alert_alert.application_id IS 'Related Asset application ID';
COMMENT ON COLUMN alert_alert.resource_id IS 'Related Asset resource ID';

CREATE TABLE IF NOT EXISTS alert_event (
    id          BIGSERIAL     PRIMARY KEY,
    event_id    VARCHAR(36)   NOT NULL UNIQUE,
    alert_id    VARCHAR(36)   NOT NULL,
    event_type  VARCHAR(64)   NOT NULL,
    actor_type  VARCHAR(32)   NOT NULL,
    actor_id    VARCHAR(64)   NOT NULL DEFAULT '',
    actor_name  VARCHAR(128)  NOT NULL DEFAULT '',
    message     VARCHAR(1024) NOT NULL DEFAULT '',
    payload     JSONB         NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ   NOT NULL,
    updated_at  TIMESTAMPTZ   NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_event_alert_id ON alert_event(alert_id, created_at);

COMMENT ON TABLE alert_event IS 'Alert context timeline event table';
COMMENT ON COLUMN alert_event.event_type IS 'Event type defined in ops/alert-contract.md';
COMMENT ON COLUMN alert_event.actor_type IS 'system / user / integration';
COMMENT ON COLUMN alert_event.payload IS 'Extended event payload JSONB';

CREATE TABLE IF NOT EXISTS alert_source (
    id            BIGSERIAL    PRIMARY KEY,
    source_id     VARCHAR(64)  NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    type          VARCHAR(64)  NOT NULL,
    enabled       BOOLEAN      NOT NULL DEFAULT TRUE,
    secret_hash   VARCHAR(128) NOT NULL DEFAULT '',
    environment   VARCHAR(64)  NOT NULL DEFAULT '',
    business_line VARCHAR(128) NOT NULL DEFAULT '',
    description   VARCHAR(255) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL,
    updated_at    TIMESTAMPTZ  NOT NULL
);

COMMENT ON TABLE alert_source IS 'Alert context external alert source configuration';
COMMENT ON COLUMN alert_source.source_id IS 'Source ID used in webhook URL path';
COMMENT ON COLUMN alert_source.type IS 'prometheus_alertmanager / huawei_ces / signoz / zabbix / custom_webhook';
COMMENT ON COLUMN alert_source.secret_hash IS 'SHA-256 hash of X-AIOPS-Webhook-Token; raw token is never returned';

CREATE TABLE IF NOT EXISTS alert_silence (
    id          BIGSERIAL    PRIMARY KEY,
    silence_id  VARCHAR(36)  NOT NULL UNIQUE,
    alert_id    VARCHAR(36)  NOT NULL DEFAULT '',
    matcher     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    reason      VARCHAR(512) NOT NULL,
    starts_at   TIMESTAMPTZ  NOT NULL,
    ends_at     TIMESTAMPTZ  NOT NULL,
    created_by  VARCHAR(36)  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL,
    updated_at  TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_silence_alert_id ON alert_silence(alert_id);

COMMENT ON TABLE alert_silence IS 'Alert context silence record table';
COMMENT ON COLUMN alert_silence.matcher IS 'Label matcher JSONB; may be empty for single-alert silence';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000013', 'app:alerts:read', 'Read alerts', 'alerts', 'read', 'Read alert list, detail and timeline', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000014', 'app:alerts:acknowledge', 'Acknowledge alerts', 'alerts', 'acknowledge', 'Acknowledge alerts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000015', 'app:alerts:assign', 'Assign alerts', 'alerts', 'assign', 'Assign alert owner', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000016', 'app:alerts:update', 'Update alerts', 'alerts', 'update', 'Start processing, recover and comment on alerts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000017', 'app:alerts:close', 'Close alerts', 'alerts', 'close', 'Close alerts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000018', 'app:alerts:silence', 'Silence alerts', 'alerts', 'silence', 'Silence and unsilence alerts', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000019', 'app:alerts:ingest', 'Manage alert sources', 'alerts', 'ingest', 'Manage alert source configuration', NOW(), NOW())
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
    'app:alerts:read',
    'app:alerts:acknowledge',
    'app:alerts:assign',
    'app:alerts:update',
    'app:alerts:close',
    'app:alerts:silence',
    'app:alerts:ingest'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
