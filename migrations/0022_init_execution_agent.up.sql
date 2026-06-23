-- 0022 Execution Agent: media, agents, command specs, leases, log streams + task extensions.

CREATE TABLE IF NOT EXISTS exec_medium (
    id                  BIGSERIAL    PRIMARY KEY,
    medium_id           VARCHAR(64)  NOT NULL UNIQUE,
    name                VARCHAR(128) NOT NULL,
    medium_type         VARCHAR(32)  NOT NULL,
    environment         VARCHAR(64)  NOT NULL DEFAULT '',
    region              VARCHAR(64)  NOT NULL DEFAULT '',
    network_zone        VARCHAR(128) NOT NULL DEFAULT '',
    capabilities        JSONB        NOT NULL DEFAULT '[]'::jsonb,
    allowed_command_ids JSONB        NOT NULL DEFAULT '[]'::jsonb,
    max_risk_level      VARCHAR(16)  NOT NULL DEFAULT 'high',
    enabled             BOOLEAN      NOT NULL DEFAULT TRUE,
    health_status       VARCHAR(16)  NOT NULL DEFAULT 'unknown',
    description         VARCHAR(512) NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL,
    updated_at          TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exec_medium_enabled_env
    ON exec_medium(enabled, environment);

COMMENT ON TABLE exec_medium IS 'Execution medium (jumpbox, target_host, etc.)';

CREATE TABLE IF NOT EXISTS exec_agent (
    id              BIGSERIAL    PRIMARY KEY,
    agent_id        VARCHAR(64)  NOT NULL UNIQUE,
    medium_id       VARCHAR(64)  NOT NULL,
    status          VARCHAR(16)  NOT NULL DEFAULT 'registered',
    public_key      TEXT         NOT NULL DEFAULT '',
    token_hash      VARCHAR(128) NOT NULL DEFAULT '',
    version         VARCHAR(32)  NOT NULL DEFAULT '',
    capabilities    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    running_tasks   INT          NOT NULL DEFAULT 0,
    free_slots      INT          NOT NULL DEFAULT 1,
    last_heartbeat  TIMESTAMPTZ,
    disabled        BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exec_agent_medium_status
    ON exec_agent(medium_id, status);

COMMENT ON TABLE exec_agent IS 'Execution agent registered on a medium';

CREATE TABLE IF NOT EXISTS exec_command_spec (
    id                 BIGSERIAL    PRIMARY KEY,
    command_spec_id    VARCHAR(64)  NOT NULL UNIQUE,
    name               VARCHAR(128) NOT NULL,
    action_type        VARCHAR(32)  NOT NULL DEFAULT 'diagnose',
    medium_types       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    risk_level         VARCHAR(16)  NOT NULL DEFAULT 'low',
    command_template   TEXT         NOT NULL,
    argument_schema    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    timeout_seconds    INT          NOT NULL DEFAULT 30,
    allowed_exit_codes JSONB        NOT NULL DEFAULT '[0]'::jsonb,
    output_redaction   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    required_caps      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    enabled            BOOLEAN      NOT NULL DEFAULT TRUE,
    description        VARCHAR(512) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ  NOT NULL,
    updated_at         TIMESTAMPTZ  NOT NULL
);

COMMENT ON TABLE exec_command_spec IS 'Controlled command template with argument schema';

CREATE TABLE IF NOT EXISTS exec_lease (
    id           BIGSERIAL    PRIMARY KEY,
    lease_id     VARCHAR(64)  NOT NULL UNIQUE,
    task_id      VARCHAR(36)  NOT NULL,
    step_id      VARCHAR(36)  NOT NULL,
    agent_id     VARCHAR(64)  NOT NULL,
    medium_id    VARCHAR(64)  NOT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'active',
    expires_at   TIMESTAMPTZ  NOT NULL,
    released_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exec_lease_task_status
    ON exec_lease(task_id, status);

CREATE INDEX IF NOT EXISTS idx_exec_lease_agent
    ON exec_lease(agent_id, status);

COMMENT ON TABLE exec_lease IS 'Short-term execution lease to prevent duplicate agent pickup';

CREATE TABLE IF NOT EXISTS exec_log_stream (
    id          BIGSERIAL    PRIMARY KEY,
    log_id      VARCHAR(64)  NOT NULL UNIQUE,
    lease_id    VARCHAR(64)  NOT NULL,
    task_id     VARCHAR(36)  NOT NULL,
    step_id     VARCHAR(36)  NOT NULL,
    agent_id    VARCHAR(64)  NOT NULL,
    stream      VARCHAR(16)  NOT NULL,
    sequence    INT          NOT NULL,
    content     TEXT         NOT NULL DEFAULT '',
    truncated   BOOLEAN      NOT NULL DEFAULT FALSE,
    redacted    BOOLEAN      NOT NULL DEFAULT FALSE,
    observed_at TIMESTAMPTZ  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL,
    updated_at  TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exec_log_stream_task_seq
    ON exec_log_stream(task_id, step_id, sequence);

COMMENT ON TABLE exec_log_stream IS 'Agent stdout/stderr log stream with server-side redaction';

ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS execution_mode VARCHAR(16) NOT NULL DEFAULT 'simulated';
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS medium_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS agent_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS dispatch_status VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS lease_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE exec_task ADD COLUMN IF NOT EXISTS command_spec_id VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS command_spec_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS command_template TEXT NOT NULL DEFAULT '';
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS arguments JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS output_redaction JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS working_dir VARCHAR(256) NOT NULL DEFAULT '';
ALTER TABLE exec_step ADD COLUMN IF NOT EXISTS requires_tty BOOLEAN NOT NULL DEFAULT FALSE;

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000060', 'app:executions:media:read', 'Read execution media', 'executions', 'media:read', 'List and view execution media', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000061', 'app:executions:media:create', 'Create execution media', 'executions', 'media:create', 'Register execution media', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000062', 'app:executions:media:update', 'Update execution media', 'executions', 'media:update', 'Update execution media', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000063', 'app:executions:media:delete', 'Delete execution media', 'executions', 'media:delete', 'Disable or delete execution media', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000064', 'app:executions:agents:manage', 'Manage execution agents', 'executions', 'agents:manage', 'Register and manage execution agents', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000065', 'app:executions:command_specs:read', 'Read command specs', 'executions', 'command_specs:read', 'View controlled command specifications', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000066', 'app:executions:command_specs:create', 'Create command specs', 'executions', 'command_specs:create', 'Create controlled command specifications', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000067', 'app:executions:command_specs:update', 'Update command specs', 'executions', 'command_specs:update', 'Update controlled command specifications', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000068', 'app:executions:command_specs:delete', 'Delete command specs', 'executions', 'command_specs:delete', 'Disable controlled command specifications', NOW(), NOW())
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
    'app:executions:media:read',
    'app:executions:media:create',
    'app:executions:media:update',
    'app:executions:media:delete',
    'app:executions:agents:manage',
    'app:executions:command_specs:read',
    'app:executions:command_specs:create',
    'app:executions:command_specs:update',
    'app:executions:command_specs:delete'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_ai_tool_permission (tool_permission_id, tool_code, tool_name, permission_mode, allow_confirm, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0003-000000000012', 'execution.media.list', 'List execution media', 'read_only', FALSE, 'Read-only list of execution media and health', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000013', 'execution.tasks.propose', 'Propose execution task', 'require_confirm', TRUE, 'Propose pending-confirm execution task from recommendation', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000014', 'execution.tasks.dispatch', 'Dispatch execution task', 'deny', FALSE, 'Agent dispatch is denied for AI tools', NOW(), NOW())
ON CONFLICT (tool_code) DO UPDATE SET
    tool_name = EXCLUDED.tool_name,
    permission_mode = EXCLUDED.permission_mode,
    allow_confirm = EXCLUDED.allow_confirm,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_ai_tool_permission (role_id, tool_permission_id, created_at, updated_at)
SELECT r.role_id, tp.tool_permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_ai_tool_permission tp ON tp.tool_code IN ('execution.media.list', 'execution.tasks.propose')
WHERE r.code = 'admin'
ON CONFLICT (role_id, tool_permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO exec_command_spec (
    command_spec_id, name, action_type, medium_types, risk_level, command_template,
    argument_schema, timeout_seconds, allowed_exit_codes, output_redaction, required_caps,
    enabled, description, created_at, updated_at
)
VALUES
    (
        'cmd_linux_disk_usage',
        '检查磁盘使用率',
        'diagnose',
        '["jumpbox","target_host"]'::jsonb,
        'low',
        'df -h {mount_point}',
        '{"type":"object","properties":{"mount_point":{"type":"string","pattern":"^/[A-Za-z0-9_./-]*$"}},"required":["mount_point"]}'::jsonb,
        10,
        '[0]'::jsonb,
        '{"patterns":["(?i)password=.*","(?i)token=.*"]}'::jsonb,
        '["linux.command.readonly"]'::jsonb,
        TRUE,
        'Read-only disk usage check',
        NOW(), NOW()
    ),
    (
        'cmd_linux_memory_snapshot',
        '内存摘要',
        'diagnose',
        '["jumpbox","target_host"]'::jsonb,
        'low',
        'free -m',
        '{}'::jsonb,
        10,
        '[0]'::jsonb,
        '{"patterns":["(?i)password=.*","(?i)token=.*"]}'::jsonb,
        '["linux.command.readonly"]'::jsonb,
        TRUE,
        'Read-only memory snapshot',
        NOW(), NOW()
    ),
    (
        'cmd_linux_cpu_snapshot',
        'CPU 快照',
        'diagnose',
        '["jumpbox","target_host"]'::jsonb,
        'low',
        'uptime',
        '{}'::jsonb,
        10,
        '[0]'::jsonb,
        '{"patterns":["(?i)password=.*","(?i)token=.*"]}'::jsonb,
        '["linux.command.readonly"]'::jsonb,
        TRUE,
        'Read-only CPU/load snapshot',
        NOW(), NOW()
    )
ON CONFLICT (command_spec_id) DO UPDATE SET
    name = EXCLUDED.name,
    medium_types = EXCLUDED.medium_types,
    risk_level = EXCLUDED.risk_level,
    command_template = EXCLUDED.command_template,
    argument_schema = EXCLUDED.argument_schema,
    timeout_seconds = EXCLUDED.timeout_seconds,
    output_redaction = EXCLUDED.output_redaction,
    required_caps = EXCLUDED.required_caps,
    enabled = EXCLUDED.enabled,
    description = EXCLUDED.description,
    updated_at = NOW();
