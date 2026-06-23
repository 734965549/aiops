-- 0017 repair default admin entry and make admin a permission superset.
-- This migration is intentionally idempotent so databases that already applied
-- an earlier 0016 still receive the final admin bootstrap shape.

INSERT INTO iam_user (user_id, username, display_name, email, password_hash, status, created_at, updated_at)
VALUES
    (
        '00000000-0000-0000-0004-000000000001',
        'admin',
        'Administrator',
        '',
        '$2a$12$F7GsOLVCz95PtwnBN6CKSeZ6vi905sptZIOtx9ffbFZXpPHJx2mKq',
        'active',
        NOW(),
        NOW()
    )
ON CONFLICT (username) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    password_hash = EXCLUDED.password_hash,
    status = EXCLUDED.status,
    updated_at = NOW();

INSERT INTO iam_user_role (user_id, role_id, source, created_at, updated_at)
SELECT u.user_id, r.role_id, 'bootstrap', NOW(), NOW()
FROM iam_user u
JOIN iam_role r ON r.code = 'admin'
WHERE u.username = 'admin'
ON CONFLICT (user_id, role_id) DO UPDATE SET
    source = EXCLUDED.source,
    updated_at = NOW();

INSERT INTO iam_role_permission (role_id, permission_id, created_at, updated_at)
SELECT r.role_id, p.permission_id, NOW(), NOW()
FROM iam_role r
CROSS JOIN iam_permission p
WHERE r.code = 'admin'
ON CONFLICT (role_id, permission_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_role_data_scope (role_id, data_scope_id, created_at, updated_at)
SELECT r.role_id, ds.data_scope_id, NOW(), NOW()
FROM iam_role r
CROSS JOIN iam_data_scope ds
WHERE r.code = 'admin'
ON CONFLICT (role_id, data_scope_id) DO UPDATE SET updated_at = NOW();

INSERT INTO iam_role_ai_tool_permission (role_id, tool_permission_id, created_at, updated_at)
SELECT r.role_id, tp.tool_permission_id, NOW(), NOW()
FROM iam_role r
CROSS JOIN iam_ai_tool_permission tp
WHERE r.code = 'admin'
ON CONFLICT (role_id, tool_permission_id) DO UPDATE SET updated_at = NOW();
