# migrations 模块说明（粤语版）

## 呢个目录做咩

`migrations` 系数据库结构嘅「装修图纸」。代码需要边啲表、边啲字段、边啲索引，都用 SQL 记录低。新环境启动时，可以按图纸自动装修数据库。

## 自研 runner 执行模式

生产 / 预发环境必须由 DBA 或发布流水线在部署 API 前显式执行本仓库自研 runner（`cmd/migrate` 二进制或 `go run ./cmd/migrate -config <path>`）。`AIOPS_DATABASE__AUTO_MIGRATE=false` 时 API 启动不会自动跑 SQL，只读取迁移状态供 `/readyz` 判断。

当前 `*.up.sql` 迁移顺序如下，runner 会按版本号自动执行并维护 `schema_migrations` 账本：

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
0018_init_integration.up.sql
0019_init_observability.up.sql
0020_init_inspection.up.sql
0022_init_execution_agent.up.sql
0023_asset_cloud_sync.up.sql
0024_integration_account_extra_config.up.sql
0025_asset_resource_labels.up.sql
0026_asset_cloud_sync_region_key.up.sql
0027_asset_sync_batch_message_text.up.sql
0028_asset_sync_batch_running_mutex.up.sql
0029_huawei_legacy_accounts_native_sync_mode.up.sql
0030_asset_sync_batch_fencing_token.up.sql
```

说明：

- `*.up.sql` 是真实建表、建索引、种子数据和权限数据。
- `schema_migrations` 是 `/readyz` 判断数据库是否追平当前版本的账本，由自研 runner 维护；如果业务 SQL 已执行但账本没写，应用会认为 migration pending。
- 禁止在生产或预发环境手工 `psql -f migrations/*.up.sql`、手工执行 `manual_schema_migrations.sql`，或手工写入 / 修改 `schema_migrations`。
- `*.down.sql` 只作为人工回滚参考，不由应用自动执行。
- `0016_seed_default_admin_user.up.sql` 是 DBA 初始化入口账号兜底：创建/重置 `admin/admin123`，并把 `admin` 角色绑定到当前全部权限、数据范围和 AI 工具权限。只执行 `0001/0002` 不算完整初始化。
- 当前仓库未包含 `0021` 文件；runner 会按实际文件名顺序执行到 `0030`，唔好自己补一条空账本记录。
- Integration、Observability、Inspection 同 Execution Agent 点样串到主链路，睇 `docs/AI运维平台整体流程与调用关系.md`。

执行命令：

```bash
# 生产 / 预发推荐使用已构建的自研 runner 二进制
./migrate -config /path/to/config.yaml

# 源码方式等价执行
go run ./cmd/migrate -config /path/to/config.yaml

# 本地联调也可使用 Makefile 封装
make migrate
make migrate-up
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
默认值为 `false`：生产 / 预发由 DBA 或发布流水线显式执行自研 runner；本地联调可使用 `make migrate` / `make migrate-up` 显式建表，避免 API 启动时隐式迁移。

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
| 数据库表结构 | 例如 `iam_*`、`alert_*`、`asset_*`、`exec_*`、`integration_*`、`obs_evidence_ref`、`inspection_*`、`schema_migrations` |
| migration 版本记录 | 防止同一条 SQL 重复执行 |
| error | SQL 错、权限不足、连接断开等 |

## 通俗比喻

数据库 migration 就似装修施工记录：今日装咗门，听日铺咗地板，每一步都有编号。以后换地方开分店，只要照住记录施工，就可以装修出同一间铺。
