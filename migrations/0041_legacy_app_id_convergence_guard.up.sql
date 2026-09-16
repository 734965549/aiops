-- 0041 legacy cloud application_id convergence guard.
-- 背景：0032 破坏性 DELETE 删除旧格式 cloud-<account_id> 应用，0037 修复并存合并/安全重命名。
-- 升级到本版本后，asset_application 中不应再存在任何 cloud-<account_id> 格式的 legacy 应用。
-- 若仍存在，说明 0032/0037 收敛失败或有代码路径仍在创建旧格式应用，必须排查修复后才能继续。
-- 本迁移使用 CHECK 约束实现硬阻断：若 legacy 应用数 > 0，INSERT 违反 CHECK(n=0) 导致迁移失败，
-- runner 不会记录版本号，下次运行会重试。兼容自研 SQL splitter（无 $$ 美元引用，纯 DML）。

CREATE TEMP TABLE _0041_guard (n int NOT NULL CHECK (n = 0));

INSERT INTO _0041_guard
SELECT count(*)
FROM asset_application aa
JOIN integration_account ia
  ON aa.application_id = 'cloud-' || trim(ia.account_id);

DROP TABLE _0041_guard;
