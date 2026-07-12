# 数据库迁移契约

本文档面向平台团队、DBA 与联调方，描述平台的数据库迁移使用方式与责任边界。

## 迁移版本体系

平台只维护一套迁移版本体系：仓库 `migrations/` 目录下的版本化 SQL，以及 `schema_migrations` 表中的已应用版本记录。

| 允许 | 禁止 |
| --- | --- |
| 生产 DBA / 发布流水线在部署 API 之前显式执行自研 runner（`cmd/migrate` 二进制或 `go run ./cmd/migrate -config <path>`） | 手工 `psql -f migrations/*.up.sql`、手工执行 `migrations/manual_schema_migrations.sql`、手工写入或修改 `schema_migrations` |
| dev/test 使用 `make migrate` / `make migrate-up` | [golang-migrate](https://github.com/golang-migrate/migrate) CLI 或库 |
| dev/test 使用 `go run ./cmd/migrate -config <path>` | 任何第三方迁移工具或另一套迁移元数据表 |
| 受控 dev/test 下 `database.auto_migrate=true`（仍调用同一 `RunMigrations` 实现） | PostgreSQL `docker-entrypoint-initdb.d` 与应用启动自动迁移混用 |
| DBA 审批、备份、窗口控制和执行结果确认 | 同一环境对同一库交替使用多种迁移版本体系 |

**禁止混用**：同一数据库实例不得交替使用 golang-migrate（或其它外部工具）、手工 `psql` 与本仓库自研 runner。`schema_migrations` 表结构、版本号语义与幂等逻辑均以本仓库 runner 为准，迁移账本只能由 runner 写入。

## 责任边界

- **生产环境**：数据库初始化和迁移必须由 **DBA** 或发布流水线在部署 API **之前**显式执行，执行入口必须是本仓库自研 runner（`cmd/migrate` 二进制或 `go run ./cmd/migrate -config <path>`）。DBA 负责审批、备份、变更窗口、目标库权限和执行结果确认；runner 负责按版本顺序执行 `migrations/*.up.sql` 并写入 `schema_migrations`。
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
0018_init_integration.up.sql             → Integration 上下文：云账号/观测平台接入、凭据引用、能力声明、连通性检查
0019_init_observability.up.sql           → Observability 上下文：查询证据引用与 app:observability:read 权限
0020_init_inspection.up.sql              → Inspection 上下文：巡检策略、运行、发现、建议
0022_init_execution_agent.up.sql         → Execution Agent：执行介体、代理、Command Spec、租约、日志流
0023_asset_cloud_sync.up.sql             → Asset 上下文：云资源同步字段、同步批次、stale 标记
0024_integration_account_extra_config.up.sql → Integration：integration_account.extra_config（provider 扩展配置，如 huawei sync_mode）
0025_asset_resource_labels.up.sql          → Asset：asset_resource.labels（云同步 CES namespace/dim_name + 原生增强 label）
0026_asset_cloud_sync_region_key.up.sql    → Asset：云资源唯一键加 region，避免多区域同类型同 ID 互相覆盖
0027_asset_sync_batch_message_text.up.sql          → Asset：asset_sync_batch.message 改为 TEXT，修复应用层 2000 rune 截断与 VARCHAR(512) 不一致
0028_asset_sync_batch_running_mutex.up.sql         → Asset：asset_sync_batch.lease_expires_at + running 部分唯一索引（账号级并发互斥，修复并发批次互相标记 stale）
0029_huawei_legacy_accounts_native_sync_mode.up.sql → Integration：历史空配置华为账号回填 sync_mode=native，修复 0024 空配置被解析为 ces 的灰度策略失效
0030_asset_sync_batch_fencing_token.up.sql         → Asset：asset_sync_batch.fencing_token + running 所有权校验索引（防止租约丢失旧任务继续写入）
0031_asset_sync_batch_summary.up.sql               → Asset：asset_sync_batch.summary JSONB 结构化摘要；批次详情页不再把 message 当作半结构化协议解析
0032_cleanup_legacy_cloud_application_ids.up.sql   -> Asset：破坏性 DELETE 脚本，按 application_id = 'cloud-' || trim(account_id) 精确关联 integration_account 删除旧格式 cloud-<account_id> 应用及其关联的 asset_resource/asset_match_rule（不处理 alert_alert/inspection_policy）；覆盖 account_id 含/不含连字符的所有情况；从未在共享环境执行，当前为开发期最终版，所有数据库须从零重建
0033_asset_sync_batch_triggered_by.up.sql           → Asset：asset_sync_batch.triggered_by（触发用户 user_id），reap 崩溃批次时 sync_reaped 审计 actor 取该字段还原原操作者
0034_huawei_ces_vpc_subtype_split.up.sql           → Asset：按 labels->>'dim_name' 把存量 cloud_sync 'vpc' 资源回填为 eip/bandwidth/subnet/peering（CES SYS.VPC 聚合 namespace 拆子类型），未知 dim 保留 vpc；仅 source='cloud_sync' 且 namespace='SYS.VPC'
0035_cloud_application_id_rune_truncation.up.sql    → Asset：按 rune 截取与 sha1 后缀修复 cloud application_id，并同步改写相关引用
0036_cloud_application_name_include_account.up.sql  → Asset：云同步应用名称包含账号信息，避免多账号同名混淆
0037_fix_huawei_ces_application_ids.up.sql         → Asset：修复 Huawei CES legacy/new application_id 并存时的安全合并；先迁移并去重子表引用，再删除旧应用；仅 legacy 存在时安全重命名，only new 时幂等
0038_cloud_application_name_normalize.up.sql       -> Asset：把反向格式云同步应用名 <provider>-<account_id>-cloud 归一化为契约格式 <provider>-cloud-<account_id>，收敛 0036 之后由反向格式代码新建的应用；仅改 name 不改 application_id
0039_cleanup_orphaned_application_refs.up.sql      -> Asset：清理 0032 DELETE 遗留的 alert_alert/inspection_policy 孤儿引用，按 integration_account 计算 old->new 映射改写为新格式；不依赖 has_old（旧应用可能已被 0032 删除）；幂等
0040_application_ref_integrity_view.up.sql          -> Asset：创建持久视图 v_asset_app_ref_integrity，暴露 asset_resource/asset_match_rule/alert_alert/inspection_policy 中指向不存在 asset_application 的孤儿引用；不修改数据，不阻断迁移；幂等（CREATE OR REPLACE VIEW）
0041_legacy_app_id_convergence_guard.up.sql         -> Asset：legacy 应用收敛硬阻断守卫，若 asset_application 中仍存在 cloud-<account_id> 格式 legacy 应用则迁移失败终止；使用 CHECK(n=0) 约束实现，兼容自研 SQL splitter；不修改业务数据
0042_backfill_orphaned_app_refs_and_guard.up.sql     -> Asset：补建 0039 改写后仍被引用但不存在的新格式 cloud application ID 对应的 asset_application 记录（字段与 ensureCloudApplication 一致），并将 v_asset_app_ref_integrity 作为硬验收（CHECK(n=0)），补建后仍有孤儿则迁移失败；幂等（ON CONFLICT DO NOTHING）；依赖 pgcrypto digest()
0043_fix_orphaned_alert_app_refs.up.sql              -> Asset：修复 DeleteApplication 缺失跨上下文引用检查导致的孤儿告警引用（alert_alert.application_id 指向不存在的应用）；将孤儿引用置空并清理 inspection_policy.scope.application_ids 中的孤儿元素；CHECK(n=0) 硬验收确保修复后视图返回 0 行；幂等
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
- 云同步应用 ID/名称收敛链 `0032 -> 0035 -> 0036 -> 0037 -> 0038 -> 0039 -> 0040 -> 0041 -> 0042 -> 0043` 必须按版本号顺序执行，DBA / 发布脚本不得只按前半段截断执行；`0032` 是破坏性 DELETE（删除旧格式应用及关联资源/规则，不处理 alert_alert/inspection_policy），`0039` 补全清理 0032 遗留的孤儿引用；`0038` 只改 `asset_application.name` 不改 `application_id`，处理的是 `0036` 之后由 `ensureCloudApplication` 代码新建的反向格式应用，必须排在 `0036`/`0037` 之后，建议放最后统一收敛名称；与 ID 类迁移 `0032`/`0035`/`0037` 互不冲突，详见 `docs/huawei-ces-sync-runbook.md` §6。`0040` 创建持久视图 `v_asset_app_ref_integrity` 供引用完整性验收（`SELECT * FROM v_asset_app_ref_integrity` 期望 0 行），不修改数据不阻断迁移；`0041` 是 legacy 应用收敛硬阻断守卫（`CHECK(n=0)` 约束），若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用则迁移终止，不修改业务数据；若 0041 阻断需排查 0032/0037 收敛失败或代码路径仍在创建旧格式应用。`0042` 补建 `0039` 改写后仍被引用但不存在的新格式 cloud application ID 对应的 `asset_application` 记录（字段与 `ensureCloudApplication` 一致），并将 `v_asset_app_ref_integrity` 作为发布硬验收（`CHECK(n=0)` 约束）：补建后若视图仍有孤儿行（如非 cloud 格式引用、或 `integration_account` 已删除的 cloud ID），迁移终止，runner 不记录版本号；若 0042 阻断需排查是否存在非 cloud 格式孤儿引用或账号已删除但仍被引用的情况，修复后重试。`0043` 修复 `DeleteApplication` 历史上仅检查同上下文引用（asset_resource/asset_match_rule）而未检查跨上下文引用（alert_alert/inspection_policy）导致的孤儿告警引用：将 `alert_alert` 中指向不存在应用的 `application_id`/`application_name` 置空，并移除 `inspection_policy.scope.application_ids` 中的孤儿元素，修复后 `CHECK(n=0)` 确保视图返回 0 行；修复后 `DeleteApplication` 已增加 `ApplicationReferenceChecker` 跨上下文引用检查，防止新增孤儿。

## 执行命令

生产 / 预发显式迁移：

```bash
# 推荐：使用已构建的自研 runner 二进制
./migrate -config /path/to/config.yaml

# 等价：源码方式执行自研 runner
go run ./cmd/migrate -config /path/to/config.yaml
```

> 禁止在生产或预发环境手工 `psql -f migrations/*.up.sql`、手工执行 `migrations/manual_schema_migrations.sql`，或手工写入 / 修改 `schema_migrations`。迁移账本必须由自研 runner 维护。

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
| `checksum` | 迁移文件内容的 SHA-256，runner 在应用迁移时写入；用于检测已应用迁移是否被改写（checksum 漂移） |
| `applied_at` | 应用时间 |

**建表规范例外**：平台要求业务表 `created_at` / `updated_at` 由 Go 程序维护、禁止 DB DEFAULT；`schema_migrations.applied_at` 为 runner 内部元数据，表定义保留 `DEFAULT NOW()` 作为兜底，但 runner 在 `INSERT` 时**显式传入** `time.Now()`，与业务表约定区分。

**checksum 漂移检测**：runner 在执行迁移前会比较已应用版本记录的 `checksum` 与当前迁移文件内容的 SHA-256。若不一致，runner 直接报错终止，**任何版本均无白名单绕过**。`checksum` 为空（历史库升级后首次运行）时，runner 自动回填当前文件 hash，不视为漂移。禁止改写已发布迁移文件；确需修复应新增后续迁移版本。

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

### `0018_init_integration`

Integration 上下文第一阶段迁移，建表：

- `integration_account`：云账号/观测平台账号（业务 ID `account_id`）
- `integration_credential_ref`：凭据引用（AES 密文或外部 Secret 引用，不存明文）
- `integration_capability`：Provider 能力声明（`metrics` / `logs` / `traces` / `topology` / `alerts` / `assets`）
- `integration_check_result`：连通性检查历史

权限种子：

- `app:integrations:read`
- `app:integrations:create`
- `app:integrations:update`
- `app:integrations:delete`
- `app:integrations:check`

并绑定到 `admin` 角色。API 契约见 `ops/cloud-observability-contract.md` §4。

## P1+ 接入、观测、巡检与执行介体迁移规划

云厂商只读接管、智能体巡检和执行介体属于后续演进能力，落地时仍沿用本仓库自研 runner，不引入第二套迁移工具。建议从 `0018` 开始连续递增，不插队、不改写已发布版本：

| 规划版本 | 建议文件 | 上下文 | 主要对象 | 状态 |
| --- | --- | --- | --- | --- |
| `0018` | `0018_init_integration.up.sql` | Integration | `integration_account`、`integration_credential_ref`、`integration_capability`、`integration_check_result` | 已落地 |
| `0019` | `0019_init_observability.up.sql` | Observability | `obs_evidence_ref`（证据引用）；权限 `app:observability:read` | 已落地（Port + fake provider；拓扑/查询历史表后续递增） |
| `0020` | `0020_init_inspection.up.sql` | Inspection | `inspection_policy`、`inspection_run`、`inspection_finding`、`inspection_recommendation` | 已落地 |
| `0021` | `0021_init_notification.up.sql` | Notification | `notification_channel`、`notification_template`、`notification_delivery` |
| `0022` | `0022_init_execution_agent.up.sql` | Execution | `exec_medium`、`exec_agent`、`exec_command_spec`、`exec_lease`、`exec_log_stream` | 已落地（当前仓库未包含 `0021` 文件，手工执行按实际文件顺序） |
| `0023` | `0023_asset_cloud_sync.up.sql` | Asset | `asset_resource` 云同步字段、`asset_sync_batch` | 已落地（资源同步批次与 stale 标记） |
| `0024` | `0024_integration_account_extra_config.up.sql` | Integration | `integration_account.extra_config`（provider 扩展配置，如 huawei sync_mode） | 已落地 |
| `0025` | `0025_asset_resource_labels.up.sql` | Asset | `asset_resource.labels`（CES namespace/dim_name + 原生增强 label） | 已落地 |
| `0026` | `0026_asset_cloud_sync_region_key.up.sql` | Asset | 云资源唯一键加 region（多区域去重） | 已落地 |
| `0027` | `0027_asset_sync_batch_message_text.up.sql` | Asset | `asset_sync_batch.message` 改为 TEXT（修复应用层 2000 rune 与 VARCHAR(512) 不一致） | 已落地 |
| `0028` | `0028_asset_sync_batch_running_mutex.up.sql` | Asset | `asset_sync_batch.lease_expires_at` + 部分唯一索引 `(integration_account_id) WHERE status='running'`（账号级并发互斥，修复并发批次互相标记 stale）；其中清理历史 running 批次用的 `LEFT(...,512)` 为历史兼容截断（沿用 0023 的 `VARCHAR(512)`），当前 message 长度约束以 0027 的 TEXT + 应用层 `sync_service.go` 2000 rune 截断为准 | 已落地 |
| `0029` | `0029_huawei_legacy_accounts_native_sync_mode.up.sql` | Integration | 把仍为空配置 `{}` 的历史华为账号回填 `sync_mode=native`，修复 0024 空配置被解析为 ces 导致升级后立即切 CES 的灰度策略失效；新账号由 `encodeExtraConfigInput` 显式写 ces | 已落地 |
| `0030` | `0030_asset_sync_batch_fencing_token.up.sql` | Asset | `asset_sync_batch.fencing_token` + running 所有权校验索引；续租按 `batch_id + fencing_token` 匹配，写前校验 running 且租约未过期，防止旧任务租约丢失后继续 upsert/stale | 已落地 |
| `0031` | `0031_asset_sync_batch_summary.up.sql` | Asset | `asset_sync_batch.summary` JSONB 结构化摘要；批次详情页不再把 `message` 当作半结构化协议解析 | 已落地 |
| `0032` | `0032_cleanup_legacy_cloud_application_ids.up.sql` | Asset | 破坏性 DELETE 脚本：按 `application_id = 'cloud-' \|\| trim(account_id)` 精确关联 `integration_account` 删除旧格式 `cloud-<account_id>` 应用及其关联的 `asset_resource`/`asset_match_rule`；覆盖 `account_id` 含/不含连字符的所有情况（初版启发式 `NOT LIKE 'cloud-%-%'` 漏判含连字符账号，已修订为精确匹配）；**不处理** `alert_alert`/`inspection_policy` 中的旧格式引用（由 `0039` 补全清理）；`down.sql` 为人工回滚参考，被删除数据无法自动恢复 | 已落地 |
| `0033` | `0033_asset_sync_batch_triggered_by.up.sql` | Asset | `asset_sync_batch.triggered_by`（触发用户 user_id）；`TriggerSync` 创建 running 批次时写入，reap 崩溃批次时 `sync_reaped` 审计 actor 取该字段，避免归因到当次请求用户 | 已落地 |
| `0034` | `0034_huawei_ces_vpc_subtype_split.up.sql` | Asset | 按 `labels->>'dim_name'` 把存量 `cloud_sync` 的 `'vpc'` 资源回填为 `eip`/`bandwidth`/`subnet`/`peering`（CES `SYS.VPC` 聚合 namespace 拆子类型，避免语义混合与 ID 碰撞），未知 dim 保留 `vpc`；仅处理 `source='cloud_sync'` 且 `labels->>'namespace'='SYS.VPC'`，native VPC 实体不受影响 | 已落地 |
| `0035` | `0035_cloud_application_id_rune_truncation.up.sql` | Asset | 按 `application_id` 的 sha1 后缀关联 `integration_account`，把旧实现按字节截取（`id[:17]`）的多字节账号 `cloud-` application_id 无损改写为按字符截取的 rune 版（`left(trim(account_id),17)`，与修复后的 `cloudApplicationID` 及 `0032` 一致）；同步改写 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy.scope.application_ids`（`inspection_policy` 改写时 `DISTINCT` 去重，避免旧字节版与新 rune 版替换后产生重复 ID，与 `0032`/`0037` 一致）；新旧应用并存时按云资源唯一键合并到新应用并删除旧应用；纯 ASCII 账号字节版=rune 版无改写；依赖 pgcrypto `digest()` | 已落地 |
| `0036` | `0036_cloud_application_name_include_account.up.sql` | Asset | 调整云同步应用名称包含账号信息，避免多账号同名混淆；同步更新历史云同步应用展示与运维排查路径 | 已落地 |
| `0037` | `0037_fix_huawei_ces_application_ids.up.sql` | Asset | 修复 Huawei CES legacy/new application_id 并存时的安全合并：先迁移并去重子表引用（`asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy.scope.application_ids`），再删除旧应用；仅 legacy 存在时安全重命名为新格式，only new 时幂等；依赖 pgcrypto `digest()` | 已落地 |
| `0038` | `0038_cloud_application_name_normalize.up.sql` | Asset | 把反向格式云同步应用名 `<provider>-<account_id>-cloud`（`ensureCloudApplication` 代码曾误用）归一化为契约格式 `<provider>-cloud-<account_id>`，收敛 0036 之后由反向格式代码新建的应用；`account_id` 从 `description` 提取（沿用 0036 思路），保证 account_id 含连字符也正确；仅改 `asset_application.name`，不改 `application_id`，不影响匹配键与引用关系；匹配策略为精确比对 `name = provider\|\|'-'||account_id||'-cloud'`（`provider` 取 `split_part(name,'-',1)`，`account_id` 取 `description` 去前缀），不使用 LIKE 模糊排除，避免 account_id 含 `-cloud-` 或以 `-cloud` 结尾时被 `NOT LIKE '%-cloud-%'` 错误漏判；幂等：契约格式不等于反向格式（仅 `account_id='cloud'` 时两者相同，重写为同值无副作用），旧格式 `<provider>-cloud` 缺 account_id 段不命中，仅反向格式精确命中 | 已落地 |
| `0039` | `0039_cleanup_orphaned_application_refs.up.sql` | Asset | 清理 `0032` DELETE 遗留的孤儿引用：按 `integration_account` 计算 `old_app_id -> new_app_id` 映射，把 `alert_alert.application_id` 和 `inspection_policy.scope.application_ids` 中仍残留的旧格式 `cloud-<account_id>` 改写为新格式 `cloud-<prefix>-<hash>`；**不依赖 `has_old`**（旧应用可能已被 `0032` 删除，`0037` 因此跳过这两个表）；幂等：已是新格式或已由 `0037` 处理过的行 WHERE 不匹配；依赖 pgcrypto `digest()`；`down.sql` 为人工回滚参考 | 已落地 |
| `0040` | `0040_application_ref_integrity_view.up.sql` | Asset | 创建持久视图 `v_asset_app_ref_integrity`，暴露 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy` 中指向不存在 `asset_application` 的孤儿引用；不修改数据，不阻断迁移；幂等（`CREATE OR REPLACE VIEW`）；`down.sql` 为 `DROP VIEW IF EXISTS` | 已落地 |
| `0041` | `0041_legacy_app_id_convergence_guard.up.sql` | Asset | legacy 应用收敛硬阻断守卫：若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用，`CHECK(n=0)` 约束失败导致迁移终止；兼容自研 SQL splitter（纯 DML，无 `$$`）；不修改业务数据；`down.sql` 无操作 | 已落地 |
| `0042` | `0042_backfill_orphaned_app_refs_and_guard.up.sql` | Asset | 补建 `0039` 改写后仍被引用但不存在的新格式 cloud application ID 对应的 `asset_application` 记录（字段与 `ensureCloudApplication` 一致：`name`/`environment`/`description`）；将 `v_asset_app_ref_integrity` 作为发布硬验收（`CHECK(n=0)` 约束），补建后若视图仍有孤儿行则迁移终止；幂等（`ON CONFLICT DO NOTHING`）；依赖 pgcrypto `digest()`；`down.sql` 为人工回滚参考 | 已落地 |
| `0043` | `0043_fix_orphaned_alert_app_refs.up.sql` | Asset | 修复 `DeleteApplication` 缺失跨上下文引用检查导致的孤儿告警引用：将 `alert_alert` 中指向不存在应用的 `application_id`/`application_name` 置空，移除 `inspection_policy.scope.application_ids` 中的孤儿元素；`CHECK(n=0)` 硬验收确保修复后 `v_asset_app_ref_integrity` 返回 0 行；幂等（仅处理孤儿行）；`down.sql` 为人工回滚参考（需从备份恢复原始值） | 已落地 |

这些迁移必须同步种子化权限和 AI 工具权限，并把 admin 角色绑定到新增权限：

- Integration（`0018` 已落地）：`app:integrations:read/create/update/delete/check`（HTTP 层 action 与权限码一一对应）。
- Observability（`0019` 已落地）：`app:observability:read`（HTTP 层 `observability` + `read`）；AI 工具 `cloud.metrics.query`、`cloud.logs.search`、`cloud.traces.query`、`cloud.topology.get` 等 readonly 模式待 `0019+` 种子。
- Inspection：`app:inspections:read/create/update/run`，AI 工具 `inspection.runs.create`、`inspection.findings.analyze`。
- Notification：`app:notifications:read/create/update/test`。
- Execution Agent：`app:executions:media:read/create/update/delete`、`app:executions:agents:manage`、`app:executions:command_specs:read/create/update/delete`，AI 工具 `execution.media.list`、`execution.tasks.propose`；`execution.tasks.dispatch` 不授予 AI。

建表仍遵守全局规范：`id BIGSERIAL` 仅作内部主键；跨上下文和对外 API 使用 `provider_account_id`、`inspection_run_id`、`medium_id`、`agent_id`、`command_spec_id`、`lease_id` 等业务 ID；业务表必须包含由 Go 维护的 `created_at` / `updated_at`；默认不加数据库外键，通过 repository 事务、唯一索引和存在性校验保证一致性。

执行介体相关表还必须预留审计与安全字段：介体类型、网络区域、环境、风险等级上限、允许的 Command Spec、Agent 最近心跳、租约过期时间、输出脱敏策略、结果摘要。凭据只保存引用，不保存明文密钥、SSH 私钥、云 AK/SK 或临时 Token。

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
