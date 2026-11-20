-- 0059 down：人工回滚参考，不由 runner 自动执行。
-- 删除图片列会丢失用户消息附图，执行前请备份 ai_session_message。
ALTER TABLE ai_session_message DROP COLUMN IF EXISTS images_json;
ALTER TABLE ai_provider DROP COLUMN IF EXISTS vision_enabled;
