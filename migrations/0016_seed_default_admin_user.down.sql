-- 0016 rollback reference.
-- Keep this conservative: only remove the binding/user inserted by this migration.

DELETE FROM iam_user_role
WHERE source = 'bootstrap'
  AND user_id IN (SELECT user_id FROM iam_user WHERE username = 'admin')
  AND role_id IN (SELECT role_id FROM iam_role WHERE code = 'admin');

DELETE FROM iam_user
WHERE user_id = '00000000-0000-0000-0004-000000000001'
  AND username = 'admin';

DELETE FROM schema_migrations WHERE version = '0016';
