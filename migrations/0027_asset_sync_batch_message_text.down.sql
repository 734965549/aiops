-- 回滚 0027：恢复 message 为 VARCHAR(512)。
-- 注意：若已有超过 512 字符的 message，回滚会失败；需先清理超长数据再回滚。

ALTER TABLE asset_sync_batch
    ALTER COLUMN message TYPE VARCHAR(512);
