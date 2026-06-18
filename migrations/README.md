# migrations 模块说明（粤语版）

## 呢个目录做咩

`migrations` 系数据库结构嘅「装修图纸」。代码需要边啲表、边啲字段、边啲索引，都用 SQL 记录低。新环境启动时，可以按图纸自动装修数据库。

## 生产 DBA 手工执行模式

生产环境如果由 DBA 统一执行数据库初始化，则 `AIOPS_DATABASE__AUTO_MIGRATE=false`，API 启动时不会自动跑 SQL。DBA 只需要使用本目录作为统一 SQL 目录，不要从其它目录拼接脚本。

执行顺序固定如下：

```text
0001_init_identity.up.sql
0002_seed_admin_permissions.up.sql
0003_external_identity.up.sql
0004_user_provisioning_permissions.up.sql
0005_user_role_source.up.sql
0006_auth_audit.up.sql
0007_init_alert.up.sql
0008_init_asset.up.sql
0009_init_audit.up.sql
0010_ai_analyze_permission.up.sql
0011_init_execution.up.sql
0012_init_runbook.up.sql
0013_dashboard_permission.up.sql
0014_init_asset_match_rule.up.sql
0015_identity_access_control_management.up.sql
0016_seed_default_admin_user.up.sql
0017_repair_default_admin_superset.up.sql
manual_schema_migrations.sql
```

说明：

- `*.up.sql` 是真实建表、建索引、种子数据和权限数据。
- `manual_schema_migrations.sql` 只用于 DBA 手工模式下补齐迁移账本，必须在前面所有 `*.up.sql` 成功后再执行。
- `schema_migrations` 是 `/readyz` 判断数据库是否追平当前版本的账本；如果业务 SQL 已执行但账本没写，应用会认为 migration pending。
- 不要为了让 `/readyz` 变绿而提前执行 `manual_schema_migrations.sql`。
- `*.down.sql` 只作为人工回滚参考，不由应用自动执行。
- `0016_seed_default_admin_user.up.sql` 是 DBA 初始化入口账号兜底：创建/重置 `admin/admin123`，并把 `admin` 角色绑定到当前全部权限、数据范围和 AI 工具权限。只执行 `0001/0002` 不算完整初始化。

示例：

```bash
psql "$AIOPS_DATABASE_DSN" -f migrations/0001_init_identity.up.sql
psql "$AIOPS_DATABASE_DSN" -f migrations/0002_seed_admin_permissions.up.sql
# ...按上方顺序继续执行...
psql "$AIOPS_DATABASE_DSN" -f migrations/0015_identity_access_control_management.up.sql
psql "$AIOPS_DATABASE_DSN" -f migrations/0017_repair_default_admin_superset.up.sql
psql "$AIOPS_DATABASE_DSN" -f migrations/manual_schema_migrations.sql
```

执行后检查：

```sql
SELECT version, name, applied_at
FROM public.schema_migrations
ORDER BY version;
```

## 调用关系

```text
make migrate / go run ./cmd/migrate
  -> bootstrap.Migrate
  -> database.RunMigrations(ctx, db, MigrateOptions{Dir})
  -> 读取 migrations/*.up.sql
  -> 写入 schema_migrations
  -> PostgreSQL 建表/建索引
```

**仅当 `database.auto_migrate=true` 时**，`bootstrap.Init`（`cmd/api` 启动路径）才会在连接 PostgreSQL 后调用同一 `RunMigrations`。
默认值为 `false`：生产由 DBA 按本文件“生产 DBA 手工执行模式”执行 SQL；本地联调可使用 `make migrate` / `make migrate-up` 显式建表，避免 API 启动时隐式迁移。

受控 dev/test 可临时开启 `auto_migrate`（例如叠加 `deployments/docker-compose.dev.yml`），与 `make migrate` **二选一**。

平台禁止与 golang-migrate 混用，详见 `ops/migration-contract.md`。

回滚文件：

```text
*.down.sql
  -> 描述点样撤销对应 up migration
```

## 入参

| 入参 | 说明 |
| --- | --- |
| migration 目录 | SQL 文件所在目录 |
| `*.up.sql` | 正向迁移，例如建表 |
| `*.down.sql` | 反向迁移，例如删表 |
| PostgreSQL 连接 | 执行 SQL 嘅目标数据库 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 数据库表结构 | 例如 `iam_user`、`iam_role`、`iam_permission`、`iam_user_role`、`iam_role_permission`、`iam_data_scope`、`iam_role_data_scope`、`iam_ai_tool_permission`、`iam_role_ai_tool_permission`、`schema_migrations` |
| migration 版本记录 | 防止同一条 SQL 重复执行 |
| error | SQL 错、权限不足、连接断开等 |

## 通俗比喻

数据库 migration 就似装修施工记录：今日装咗门，听日铺咗地板，每一步都有编号。以后换地方开分店，只要照住记录施工，就可以装修出同一间铺。
