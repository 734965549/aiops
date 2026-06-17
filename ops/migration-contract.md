# 数据库迁移契约

本文档面向平台团队、DBA 与联调方，描述平台的数据库迁移使用方式与责任边界。

## 唯一执行方式

平台**仅**使用仓库内自研迁移 runner（`pkg/database/migrate.go`），通过 `schema_migrations` 表跟踪已应用版本。

| 允许 | 禁止 |
| --- | --- |
| `make migrate` | [golang-migrate](https://github.com/golang-migrate/migrate) CLI 或库 |
| `go run ./cmd/migrate -config <path>` | 任何与自研 runner 并行的第三方迁移工具 |
| CI / 发布流水线调用上述命令 | PostgreSQL `docker-entrypoint-initdb.d` 手工 `\i` 与应用 runner 混用 |
| 受控 dev/test 下 `database.auto_migrate=true`（仍走同一 runner） | 同一环境对同一库交替使用多种迁移执行器 |

**禁止混用**：同一数据库实例不得交替使用 golang-migrate（或其它外部工具）与自研 runner。`schema_migrations` 表结构、版本号语义与幂等逻辑均以自研 runner 为准。

## 责任边界

- **生产环境**：迁移必须由 **DBA / 发布流水线** 在部署 API **之前**显式执行（`make migrate` 或等价 `go run ./cmd/migrate`），`database.auto_migrate` 保持 `false`。
- **应用启动**：`cmd/api` 默认不执行迁移，只连接数据库并读取迁移状态供 `/readyz` 判读。
- `database.auto_migrate=true` 仅限本地或测试环境临时便利，仍调用同一 `RunMigrations` 实现，**不得**作为生产默认策略。

## 执行顺序与 bootstrap 分工

迁移脚本**必须按版本号顺序**执行，当前依赖关系：

```text
0001_init_identity.up.sql              → 建表（iam_user、iam_role、iam_permission 等）
0002_seed_admin_permissions.up.sql     → 种子 admin 角色、基础权限、数据范围、AI 工具权限
0003_external_identity.up.sql          → iam_external_identity（外部身份绑定）
0004_user_provisioning_permissions.up.sql → 管理员用户预置 / LDAP 导入权限
```

| 职责 | 0001 / 0002 迁移 | 启动期 bootstrap（`cmd/api`） |
| --- | --- | --- |
| 表结构 | ✅ 0001 | — |
| admin 角色及权限集合 | ✅ 0002 | — |
| 默认管理员用户账号 | — | ✅ `EnsureBootstrapUser`（读 `auth.bootstrap_*` 配置） |
| 用户与 admin 角色绑定 | — | ✅ `ensureBootstrapAdminRole`（幂等写入 `iam_user_role`） |

**要点**：

- `0002` 只创建 `admin` 角色及其权限绑定，**不创建用户**。
- 默认管理员用户由启动期根据 `auth.bootstrap_username` / `auth.bootstrap_password` 幂等创建；生产环境 bootstrap 配置必须留空。
- 若只执行 `0001` 而未执行 `0002`，bootstrap 绑定 admin 角色时会因角色不存在而失败。
- 若未执行 `0004`，管理员无法使用域账号导入等 Admin 接口（403）。
- 各 `*.up.sql` 内部使用 `ON CONFLICT` 保证幂等，但**版本顺序不可打乱**（0002 依赖 0001 表结构，0003/0004 依赖 0001/0002）。

## 执行命令

本地联调（PostgreSQL 已启动）：

```bash
# 推荐：Makefile 封装
make migrate

# 等价：直接 go run
go run ./cmd/migrate -config configs/config.yaml
```

CI / 发布流水线示例：

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

默认管理员**用户**及 `iam_user_role` 绑定由 `cmd/api` 启动期 bootstrap 完成，见上文「执行顺序与 bootstrap 分工」。

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

## 运行期判读

迁移状态的结构化结果与 readiness 语义已统一到 `ops/health-contract.md`，用于联调与探针判读。

## 前端联调常见症状

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| 登录成功，Authed 接口 403（含域账号导入页） | 未执行 `0002` 和/或 `0004` | `make migrate` 追平至最新版本 |
| API 启动即退出（`ensure bootstrap admin role failed`） | 仅执行 `0001` 且 bootstrap 已配置，admin 角色不存在 | 同上，须先执行 `0002` |
| Dashboard `/readyz` 显示迁移未追平 | pending `0002` 或更高版本 | 同上 |
| 401 循环跳转登录 | JWT 无效或未注入 `JWT_SECRET` | 检查 `auth.jwt_secret` 与登录响应 |

前端在 403 时会提示检查迁移 `0002`；可通过 `GET /readyz` 的 `checks[migration]` 确认 pending 数量。
