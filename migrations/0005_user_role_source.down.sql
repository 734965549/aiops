DROP INDEX IF EXISTS idx_iam_user_role_user_source;

ALTER TABLE iam_user_role DROP COLUMN IF EXISTS source;
