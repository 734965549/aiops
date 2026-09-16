-- 0044 lock the default admin account seeded by 0016/0017.
--
-- 背景：0016/0017 会种入默认 admin/admin123 账号，并在用户名冲突时把密码重置为
-- admin123 并激活。生产环境即使关闭 bootstrap 也无法避免，导致生产存在已知凭据。
--
-- 本迁移把 username=admin 且 password_hash 仍为已知 admin123 哈希的账号置为 locked
-- 并清空 password_hash，消除 admin/admin123 可用性（按用户名匹配，非仅固定种子 user_id）。
-- 仅在 password_hash 仍是已知 admin123 哈希时才锁定，避免覆盖 DBA 已设置的强密码。
-- 账号行保留（不删除），以维持审计 / 告警 / 执行记录对 user_id 的引用完整性。
--
-- 各环境影响：
--   - 生产：bootstrap 关闭，默认 admin 保持锁定、不可登录。API 对外开放前须由受控
--     DBA/发布步骤创建安全管理员（详见 docs/release-checklist.md）。
--   - dev/test：bootstrap 启用时，EnsureBootstrapUser 检测到锁定的默认 admin 会用配置
--     的 bootstrap 密码重新激活，因此 admin/admin123 在 dev 仍可登录，E2E 不受影响。

UPDATE iam_user
SET status = 'locked',
    password_hash = '',
    updated_at = NOW()
WHERE username = 'admin'
  AND password_hash = '$2a$12$F7GsOLVCz95PtwnBN6CKSeZ6vi905sptZIOtx9ffbFZXpPHJx2mKq';
