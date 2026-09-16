-- 0030 asset sync batch fencing token.
-- 背景：0028 引入 running 租约后，若续租遇到临时 DB 错误直接停止心跳，主同步仍可能继续 upsert/stale；
-- TTL 到期后新请求可 reap 旧批次并启动新批次，旧任务写入会破坏账号级互斥语义。
-- 解决：为每个批次增加 fencing_token，续租和写前校验必须匹配 batch_id + token + running + 未过期租约。

ALTER TABLE asset_sync_batch
    ADD COLUMN IF NOT EXISTS fencing_token VARCHAR(64);

UPDATE asset_sync_batch
SET fencing_token = batch_id
WHERE fencing_token IS NULL OR fencing_token = '';

ALTER TABLE asset_sync_batch
    ALTER COLUMN fencing_token SET NOT NULL,
    ALTER COLUMN fencing_token SET DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_asset_sync_batch_fencing_running
    ON asset_sync_batch(batch_id, fencing_token, lease_expires_at)
    WHERE status = 'running';

COMMENT ON COLUMN asset_sync_batch.fencing_token IS '同步批次租约所有权令牌；续租与写前校验必须匹配，用于防止旧任务在租约丢失后继续写入';
