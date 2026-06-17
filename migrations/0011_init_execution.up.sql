-- 0011 initialize Execution context.

CREATE TABLE IF NOT EXISTS exec_task (
    id              BIGSERIAL    PRIMARY KEY,
    task_id         VARCHAR(36)  NOT NULL UNIQUE,
    source_type     VARCHAR(32)  NOT NULL,
    source_id       VARCHAR(128) NOT NULL DEFAULT '',
    operation_type  VARCHAR(32)  NOT NULL,
    name            VARCHAR(255) NOT NULL,
    status          VARCHAR(32)  NOT NULL,
    risk_level      VARCHAR(16)  NOT NULL,
    target_type     VARCHAR(64)  NOT NULL DEFAULT '',
    target_id       VARCHAR(128) NOT NULL DEFAULT '',
    target_name     VARCHAR(255) NOT NULL DEFAULT '',
    environment     VARCHAR(64)  NOT NULL DEFAULT '',
    parameters      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    rollback_plan   JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_by      VARCHAR(36)  NOT NULL DEFAULT '',
    confirmed_by    VARCHAR(36)  NOT NULL DEFAULT '',
    executed_by     VARCHAR(36)  NOT NULL DEFAULT '',
    confirmed_at    TIMESTAMPTZ,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    result_summary  TEXT         NOT NULL DEFAULT '',
    error_message   TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_exec_task_status_created ON exec_task(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_exec_task_source ON exec_task(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_exec_task_created_by ON exec_task(created_by);

COMMENT ON TABLE exec_task IS 'Execution context task table';

CREATE TABLE IF NOT EXISTS exec_step (
    id              BIGSERIAL    PRIMARY KEY,
    step_id         VARCHAR(36)  NOT NULL UNIQUE,
    task_id         VARCHAR(36)  NOT NULL,
    step_order      INT          NOT NULL,
    name            VARCHAR(255) NOT NULL,
    action_type     VARCHAR(64)  NOT NULL,
    status          VARCHAR(32)  NOT NULL,
    output          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    error_message   TEXT         NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL,
    updated_at      TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_exec_step_task_order UNIQUE (task_id, step_order)
);

CREATE INDEX IF NOT EXISTS idx_exec_step_task_id ON exec_step(task_id);

COMMENT ON TABLE exec_step IS 'Execution context task step table';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000023', 'app:executions:read', 'Read execution tasks', 'executions', 'read', 'Read execution task list and details', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000024', 'app:executions:create', 'Create execution tasks', 'executions', 'create', 'Create execution task from alert or manual input', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000025', 'app:executions:confirm', 'Confirm execution tasks', 'executions', 'confirm', 'Confirm pending medium/high risk execution tasks', NOW(), NOW()),
    ('00000000-0000-0000-0001-000000000026', 'app:executions:execute', 'Execute execution tasks', 'executions', 'execute', 'Trigger execution task run', NOW(), NOW())
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
    'app:executions:read',
    'app:executions:create',
    'app:executions:confirm',
    'app:executions:execute'
)
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
