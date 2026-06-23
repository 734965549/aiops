-- 0020 initialize Inspection context (policies, runs, findings, recommendations + permissions).

CREATE TABLE IF NOT EXISTS inspection_policy (
    id                     BIGSERIAL    PRIMARY KEY,
    policy_id              VARCHAR(64)  NOT NULL UNIQUE,
    name                   VARCHAR(128) NOT NULL,
    enabled                BOOLEAN      NOT NULL DEFAULT TRUE,
    schedule               VARCHAR(64)  NOT NULL DEFAULT '',
    scope                  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    checks                 JSONB        NOT NULL DEFAULT '[]'::jsonb,
    agent_profile          VARCHAR(64)  NOT NULL DEFAULT 'sre_default',
    notification_policy_id VARCHAR(64)  NOT NULL DEFAULT '',
    deleted                BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ  NOT NULL,
    updated_at             TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inspection_policy_enabled
    ON inspection_policy(enabled)
    WHERE deleted = FALSE;

COMMENT ON TABLE inspection_policy IS 'Inspection policy: scope, checks, schedule and agent profile';
COMMENT ON COLUMN inspection_policy.scope IS 'JSON: environment, account_id, provider, application_ids, resource_types';
COMMENT ON COLUMN inspection_policy.checks IS 'JSON array of check codes, e.g. metrics.cpu, traces.error_rate';

CREATE TABLE IF NOT EXISTS inspection_run (
    id           BIGSERIAL    PRIMARY KEY,
    run_id       VARCHAR(64)  NOT NULL UNIQUE,
    policy_id    VARCHAR(64)  NOT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'pending',
    trigger_type VARCHAR(16)  NOT NULL DEFAULT 'manual',
    summary      VARCHAR(512) NOT NULL DEFAULT '',
    timeline     JSONB        NOT NULL DEFAULT '[]'::jsonb,
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inspection_run_policy_created
    ON inspection_run(policy_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_inspection_run_status
    ON inspection_run(status);

COMMENT ON TABLE inspection_run IS 'Single inspection execution record with state machine and timeline';
COMMENT ON COLUMN inspection_run.timeline IS 'JSON array of {ts, event, detail} audit-friendly events';

CREATE TABLE IF NOT EXISTS inspection_finding (
    id                 BIGSERIAL    PRIMARY KEY,
    finding_id         VARCHAR(64)  NOT NULL UNIQUE,
    run_id             VARCHAR(64)  NOT NULL,
    policy_id          VARCHAR(64)  NOT NULL,
    risk_level         VARCHAR(16)  NOT NULL,
    category           VARCHAR(64)  NOT NULL DEFAULT '',
    summary            VARCHAR(512) NOT NULL,
    detail             TEXT         NOT NULL DEFAULT '',
    affected_resources JSONB        NOT NULL DEFAULT '[]'::jsonb,
    evidence_refs      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    confidence         REAL         NOT NULL DEFAULT 0,
    uncertainty        VARCHAR(512) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ  NOT NULL,
    updated_at         TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inspection_finding_run
    ON inspection_finding(run_id);

CREATE INDEX IF NOT EXISTS idx_inspection_finding_risk
    ON inspection_finding(risk_level);

COMMENT ON TABLE inspection_finding IS 'Inspection finding with evidence refs and AI analysis metadata';

CREATE TABLE IF NOT EXISTS inspection_recommendation (
    id                   BIGSERIAL    PRIMARY KEY,
    recommendation_id    VARCHAR(64)  NOT NULL UNIQUE,
    finding_id           VARCHAR(64)  NOT NULL,
    run_id               VARCHAR(64)  NOT NULL,
    title                VARCHAR(256) NOT NULL,
    reason               TEXT         NOT NULL DEFAULT '',
    suggested_action     TEXT         NOT NULL DEFAULT '',
    risk_level           VARCHAR(16)  NOT NULL,
    status               VARCHAR(16)  NOT NULL DEFAULT 'open',
    can_create_execution BOOLEAN      NOT NULL DEFAULT FALSE,
    confidence           REAL         NOT NULL DEFAULT 0,
    uncertainty          VARCHAR(512) NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ  NOT NULL,
    updated_at           TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inspection_recommendation_run
    ON inspection_recommendation(run_id);

CREATE INDEX IF NOT EXISTS idx_inspection_recommendation_finding
    ON inspection_recommendation(finding_id);

COMMENT ON TABLE inspection_recommendation IS 'Actionable recommendation linked to finding and evidence chain';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000046', 'app:inspections:read', 'Read inspections', 'inspections', 'read', 'View inspection policies, runs, findings and recommendations', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000047', 'app:inspections:write', 'Manage inspections', 'inspections', 'write', 'Create/update policies and trigger inspection runs', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code IN ('app:inspections:read', 'app:inspections:write')
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_ai_tool_permission (tool_permission_id, tool_code, tool_name, permission_mode, allow_confirm, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0003-000000000010', 'inspection.runs.create', 'Create inspection run', 'require_confirm', TRUE, 'Manually trigger inspection run via AI tool gateway', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000011', 'inspection.findings.analyze', 'Analyze inspection findings', 'read_only', FALSE, 'Read-only analysis of inspection findings with evidence chain', NOW(), NOW())
ON CONFLICT (tool_code) DO UPDATE SET
    tool_name = EXCLUDED.tool_name,
    permission_mode = EXCLUDED.permission_mode,
    allow_confirm = EXCLUDED.allow_confirm,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_ai_tool_permission (role_id, tool_permission_id, created_at, updated_at)
SELECT r.role_id, tp.tool_permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_ai_tool_permission tp ON tp.tool_code IN ('inspection.runs.create', 'inspection.findings.analyze')
WHERE r.code = 'admin'
ON CONFLICT (role_id, tool_permission_id) DO UPDATE SET updated_at = NOW();
