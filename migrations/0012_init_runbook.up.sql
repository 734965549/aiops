-- 0012 initialize Runbook templates and extend Execution task/step tables.

CREATE TABLE IF NOT EXISTS runbook_template (
    id                  BIGSERIAL    PRIMARY KEY,
    template_id         VARCHAR(36)  NOT NULL UNIQUE,
    name                VARCHAR(255) NOT NULL,
    description         TEXT         NOT NULL DEFAULT '',
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    operation_type      VARCHAR(32)  NOT NULL,
    risk_level          VARCHAR(16)  NOT NULL,
    match_alert_name    VARCHAR(255) NOT NULL DEFAULT '',
    match_resource_type VARCHAR(64)  NOT NULL DEFAULT '',
    match_environment   VARCHAR(64)  NOT NULL DEFAULT '',
    parameter_schema    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    rollback_plan       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_by          VARCHAR(36)  NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL,
    updated_at          TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runbook_template_enabled ON runbook_template(enabled, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_runbook_template_match ON runbook_template(match_environment, match_resource_type, match_alert_name);
CREATE INDEX IF NOT EXISTS idx_runbook_template_risk ON runbook_template(risk_level);

COMMENT ON TABLE runbook_template IS 'Runbook template table';

CREATE TABLE IF NOT EXISTS runbook_step (
    id                  BIGSERIAL    PRIMARY KEY,
    step_id             VARCHAR(36)  NOT NULL UNIQUE,
    template_id         VARCHAR(36)  NOT NULL,
    step_order          INT          NOT NULL,
    name                VARCHAR(255) NOT NULL,
    action_type         VARCHAR(64)  NOT NULL,
    risk_level          VARCHAR(16)  NOT NULL,
    dry_run_supported   BOOLEAN      NOT NULL DEFAULT FALSE,
    default_dry_run     BOOLEAN      NOT NULL DEFAULT FALSE,
    parameter_schema    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    default_parameters  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    rollback_plan       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    timeout_seconds     INT          NOT NULL DEFAULT 300,
    created_at          TIMESTAMPTZ  NOT NULL,
    updated_at          TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_runbook_step_template_order UNIQUE (template_id, step_order)
);

CREATE INDEX IF NOT EXISTS idx_runbook_step_template ON runbook_step(template_id, step_order);

COMMENT ON TABLE runbook_step IS 'Runbook step template table';

ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS runbook_template_id VARCHAR(36) NOT NULL DEFAULT '';
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS runbook_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS dry_run BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS runbook_step_id VARCHAR(36) NOT NULL DEFAULT '';
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS parameters JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS risk_level VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS dry_run BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS rollback_plan JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS timeout_seconds INT NOT NULL DEFAULT 0;

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000027', 'app:runbooks:read', 'Read runbooks', 'runbooks', 'read', 'Read Runbook templates and recommendations', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000028', 'app:runbooks:create', 'Create runbooks', 'runbooks', 'create', 'Create Runbook templates', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000029', 'app:runbooks:update', 'Update runbooks', 'runbooks', 'update', 'Update Runbook templates', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000030', 'app:runbooks:delete', 'Delete runbooks', 'runbooks', 'delete', 'Delete Runbook templates', NOW(), NOW())
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
    'app:runbooks:read',
    'app:runbooks:create',
    'app:runbooks:update',
    'app:runbooks:delete'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO runbook_template (
    template_id, name, description, enabled, operation_type, risk_level,
    match_alert_name, match_resource_type, match_environment,
    parameter_schema, rollback_plan, created_by, created_at, updated_at
) VALUES (
    '00000000-0000-0000-0002-000000000001',
    'Restart service and verify',
    'Handle production Pod HighCPU by restarting the target service and verifying metrics recovery',
    TRUE, 'runbook', 'medium',
    'HighCPU', 'pod', 'prod',
    '{"type":"object","properties":{"service_name":{"type":"string"},"replicas":{"type":"integer"}}}'::jsonb,
    '{"description":"If verification fails, roll back to the previous version"}'::jsonb,
    'system', NOW(), NOW()
) ON CONFLICT (template_id) DO NOTHING;

INSERT INTO runbook_step (
    step_id, template_id, step_order, name, action_type, risk_level,
    dry_run_supported, default_dry_run, parameter_schema, default_parameters,
    rollback_plan, timeout_seconds, created_at, updated_at
) VALUES
    (
        '00000000-0000-0000-0002-000000000011',
        '00000000-0000-0000-0002-000000000001', 1,
        'Dry-run precheck', 'command', 'low',
        TRUE, TRUE, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb, 120, NOW(), NOW()
    ),
    (
        '00000000-0000-0000-0002-000000000012',
        '00000000-0000-0000-0002-000000000001', 2,
        'Restart target service', 'restart', 'medium',
        TRUE, FALSE, '{}'::jsonb, '{"grace_period_s":30}'::jsonb, '{}'::jsonb, 300, NOW(), NOW()
    ),
    (
        '00000000-0000-0000-0002-000000000013',
        '00000000-0000-0000-0002-000000000001', 3,
        'Verify metrics recovery', 'http', 'low',
        TRUE, TRUE, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb, 180, NOW(), NOW()
    )
ON CONFLICT (step_id) DO NOTHING;
