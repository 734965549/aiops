-- 0060: Observability 证据快照。
-- 旧行保留 summary-only 语义；新增可空 payload/query_params/window/collected_at，不用空对象伪造历史快照。
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS trace_id VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS evidence_type VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS provider VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS collector VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS query_params JSONB;
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS resource_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS cloud_resource_id VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS window_start TIMESTAMPTZ;
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS window_end TIMESTAMPTZ;
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS collected_at TIMESTAMPTZ;
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'ok';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS reason VARCHAR(512) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS payload JSONB;
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS payload_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS schema_version VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS truncated BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE obs_evidence_ref ADD COLUMN IF NOT EXISTS original_count INTEGER;

UPDATE obs_evidence_ref
SET evidence_type = query_type
WHERE evidence_type = '';

CREATE INDEX IF NOT EXISTS idx_obs_evidence_trace_created
    ON obs_evidence_ref(trace_id, created_at DESC) WHERE trace_id <> '';

COMMENT ON COLUMN obs_evidence_ref.trace_id IS '触发证据采集的请求或任务 trace_id';
COMMENT ON COLUMN obs_evidence_ref.evidence_type IS '证据类型，例如 metrics / logs / alerts';
COMMENT ON COLUMN obs_evidence_ref.collector IS '定稿采集器及版本，例如 ai.metric_evidence.v1';
COMMENT ON COLUMN obs_evidence_ref.query_params IS '脱敏后的完整查询参数；历史摘要证据可为 NULL';
COMMENT ON COLUMN obs_evidence_ref.window_start IS '证据业务窗口起点；历史摘要证据可为 NULL';
COMMENT ON COLUMN obs_evidence_ref.window_end IS '证据业务窗口终点；历史摘要证据可为 NULL';
COMMENT ON COLUMN obs_evidence_ref.collected_at IS '实际完成采集时间，不等同于分析请求时间';
COMMENT ON COLUMN obs_evidence_ref.status IS 'collecting / ok / skipped / permission_denied / unsupported / error';
COMMENT ON COLUMN obs_evidence_ref.payload IS '实际参与 AI 分析的脱敏定稿快照；历史摘要证据为 NULL';
COMMENT ON COLUMN obs_evidence_ref.payload_hash IS '定稿 payload 规范化 JSON 的 SHA-256';
COMMENT ON COLUMN obs_evidence_ref.schema_version IS 'payload 结构版本，例如 ai.metric_evidence.v1';
COMMENT ON COLUMN obs_evidence_ref.truncated IS '定稿前是否发生降采样或大小裁剪';
COMMENT ON COLUMN obs_evidence_ref.original_count IS '裁剪前数据点总数；历史摘要证据为 NULL';
