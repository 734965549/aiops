# pkg/database 模块说明

`database` 提供 PostgreSQL(GORM)/Redis 连接封装、统一模型基类、PG 错误映射、GORM 日志桥接与**自研迁移 runner**。只关注连接与通用机制；业务仓储在 `internal/<context>` 内注入 `*gorm.DB`。

## 连接

- `NewPostgres(ctx, cfg, tz)`：GORM 打开后立即 5s 超时 ping；`PrepareStmt=true`、`SkipDefaultTransaction=true`（未显式开事务的路径省去隐式 begin/commit）；连接池 `max_idle_conns/max_open_conns/conn_max_lifetime` 按配置生效；DSN 由配置 + 时区生成。
- `NewRedis(ctx, cfg)`：3s 超时 ping，失败即关闭连接返回错误。
- GORM 日志桥接（`gorm_logger.go`）：经 `pkg/logger` 输出（不直接打 stdout）；慢查询阈值 200ms；级别来自 `database.log_level`，默认不输出原始 SQL。

## BaseModel（`model.go`）

- `ID`（BIGSERIAL 自增主键）与业务标识列严格分离；`CreatedAt/UpdatedAt` 由 GORM 钩子在程序内赋值，不用 DB DEFAULT/触发器。
- `BeforeCreate`：显式传入的 `CreatedAt` 保留（补数据不破坏历史时间线），仅补齐 `UpdatedAt`；`BeforeUpdate` 永远刷新 `UpdatedAt`。
- 钩子只走 GORM Statement 写入路径（Create/Save/Updates）；`db.Exec` 直写会绕过钩子，确需使用时必须手动维护时间戳并说明原因（项目宪章）。

## PG 错误映射（`pgerrors.go`）

`IsUniqueViolation`（23505）与 `MapUniqueViolation(err, target)` 供仓储把唯一冲突翻译为业务错误。

## 迁移 runner（`migrate.go`，平台唯一允许的迁移工具）

- `ResolveMigrationDir`：优先二进制同级 `./migrations`（容器布局），其次工作目录（go run）。
- 文件名 `^(\d{4,})_name\.up\.sql$`，按版本字符串升序；只执行 `up`，`*.down.sql` 仅为人工回滚参考。
- 账本表 `schema_migrations(version, name, checksum, applied_at DEFAULT NOW())`；缺 checksum 列时自动补列。
- `RunMigrations`：建表 → 扫描 → 比对已应用 → **checksum 校验**（存储为空则 backfill；与当前文件不一致直接报错终止，fail-closed）→ 逐个在**单事务**内执行分句 SQL 并插账本。
- `TargetVersion` 可截断 pending（按整数比较，兼容 10000 vs 9999 字典序陷阱）。
- `ReadMigrationStatus`：只读状态（latest/applied/pending/up_to_date + `computeChecksumDrifts` 漂移列表），供 `/readyz` 展示；空 checksum 也算漂移，不做 backfill、不返回 error。
- **SQL 分句边界**：`splitSQLStatements` 只剔除整行 `--` 注释，并识别单引号字符串（含 `''` 转义）；**行尾 `--` 注释、块注释 `/* */` 或 dollar-quoted（`$$`）字符串内的分号会导致错误切分**——迁移文件必须避免这几类写法（契约 `ops/migration-contract.md` 已登记，`0041`/`0042` 守卫即为兼容该 splitter 的纯 DML 写法）。

## 测试

`migrate_test.go`/`migrate_upgrade_test.go` 覆盖扫描、分句、checksum 与升级路径；`gorm_logger_test.go` 覆盖日志级别。修改 runner 或 SQL splitter 必须跑 `go test ./pkg/database/...`（项目宪章）。
