-- 0016 seed default local admin user for controlled initialization.
-- Password is bcrypt(admin123), generated with pkg/auth default cost 12.

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
ON CONFLICT (user_id, role_id) DO NOTHING;
