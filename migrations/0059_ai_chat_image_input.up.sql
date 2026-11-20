-- 0059: AI 助手图片输入（多模态）
-- ai_session_message.images_json：用户消息附图的 data URL 数组
-- （image/png、image/jpeg、image/webp；单张解码后 <= 2MB，每条消息最多 3 张）。
-- ai_provider.vision_enabled：类型 b 模型是否接受图片输入，默认 false；
-- 未开启时带图消息在 application 层返回 400，不会调用模型。
-- 仅新增列，不改写历史数据；时间戳仍由 Go 程序维护。
ALTER TABLE ai_session_message ADD COLUMN IF NOT EXISTS images_json JSONB;
ALTER TABLE ai_provider ADD COLUMN IF NOT EXISTS vision_enabled BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN ai_session_message.images_json IS '用户消息附图 data URL 数组（base64）；NULL 表示无图';
COMMENT ON COLUMN ai_provider.vision_enabled IS '类型 b 模型是否支持图片输入（多模态）';
