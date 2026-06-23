-- 0017 rollback reference.
-- This migration is a repair/normalization step; do not remove admin's broad
-- grants automatically during production rollback.

DELETE FROM schema_migrations WHERE version = '0017';
