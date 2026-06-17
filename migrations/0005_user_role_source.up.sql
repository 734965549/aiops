-- 0005 user-role binding source.

ALTER TABLE iam_user_role
    ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'manual';

UPDATE iam_user_role SET source = 'manual' WHERE source IS NULL OR source = '';

CREATE INDEX IF NOT EXISTS idx_iam_user_role_user_source ON iam_user_role(user_id, source);

COMMENT ON COLUMN iam_user_role.source IS 'Role binding source: manual/bootstrap, ldap_import, or external_group';
