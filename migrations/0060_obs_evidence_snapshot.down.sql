-- 0060 down：人工回滚参考，不由 runner 自动执行。
-- 删除 payload/query_params 等列会永久丢失证据快照，执行前必须备份 obs_evidence_ref。
DROP INDEX IF EXISTS idx_obs_evidence_trace_created;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS original_count;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS truncated;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS schema_version;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS payload_hash;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS payload;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS reason;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS status;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS collected_at;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS window_end;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS window_start;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS cloud_resource_id;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS resource_id;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS query_params;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS collector;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS provider;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS evidence_type;
ALTER TABLE obs_evidence_ref DROP COLUMN IF EXISTS trace_id;
