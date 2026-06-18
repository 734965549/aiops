# 数据库迁移契约

本文档面向平台团队、DBA 与联调方，描述平台的数据库迁移使用方式与责任边界。

## 迁移版本体系

平台只维护一套迁移版本体系：仓库 `migrations/` 目录下的版本化 SQL，以及 `schema_migrations` 表中的已应用版本记录。

| 允许 | 禁止 |
| --- | --- |
| 生产 DBA 按顺序手工执行 `migrations/*.up.sql` 并执行 `migrations/manual_schema_migrations.sql` | [golang-migrate](https://github.com/golang-migrate/migrate) CLI 或库 |
| dev/test 使用 `make migrate` | 任何第三方迁移工具或另一套迁移元数据表 |
| dev/test 使用 `go run ./cmd/migrate -config <path>` | PostgreSQL `docker-entrypoint-initdb.d` 与应用启动自动迁移混用 |
| 受控 dev/test 下 `database.auto_migrate=true`（仍走同一版本体系） | 同一环境对同一库交替使用多种迁移版本体系 |

**禁止混用**：同一数据库实例不得交替使用 golang-migrate（或其它外部工具）与本仓库 `migrations/` 版本体系。`schema_migrations` 表结构、版本号语义与幂等逻辑均以本仓库为准。

## 责任边界

- **生产环境**：数据库初始化和迁移必须由 **DBA** 在部署 API **之前**显式执行。DBA 按 `migrations/README.md` 中的顺序执行 `migrations/*.up.sql`，最后执行 `migrations/manual_schema_migrations.sql` 写入账本，`database.auto_migrate` 保持 `false`。
- **应用启动**：`cmd/api` 默认不执行迁移，只连接数据库并读取迁移状态供 `/readyz` 判读。
- `database.auto_migrate=true` 仅限本地或测试环境临时便利，仍调用同一 `RunMigrations` 实现，**不得**作为生产默认策略。

## 执行顺序与 bootstrap 分工

迁移脚本**必须按版本号顺序**执行，当前依赖关系：

```text
0001_init_identity.up.sql              → 建表（iam_user、iam_role、iam_permission 等）
0002_seed_admin_permissions.up.sql     → 种子 admin 角色、基础权限、数据范围、AI 工具权限
0003_external_identity.up.sql          → iam_external_identity（外部身份绑定）
0004_user_provisioning_permissions.up.sql → 管理员用户预置 / LDAP 导入权限
0016_seed_default_admin_user.up.sql    → 种子默认本地管理员 admin/admin123，并把 admin 角色绑定为当前权限全集
0017_repair_default_admin_superset.up.sql → 修复已应用旧 0016 的环境，重新确保 admin 入口和权限全集
```

| 职责 | 0001 / 0002 / 0016 迁移 | 启动期 bootstrap（`cmd/api`） |
| --- | --- | --- |
| 表结构 | ✅ 0001 | — |
| admin 角色及权限集合 | ✅ 0002 | — |
| 默认管理员用户账号 | ✅ 0016（`admin/admin123`） | ✅ `EnsureBootstrapUser`（读 `auth.bootstrap_*` 配置，兼容旧 dev 启动链路） |
| 用户与 admin 角色绑定 | ✅ 0016 | ✅ `ensureBootstrapAdminRole`（幂等写入 `iam_user_role`） |
| admin 超集授权 | ✅ 0016（绑定所有已存在权限、数据范围、AI 工具权限） | — |

**要点**：

- `0002` 只创建 `admin` 角色及其权限绑定，**不创建用户**。
- `0016` 直接 upsert 默认本地管理员 `admin/admin123` 并绑定 `admin` 角色；如果库中已存在 `admin` 用户，会将其密码重置为 `admin123` 并启用该账号。
- `0016` 会把 `admin` 角色绑定到执行到当前版本时所有已存在的 `iam_permission`、`iam_data_scope`、`iam_ai_tool_permission`，作为 DBA 初始化后的入口超集账号。
- `0017` 是兼容修复迁移：如果某个环境已经应用过早期 `0016`，runner 不会重跑 `0016`，因此必须通过 `0017` 再次补齐默认 admin 和权限全集。
- 启动期 bootstrap 仍保留，用于 dev/test 或旧数据库兼容；生产环境 bootstrap 配置必须留空，且上线后必须改密或禁用默认账号。
- 若只执行 `0001` 而未执行 `0002`，bootstrap 绑定 admin 角色时会因角色不存在而失败。
- 若未执行 `0004`，管理员无法使用域账号导入等 Admin 接口（403）。
- 各 `*.up.sql` 内部使用 `ON CONFLICT` 保证幂等，但**版本顺序不可打乱**（0002 依赖 0001 表结构，0003/0004 依赖 0001/0002）。

## 执行命令

生产 DBA 手工模式：

```bash
psql "$AIOPS_DATABASE_DSN" -f migrations/0001_init_identity.up.sql
psql "$AIOPS_DATABASE_DSN" -f migrations/0002_seed_admin_permissions.up.sql
# ...按 migrations/README.md 顺序继续执行...
psql "$AIOPS_DATABASE_DSN" -f migrations/0017_repair_default_admin_superset.up.sql
psql "$AIOPS_DATABASE_DSN" -f migrations/manual_schema_migrations.sql
```

本地联调（PostgreSQL 已启动）：

```bash
# 推荐：Makefile 封装
make migrate

# 等价：直接 go run
go run ./cmd/migrate -config configs/config.yaml
```

受控 dev/test 流水线示例：

```bash
go run ./cmd/migrate -config /path/to/config.yaml
# 或先 make build 后由运维脚本在目标环境执行等价迁移步骤
```

容器化 dev 可选覆盖（仍走自研 runner，经 `bootstrap.Init` 触发）：

```bash
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
```

上述覆盖将 `AIOPS_DATABASE__AUTO_MIGRATE=true` 注入 `api` 服务，与 `make migrate` **二选一**，避免重复执行。

## 远程 RDS / PgBouncer 兼容

部分托管 PostgreSQL（如阿里云 RDS、经 PgBouncer 代理的连接）在 **extended query / prepared statement** 模式下，单条 `Exec` 内嵌多条 SQL 会返回 `SQLSTATE 42601`（`cannot insert multiple commands into a prepared statement`）。

自研 runner 对此的处理：

- 每个 `*.up.sql` 在事务内**按语句逐条**执行（`splitSQLStatements`），并关闭 `PrepareStmt`；
- 语句分隔以分号为准，忽略整行 `--` 注释，单引号字符串内的分号不拆分（支持 PostgreSQL `''` 转义）；
- **不支持**美元引用（`$tag$...$tag$`）或函数/触发器体内的多语句块——若未来需要，应拆成独立迁移文件或扩展 splitter。

`cmd/migrate` 超时读取 `database.migrate_timeout_s`（默认 300s），远程高延迟库联调时可适当调大。

单元测试：`pkg/database/migrate_test.go`（含对仓库内全部 `*.up.sql` 的拆分冒烟）。

## 回滚原则

- `*.down.sql` 仅作为人工回滚参考。
- 生产回滚应由 DBA 按审批流程显式执行，避免误删数据。
- 自研 runner 当前仅处理 `*.up.sql`；回滚不通过 runner 自动执行。
- `0001_init_identity.down.sql` 末尾会 `DELETE FROM schema_migrations WHERE version IN ('0001','0002')`，便于 dev 环境完整 down 后重新 up。
- `0002_seed_admin_permissions.down.sql` 按 `code` / `tool_code` 删除种子数据，并移除 `schema_migrations` 中 `0002` 记录。

## 命名规则

- 文件名格式：`<version>_<name>.up.sql` / `<version>_<name>.down.sql`
- 版本号建议使用 4 位递增编号：`0001`、`0002`、…
- 同一限界上下文的表名保持一致前缀：`iam_*`、`alert_*`、`asset_*`、`exec_*`、`audit_*`

## schema_migrations 表

自研 runner 启动时自动创建：

| 列 | 说明 |
| --- | --- |
| `version` | 主键，4 位版本号 |
| `name` | 迁移名称 |
| `applied_at` | 应用时间 |

**建表规范例外**：平台要求业务表 `created_at` / `updated_at` 由 Go 程序维护、禁止 DB DEFAULT；`schema_migrations.applied_at` 为 runner 内部元数据，表定义保留 `DEFAULT NOW()` 作为兜底，但 runner 在 `INSERT` 时**显式传入** `time.Now()`，与业务表约定区分。

## 已落地迁移

### `0001_init_identity`

Identity 初始化迁移，包含以下表结构：

| 表名 | 说明 |
| --- | --- |
| `iam_user` | 用户主表 |
| `iam_role` | 角色主表 |
| `iam_permission` | 权限主表 |
| `iam_user_role` | 用户角色关联表 |
| `iam_role_permission` | 角色权限关联表 |
| `iam_data_scope` | 数据范围主表 |
| `iam_role_data_scope` | 角色数据范围关联表 |
| `iam_ai_tool_permission` | AI 工具权限主表 |
| `iam_role_ai_tool_permission` | 角色 AI 工具权限关联表 |

该迁移遵循平台建表规范：DB 主键统一为 `BIGSERIAL id`，业务标识独立唯一列，`created_at` / `updated_at` 由 Go 程序维护，不设置 DB 默认值或触发器。

### `0002_seed_admin_permissions`

种子数据迁移，**不负责创建用户**：

| 对象 | 说明 |
| --- | --- |
| `iam_role`（code=`admin`） | 系统内置管理员角色 |
| `iam_permission` | Identity 只读、授权校验、AI provider 管理、AI 工具调用等基础权限 |
| `iam_data_scope`（code=`all-data`） | 全部数据范围 |
| `iam_ai_tool_permission` | 告警分析、指标查询、日志检索、执行预案等工具权限 |
| 关联表 | 将上述权限绑定到 `admin` 角色 |

默认管理员**用户**由 `0016_seed_default_admin_user` 种子；`cmd/api` 启动期 bootstrap 仍保留为 dev/test 兼容链路，见上文「执行顺序与 bootstrap 分工」。

### `0003_external_identity`

| 对象 | 说明 |
| --- | --- |
| `iam_external_identity` | LDAP DN / OIDC `sub` 与平台 `user_id` 的绑定；外部登录仅认此表预置记录 |

### `0004_user_provisioning_permissions`

| 对象 | 说明 |
| --- | --- |
| `iam_permission` | `app:identity.users:create`、`app:identity.external_identities:create` |
| `iam_role_permission` | 将上述权限绑定到 `admin` 角色 |

用于管理员创建本地用户、预置域账号绑定、LDAP 目录浏览与批量导入（前端 `/identity/ldap-import`）。

### `0011_init_execution`

| 对象 | 说明 |
| --- | --- |
| `exec_task` / `exec_step` | 执行任务主表与步骤表 |
| `iam_permission` | `app:executions:read/create/confirm/execute` |
| `iam_role_permission` | 绑定到 `admin` 角色 |

### `0016_seed_default_admin_user`

| 对象 | 说明 |
| --- | --- |
| `iam_user` | upsert 默认本地管理员 `admin/admin123`，密码以 bcrypt hash 存储 |
| `iam_user_role` | 将 `admin` 用户绑定到内置 `admin` 角色 |
| `iam_role_permission` | 将内置 `admin` 角色绑定到所有已存在权限 |
| `iam_role_data_scope` | 将内置 `admin` 角色绑定到所有已存在数据范围 |
| `iam_role_ai_tool_permission` | 将内置 `admin` 角色绑定到所有已存在 AI 工具权限 |

该迁移用于受控初始化和联调兜底。若目标环境已有 `admin` 用户，迁移会将其本地密码重置为 `admin123` 并设为 `active`。DBA 初始化必须按顺序执行到 `0016`，只执行 `0001/0002` 仍不是完整可用系统；生产上线后必须立即改密或禁用默认账号。

### `0017_repair_default_admin_superset`

`0017` 重复执行 `0016` 的最终形态：创建/重置 `admin/admin123`、绑定 `admin` 角色，并把 `admin` 角色绑定到所有已存在权限、数据范围和 AI 工具权限。它用于修复已经记录旧版 `0016` 的数据库；新库按顺序执行时同样幂等。

## 运行期判读

迁移状态的结构化结果与 readiness 语义已统一到 `ops/health-contract.md`，用于联调与探针判读。

## 前端联调常见症状

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| 登录成功，Authed 接口 403（含域账号导入页） | 未执行 `0002`、`0004`、`0016` 或旧 token 未刷新 | `make migrate` 追平至最新版本，并重新登录 |
| API 启动即退出（`ensure bootstrap admin role failed`） | 仅执行 `0001` 且 bootstrap 已配置，admin 角色不存在 | 同上，须先执行 `0002` |
| Dashboard `/readyz` 显示迁移未追平 | pending `0002` 或更高版本 | 同上 |
| 401 循环跳转登录 | JWT 无效或未注入 `JWT_SECRET` | 检查 `auth.jwt_secret` 与登录响应 |

前端在 403 时会提示检查迁移 `0002`；可通过 `GET /readyz` 的 `checks[migration]` 确认 pending 数量。
