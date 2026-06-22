-- 0019 initialize Observability context (query evidence refs + permissions).

CREATE TABLE IF NOT EXISTS obs_evidence_ref (
    id          BIGSERIAL    PRIMARY KEY,
    evidence_id VARCHAR(64)  NOT NULL UNIQUE,
    account_id  VARCHAR(64)  NOT NULL,
    query_type  VARCHAR(32)  NOT NULL,
    query_hash  VARCHAR(64)  NOT NULL DEFAULT '',
    summary     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ  NOT NULL,
    updated_at  TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_obs_evidence_account_created
    ON obs_evidence_ref(account_id, created_at DESC);

COMMENT ON TABLE obs_evidence_ref IS 'Observability query evidence references for inspection and audit';
COMMENT ON COLUMN obs_evidence_ref.evidence_id IS 'Business evidence identifier returned via API';
COMMENT ON COLUMN obs_evidence_ref.query_hash IS 'SHA256 hash of normalized query parameters';
COMMENT ON COLUMN obs_evidence_ref.summary IS 'Redacted query result summary, never raw secrets or full logs';

INSERT INTO iam_permission (permission_id, code, name, resource, action, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0001-000000000042', 'app:observability:read', 'Read observability queries', 'observability', 'read', 'Query metrics, logs, traces and topology via provider ports', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    resource = EXCLUDED.resource,
    action = EXCLUDED.action,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_permission p ON p.code = 'app:observability:read'
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();
