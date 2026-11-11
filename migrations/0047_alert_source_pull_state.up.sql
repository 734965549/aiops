-- 0047: alert_source 拉取状态补充列。
-- 0046 已在部分环境应用，按迁移规则不改写已发布版本；
-- delivery_mode / sync_state 单独在 0047 补齐。时间戳仍由 Go 维护。

ALTER TABLE alert_source
    ADD COLUMN IF NOT EXISTS delivery_mode VARCHAR(16) NOT NULL DEFAULT 'webhook',
    ADD COLUMN IF NOT EXISTS sync_state JSONB NOT NULL DEFAULT '{}'::jsonb;
