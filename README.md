# AI 运维平台

> 面向智能化、自动化、闭环化的企业级 AI 运维平台。
> 当前阶段：**P0 业务闭环已打通**，支持告警接入 → 资产匹配 → Runbook 推荐 → 执行确认 → Dashboard 汇总 → 审计追溯的完整演示链路；云接入、统一观测、手动巡检和 AI 助手已在不同范围内落地。

## 模块状态

状态按代码路径和仓库内验证证据划分，不把规划项计入已落地能力。“有 E2E 覆盖”仅表示仓库存在对应脚本，不代表当前环境已经执行通过；发布结果以当次测试和验收记录为准。

| 模块 | 当前实现 | 前端入口 | 验证覆盖 | 完成边界 |
|------|----------|----------|----------|----------|
| Identity 认证与权限 | 已落地 | 登录、域账号导入、权限管理 | `e2e-identity-access.ps1` 覆盖用户角色主链路；另有单元测试 | 本地/LDAP/OAuth、JWT、RBAC、数据范围与 AI 工具权限已具备；动态菜单、MFA 未落地 |
| Alert 告警中心 | 已落地 | `/alerts` | `e2e-alert.ps1`、`e2e-alert-sync.ps1` | Webhook、状态流转、CES 手动拉取已落地；后台拉取 worker 可选且默认关闭 |
| Asset 资源与应用 | 已落地，云同步分模式支持 | `/assets`、`/assets/topology` | `e2e-asset.ps1`、`e2e-asset-sync.ps1`；真实云对账另有可选脚本 | 应用/资源、匹配规则、异步同步、CMDB 拓扑已落地；原生增强、同步数据范围、审计生命周期和云应用命名仍有专项差距 |
| Runbook 处置预案 | 已落地 | `/runbooks`、告警详情 | `e2e-runbook.ps1` | 模板管理、告警推荐、结构化多步任务生成；真实动作仍交给 Execution |
| Execution 自动化执行 | 核心闭环与 Agent 控制面已落地 | `/executions` | `e2e-execution.ps1`、`e2e-execution-agent*.ps1` | 默认模拟执行；独立 Agent、专门 UI、自动选择、mTLS、Runbook→Agent 和 Agent Alert 时间线未完整交付 |
| Dashboard 首页驾驶舱 | 已落地 | `/dashboard` | 单元/HTTP 测试；Identity E2E 单次摘要调用，无专用 E2E | 告警、执行、资产、Runbook 聚合摘要；降级 0 与真实 0 不可区分，无数据范围过滤 |
| Audit 审计中心 | 已落地，前端聚合与导出有范围限制 | `/audits` | 服务单元测试与跨模块 E2E 审计断言，无独立 E2E/前端 Vitest | 精确分页检索；聚合仅当前页，CSV 实际最多 100 条；资源类型快捷选项未与真实目录对齐，认证审计独立查询 |
| AI Provider 与工具网关 | 持久化配置、三类 Executor、直接转发与 8 个对话只读工具已落地 | `/system/ai-config`、Identity 权限管理 | 后端单元/HTTP/出站测试；AI E2E 使用 mock/fake，无直接 invoke 专项 | 无健康检查/测试连接；Provider CRUD 无操作审计，页面编辑会清 headers；执行类内置工具未落地 |
| AI 运维助手 | 会话、幂等、SSE、受限循环、记忆与知识已落地，前端部分交互有差距 | `/ai-assistant`、全局浮层、`/system/ai-config` | 后端/前端部分测试与 `e2e-ai-*.ps1`；均为 mock/fake，非浏览器或真实模型全量验收 | 详情最近 200 条；重试会重放旧回答，归档输入/模型禁用/详情刷新有缺口；记忆/知识管理筛选与展示有限，真实/负向场景需专项；只生成待确认执行计划 |
| Integration 接入账号 | 账号与字段级检查已落地 | `/integrations` | 单元/HTTP/配置纯函数测试；`e2e-integration.ps1` 覆盖 none 主路径 | AES-GCM 凭据、能力声明、软删除清密文已落地；检查不访问外部服务，capabilities 非实测；agency 前端和完整密钥轮换未落地 |
| Observability 统一观测与证据 | 四类查询与 AI 定稿快照已落地，真实 Provider 部分落地 | `/observability` | 单元/mock/PostgreSQL 测试与前端 Vitest；查询和快照 E2E 均使用 fake/mock | 华为 AK/SK 指标真实，真实日志/链路/拓扑不支持；SigNoz/Prometheus 为 fake；快照无公共详情 API/UI，真实指标 E2E 未提供 |
| Inspection 巡检中心 | 手动同步规则巡检与后端转执行已落地 | `/inspections` | 单元/HTTP 测试；`e2e-inspection.ps1` 使用 fake；无转执行专项测试 | 非 AI Agent；定时 worker/取消/通知未落地，真实 scope 采集受限；转执行无前端、自动介体、跨模块事务或完成回写 |

**文档**：
- [演示流程](docs/demo-flow.md) — 10 步完整闭环演示
- [整体流程与调用关系](docs/AI运维平台整体流程与调用关系.md) — 将 P0 闭环、只读观测、巡检、执行介体串联到一张图
- [调用关系图](docs/AI运维平台调用关系图.md) — 后端总图、P0 时序、AI 工具、观测巡检与 Agent 派发图
- [上线检查清单](docs/release-checklist.md) — 发布前必查项
- [验收清单](docs/acceptance-checklist.md) — 模块级验收明细
- [文档与实现复核记录](docs/documentation-review.md) — 当前实现边界、文档差距和逐模块更新队列
- [Kubernetes 部署说明](deployments/kubernetes.md) — 外挂 PostgreSQL/Redis 的 K8s 参考；迁移章节仍待修订，生产操作先以迁移契约和发布清单为准

## 快速验收

```powershell
# 环境：中间件 + 显式迁移；不要与 dev 自动迁移叠加执行
docker compose -f deployments/docker-compose.yml up -d postgres redis
go run ./cmd/migrate

# 测试与构建
go test ./cmd/... ./internal/... ./pkg/...
cd web
npm run build
npm test
cd ..

# P0 与云接入链路
.\scripts\e2e-alert.ps1
.\scripts\e2e-alert-sync.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-asset-sync.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1
.\scripts\e2e-identity-access.ps1
.\scripts\e2e-integration.ps1
.\scripts\e2e-observability.ps1
.\scripts\e2e-inspection.ps1
.\scripts\e2e-execution-agent.ps1
.\scripts\e2e-execution-agent-permission.ps1

# AI 专项链路
.\scripts\e2e-ai-assistant.ps1
.\scripts\e2e-ai-memory.ps1
.\scripts\e2e-ai-knowledge.ps1
.\scripts\e2e-ai-stream.ps1
.\scripts\e2e-ai-agent-loop.ps1
.\scripts\e2e-ai-rules.ps1
.\scripts\e2e-ai-evidence.ps1
```

以下是带外部资源前提的可选验收，不属于默认可直接运行的全量脚本：

```powershell
# 真实华为云 CES 资源同步与对账：需只读子账号、资源组和相应权限
.\scripts\e2e-asset-sync-real.ps1

# RDS for PostgreSQL 历史会话：需迁移 0061、长事务告警及其关联云账号
.\scripts\e2e-postgres-history.ps1 -AlertId <alert_id>
```

按改动范围选择最小必要验证；触及共享模块、权限、迁移或执行闭环时，再按 [验收清单](docs/acceptance-checklist.md) 扩大范围。

## 目录结构

```
aiops/
├── cmd/                      # 程序入口（按可执行文件分子目录）
│   ├── api/                  # HTTP API 主入口，也支持 -migrate 迁移后退出
│   ├── migrate/              # 数据库迁移独立入口（make migrate）
│   └── ai-orchestrator/      # 可选 type=c 内部 AI 编排参考服务，非主 API 必需组件
├── internal/                 # 平台内部实现，禁止外部直接 import
│   ├── bootstrap/            # 配置/日志/数据库装配
│   ├── server/               # Gin 引擎、HTTP server、RouteRegistrar 接口
│   ├── version/              # 版本号信息（编译期注入）
│   ├── identity/             # 认证、RBAC、LDAP/OAuth
│   ├── alert/                # Webhook/CES 拉取、状态流转、时间线、资产绑定
│   ├── asset/                # 应用/资源、匹配规则、云同步、CMDB 拓扑与关系
│   ├── runbook/              # 处置预案模板与推荐
│   ├── execution/            # 执行任务状态机、确认、Agent 控制面与时间线
│   ├── dashboard/            # 首页聚合摘要
│   ├── audit/                # 操作审计查询
│   ├── ai/                   # Provider、助手会话、工具循环、记忆/知识、诊断与执行提议
│   ├── integration/          # 云账号/观测平台账号接入、凭据引用、能力探测
│   ├── observability/        # 统一只读查询、资源/告警采集端口与证据快照
│   └── inspection/           # 手动巡检策略、运行、Finding 与 Recommendation
├── pkg/                      # 跨模块共享库（可被 internal 与未来 cmd 复用）
│   ├── auth/                 # JWT、密码、refresh 轮换、登录限流与 IP 白名单
│   ├── config/               # YAML + 环境变量配置加载（viper）
│   ├── logger/               # zap 日志，trace 注入
│   ├── errors/               # 平台统一错误类型与错误码
│   ├── database/             # PostgreSQL（GORM）+ Redis 初始化
│   ├── pagination/           # 通用分页参数
│   └── transport/http/       # HTTP 响应封装与通用中间件
├── configs/                  # 配置样例
├── deployments/              # docker-compose、Dockerfile
├── migrations/               # SQL 迁移脚本
├── docs/                     # 设计文档（业务流程、信息架构、技术架构、页面原型）
└── web/                      # 前端 Vue 3 工程
    ├── src/
    │   ├── api/              # axios 封装与各模块 API
    │   ├── layouts/          # 布局组件
    │   ├── router/           # vue-router 路由
    │   ├── stores/           # pinia 状态
    │   ├── views/            # 页面（dashboard / alerts / assets / runbooks / executions 等）
    │   └── styles/
    ├── vite.config.ts
    └── package.json
```

## 快速开始

### 0. 前置依赖

| 工具 | 版本要求 | 说明 |
| --- | --- | --- |
| Go | 1.26+ | 后端运行时 |
| Node.js | 18+ | 前端运行时 |
| Docker / Docker Desktop | 任意近期版本 | 启动 PostgreSQL + Redis |
| Make | 可选 | 使用根目录 `Makefile` 简化命令 |

### 1. 启动中间件

| 模式 | 命令 | 适用场景 |
| --- | --- | --- |
| **仅中间件** | `docker compose -f deployments/docker-compose.yml up -d postgres redis` | 本地 `go run ./cmd/api` 联调（推荐） |
| **全栈（默认）** | `docker compose -f deployments/docker-compose.yml up -d` | 容器化 API + PG + Redis |
| **全栈 dev 就绪** | `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d` | 自动迁移 + bootstrap 管理员 + 挂载 `config.yaml`（仅后端） |
| **全栈 + 前端容器** | `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml -f deployments/docker-compose.host.yml -f deployments/docker-compose.web.yml up -d` | 前后端分离容器：API + Web + PG + Redis；访问 `http://127.0.0.1/` |

> 后端（`aiops-api`）与前端（`aiops-web`）是**两个独立镜像/容器**。前端服务由 `deployments/docker-compose.web.yml` 定义，使用 `deployments/nginx/web.conf` 反代 `api:8080`；`docker-compose.host.yml` 仅在 PostgreSQL 因 seccomp 报错的宿主机叠加。生产走 `docker-compose.prod.yml`。详见 `deployments/README.md`。

```bash
# 仅 PostgreSQL + Redis（本地 go run 联调常用）
docker compose -f deployments/docker-compose.yml up -d postgres redis

# 全栈：API 会启动，但默认不建表 → /readyz 可能 not_ready
docker compose -f deployments/docker-compose.yml up -d

# 全栈 dev 就绪：推荐首次容器联调使用（AUTO_MIGRATE + admin 账号）
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
```

> **注意**：默认 Compose **不会**自动建表。PostgreSQL 未挂载 `docker-entrypoint-initdb.d`，且 `database.auto_migrate` 在代码默认值、`config.example.yaml`、`.env.example` 与主 `docker-compose.yml` 中均为 `false`；仅当显式设为 `true` 时 `bootstrap.Init` 才会执行迁移。全栈默认模式需先 `make migrate` 或叠加 `docker-compose.dev.yml`。

> **安全**：Compose 主文件将 PostgreSQL（5432）、Redis（6379）映射到宿主机；PG 默认 `aiops/aiops`，**仅限本机开发或受控初始化**。迁移历史中的 `0016` 会种子默认管理员，但 `0044` 会锁定仍使用已知默认密码的账号；不要假设完整迁移后 `admin/admin123` 必然可登录。受控 dev/test 可由 bootstrap 重新激活，生产应关闭 bootstrap，使用 `scripts/provision-prod-admin.ps1` 初始化安全管理员，并通过 secrets 注入 JWT、数据库密码和 Execution Agent 注册令牌。生产环境还须 `redis.required=true`，且不得发布 DB/Redis 端口。

### 2. 数据库迁移（必做）

平台**仅**使用自研迁移 runner（`pkg/database/migrate.go`），**禁止**与 golang-migrate 等外部工具混用。契约见 `ops/migration-contract.md`。

PostgreSQL 就绪后，在启动 API **之前**执行：

```bash
make migrate
# 等价别名：
make migrate-up
# 或直接 go run：
go run ./cmd/migrate -config configs/config.yaml
```

当前迁移文件（`0001` -> `0062`，按实际文件名顺序执行；当前仓库未包含 `0021` 文件，不要手工补空版本）：

| 版本 | 文件 | 说明 |
| --- | --- | --- |
| `0001` | `0001_init_identity.up.sql` | Identity 核心表 |
| `0002` | `0002_seed_admin_permissions.up.sql` | admin 角色与基础权限种子 |
| `0003`–`0006` | 外部身份、用户预置、认证审计 | LDAP/OAuth 与登录审计 |
| `0007` | `0007_init_alert.up.sql` | 告警模块 |
| `0008` | `0008_init_asset.up.sql` | 资产注册表 |
| `0009` | `0009_init_audit.up.sql` | 审计模块 |
| `0010`–`0011` | AI 分析权限、执行模块 | 工具权限与执行任务 |
| `0012` | `0012_init_runbook.up.sql` | Runbook 模板与种子数据 |
| `0013` | `0013_dashboard_permission.up.sql` | Dashboard 读权限 |
| `0014` | `0014_init_asset_match_rule.up.sql` | 可配置告警匹配规则 |
| `0015` | `0015_identity_access_control_management.up.sql` | 权限管理 P1：viewer 角色、用户角色绑定、角色权限、数据范围、AI 工具权限 |
| `0016` | `0016_seed_default_admin_user.up.sql` | 受控初始化默认本地管理员 `admin/admin123`，并将 admin 角色绑定为当前权限全集 |
| `0017` | `0017_repair_default_admin_superset.up.sql` | 修复已应用旧 `0016` 的环境，重新确保默认 admin 入口和权限全集 |
| `0018` | `0018_init_integration.up.sql` | Integration：云账号/观测平台账号、凭据引用、能力声明、连通性结果 |
| `0019` | `0019_init_observability.up.sql` | Observability：证据引用与 `app:observability:read` |
| `0020` | `0020_init_inspection.up.sql` | Inspection：巡检策略、运行、Finding、Recommendation |
| `0022` | `0022_init_execution_agent.up.sql` | Execution Agent：执行介体、代理、Command Spec、租约、日志流 |
| `0023` | `0023_asset_cloud_sync.up.sql` | Asset：云资源同步字段、同步批次、stale 标记 |
| `0024` | `0024_integration_account_extra_config.up.sql` | Integration：integration_account.extra_config（provider 扩展配置，如 huawei sync_mode） |
| `0025` | `0025_asset_resource_labels.up.sql` | Asset：asset_resource.labels（CES namespace/dim_name + 原生增强 label） |
| `0026` | `0026_asset_cloud_sync_region_key.up.sql` | Asset：云资源唯一键加 region，避免多区域同类型同 ID 互相覆盖 |
| `0027` | `0027_asset_sync_batch_message_text.up.sql` | Asset：`asset_sync_batch.message` 改为 TEXT，修复应用层 2000 rune 截断与 VARCHAR(512) 不一致 |
| `0028` | `0028_asset_sync_batch_running_mutex.up.sql` | Asset：`asset_sync_batch.lease_expires_at` + running 部分唯一索引（账号级并发互斥） |
| `0029` | `0029_huawei_legacy_accounts_native_sync_mode.up.sql` | Integration：历史空配置华为账号回填 `sync_mode=native`，修复 0024 空配置被解析为 ces 的灰度策略失效 |
| `0030` | `0030_asset_sync_batch_fencing_token.up.sql` | Asset：`asset_sync_batch.fencing_token` 与 running 所有权校验索引，防止旧任务租约丢失后继续写入 |
| `0031` | `0031_asset_sync_batch_summary.up.sql` | Asset：`asset_sync_batch.summary` JSONB 结构化摘要；批次详情页不再把 `message` 当作半结构化协议解析 |
| `0032` | `0032_cleanup_legacy_cloud_application_ids.up.sql` | Asset：破坏性 DELETE 脚本，按 `application_id = 'cloud-' \|\| trim(account_id)` 精确关联 `integration_account` 删除旧格式 `cloud-<account_id>` 应用及其关联的 `asset_resource`/`asset_match_rule`（不处理 `alert_alert`/`inspection_policy`，由 `0039` 补全清理）；覆盖 `account_id` 不含 `-` 与含 `-` 两类账号；保留 `integration_account`，升级后需重新触发云同步（无需重新录入账号）；从未在共享环境执行，所有数据库须从零重建 |
| `0033` | `0033_asset_sync_batch_triggered_by.up.sql` | Asset：增加 `asset_sync_batch.triggered_by` 列；当前 TriggerSync 尚未写入该字段，reap 原操作者归因未完整落地 |
| `0034` | `0034_huawei_ces_vpc_subtype_split.up.sql` | Asset：按 `labels->>'dim_name'` 把存量 `SYS.VPC` 的 `vpc` 行回填为 `eip`/`bandwidth`/`subnet`/`peering`，避免子资源语义混合与 ID 碰撞 |
| `0035` | `0035_cloud_application_id_rune_truncation.up.sql` | Asset：按 sha1 后缀关联账号，把旧实现按字节截取的多字节账号 `cloud-` application_id 无损改写为按字符（rune）截取的 rune 版（与 `cloudApplicationID`/`0032` 的 `left(...,17)` 一致）；同步改写 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy.scope.application_ids`；纯 ASCII 账号无改写；依赖 pgcrypto |
| `0036` | `0036_cloud_application_name_include_account.up.sql` | Asset：迁移历史云同步应用名称包含账号信息，避免多账号同名混淆；当前运行时新建名称仍有未对齐差距 |
| `0037` | `0037_fix_huawei_ces_application_ids.up.sql` | Asset：修复 Huawei CES legacy/new application_id 并存时的安全合并；先迁移并去重子表引用，再删除旧应用；仅 legacy 存在时安全重命名，only new 时幂等 |
| `0038` | `0038_cloud_application_name_normalize.up.sql` | Asset：迁移归一化历史反向格式应用名为 `<provider>-cloud-<account_id>`；当前 `ensureCloudApplication` 新建名称仍未完全对齐该目标，需单独修复 |
| `0039` | `0039_cleanup_orphaned_application_refs.up.sql` | Asset：清理 `0032` DELETE 遗留的 `alert_alert`/`inspection_policy` 孤儿引用，按 `integration_account` 计算 old->new 映射改写为新格式；不依赖 `has_old`；幂等；依赖 pgcrypto |
| `0040` | `0040_application_ref_integrity_view.up.sql` | Asset：创建持久视图 `v_asset_app_ref_integrity`，暴露 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy` 中指向不存在 `asset_application` 的孤儿引用；不修改数据，不阻断迁移；幂等（`CREATE OR REPLACE VIEW`）；验收方式 `SELECT * FROM v_asset_app_ref_integrity` 期望 0 行 |
| `0041` | `0041_legacy_app_id_convergence_guard.up.sql` | Asset：legacy 应用收敛硬阻断守卫，若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用则 `CHECK(n=0)` 失败导致迁移终止；不修改业务数据；若 0041 阻断需排查 0032/0037 收敛失败或代码路径仍在创建旧格式应用，修复后由 `0042` 收口补建 |
| `0042` | `0042_backfill_orphaned_app_refs_and_guard.up.sql` | Asset：补建 0039 改写后仍被引用但不存在的新格式 cloud application ID 对应的 `asset_application` 记录（字段与 `ensureCloudApplication` 一致），并将 `v_asset_app_ref_integrity` 作为硬验收（`CHECK(n=0)`），补建后仍有孤儿则迁移失败；幂等（`ON CONFLICT DO NOTHING`）；依赖 pgcrypto |
| `0043` | `0043_fix_orphaned_alert_app_refs.up.sql` | Asset：修复 `DeleteApplication` 缺失跨上下文引用检查导致的孤儿告警引用；将 `alert_alert` 中指向不存在应用的 `application_id`/`application_name` 置空，移除 `inspection_policy.scope.application_ids` 中的孤儿元素；`CHECK(n=0)` 硬验收确保修复后 `v_asset_app_ref_integrity` 返回 0 行；幂等 |
| `0044` | `0044_lock_default_admin_account.up.sql` | Identity：锁定 `username='admin'` 且 `password_hash` 仍为已知 admin123 哈希的默认管理员（`status=locked`、清空 `password_hash`）；不覆盖 DBA 已设置的强密码；dev/test 在 bootstrap 启用时由 `EnsureBootstrapUser` 重新激活，生产 bootstrap 关闭则保持锁定、须由 DBA 创建安全管理员；幂等 |
| `0045` | `0045_inspection_policy_deleted_scope_cleanup.up.sql` | Asset：回填清空已软删除 `inspection_policy.scope.application_ids`；重建 `v_asset_app_ref_integrity` 仅检查 `deleted=false` 策略；与 `ApplicationReferenceChecker` 及运行时 `SoftDelete` 契约一致；幂等 |
| `0046`–`0047` | `0046_alert_source_pull.up.sql`、`0047_alert_source_pull_state.up.sql` | Alert：告警源拉取接入（`delivery_mode=pull`、华为云 CES V2 告警拉取、绑定账号与区域/间隔校验）及拉取状态列（游标、租约、最近同步结果）；契约 `ops/huawei-ces-alert-sync-contract.md` |
| `0048` | `0048_ai_provider.up.sql` | AI：provider 持久化配置，`api_key` 仅保存 AES-GCM 密文；YAML `ai.providers` 仅在表为空时作为种子写入 |
| `0049` | `0049_ai_session.up.sql` | AI：运维助手会话与消息（多轮对话、反馈、工具调用记录）及 `app:ai:chat` 系列权限 |
| `0050` | `0050_ai_session_message_idempotency.up.sql` | AI：消息幂等键 `client_message_id`，客户端重试去重，避免重复消息与重复模型调用 |
| `0051` | `0051_ai_memory_case.up.sql` | AI：相似案例记忆（RAG-lite）：valid 反馈沉淀、检索注入、`memory_case` 引用 |
| `0052` | `0052_ai_knowledge_item.up.sql` | AI：知识沉淀条目（draft/active/disabled，审核激活后注入 Prompt）+ `app:ai:knowledge:manage` 权限种子 |
| `0053` | `0053_ai_session_share.up.sql` | AI：会话只读分享，token 只存 SHA-256，明文仅创建响应返回一次 |
| `0054` | `0054_ai_memory_manage_permission.up.sql` | AI：记忆管理权限 `app:ai:memory:manage` 种子并绑定 admin |
| `0055` | `0055_ai_alert_rules_tool_permission.up.sql` | AI：规则体检只读工具 `alert.rules.list` 权限种子并绑定 admin；无表结构变更 |
| `0056` | `0056_ai_discovery_tools_permission.up.sql` | AI：上下文发现只读工具 `accounts.list` / `resource.analyze` / `resources.list` 权限种子并绑定 admin；无表结构变更 |
| `0057` | `0057_ai_provider_reasoning_effort.up.sql` | AI：provider 增加 `reasoning_effort`（思考型模型推理强度档位） |
| `0058` | `0058_asset_resource_relation.up.sql` | Asset：CMDB 资源关系表 `asset_resource_relation`（跨应用手工关系；查询期云派生关系见拓扑二期）；复用 `app:assets:read/write`，无新增权限 |
| `0059` | `0059_ai_chat_image_input.up.sql` | AI：助手图片输入（多模态）：`ai_session_message.images_json`（用户消息附图 data URL 数组，最多 3 张、单张解码后 ≤2MB）+ `ai_provider.vision_enabled`（类型 b 视觉模型开关，默认 false，未开启时带图消息 400 不落库）；契约 `ops/ai-contract.md` §10.20 |
| `0060` | `0060_obs_evidence_snapshot.up.sql` | Observability：`obs_evidence_ref` 增加 CES 定稿 payload/hash、查询参数、资源与业务窗口、采集时间、状态及截断元数据；历史证据保持 summary-only |
| `0061` | `0061_postgres_history_tool.up.sql` | AI：种子 `postgres.activity.query`（read_only）并绑定 admin，支持 RDS for PostgreSQL 历史会话、等待事件和 Top SQL 只读诊断 |
| `0062` | `0062_ai_rule_health_tool_permission.up.sql` | AI：种子批量规则体检只读工具 `alert.rules.health` 并绑定 admin；无表结构变更 |

详见 `ops/migration-contract.md`。

**容器化 dev 可选**：使用 `deployments/docker-compose.dev.yml` 为 `api` 开启 `AIOPS_DATABASE__AUTO_MIGRATE=true`（仍走同一 runner），与 `make migrate` **二选一**：

```bash
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
```

迁移就绪后，`GET http://127.0.0.1:8080/readyz` 应返回顶层 `data.status: "ready"`，且 `checks` 中 `migration`、`db` 子项 `status` 均为 `ok`；`redis` 在 `redis.required=true`（生产默认）时须为 `ok`，`redis.required=false` 时 Redis 不可用仅记为 `degraded`，**不阻塞**顶层 `ready`（`cmd/api` 须向 `NewEngine` 注入 DB、Redis，见 `ops/health-contract.md`）。

### 3. 启动后端

```bash
# 准备配置文件（首次）
copy configs\config.example.yaml configs\config.yaml   # PowerShell
# 或：cp configs/config.example.yaml configs/config.yaml

# 拉取依赖（首次）
go mod tidy

# 启动 API
go run ./cmd/api -config configs/config.yaml
```

启动成功后：

- 健康检查：`GET http://127.0.0.1:8080/healthz`
- 就绪检查：`GET http://127.0.0.1:8080/readyz`
- 版本信息：`GET http://127.0.0.1:8080/version`
- 登录（仅受控 dev/test 已激活默认管理员时）：`POST http://127.0.0.1:8080/api/identity/login`，请求体 `{"username":"admin","password":"admin123"}`
- 刷新 token：`POST http://127.0.0.1:8080/api/identity/refresh`，请求体 `{"refresh_token":"..."}`
- 当前用户（鉴权）：`GET http://127.0.0.1:8080/api/identity/me`，需要 `Authorization: Bearer <access_token>`

任意接口的响应均带 `X-Trace-Id`，并在标准 `context.Context` 中闭合，
业务下层可通过 `middleware.TraceIDFromContext(ctx)` 取到完整链路 ID。

> `0016_seed_default_admin_user` 负责种子默认管理员与权限全集，随后的 `0044_lock_default_admin_account` 会锁定仍使用已知默认密码的账号。
> `auth.bootstrap_username` / `auth.bootstrap_password` 仅用于受控 dev/test，可在启动时重新激活默认管理员；生产环境必须留空 bootstrap 配置，并通过 `scripts/provision-prod-admin.ps1` 或等效受控流程创建安全管理员。

### 4. 启动前端

```bash
cd web
npm install      # 首次
npm run dev
```

默认监听 `http://127.0.0.1:5173`。`/api`、`/healthz`、`/readyz`、`/version` 已通过 vite 反向代理到后端 8080。

也可以用容器跑前端（与后端分离）：叠加 `deployments/docker-compose.web.yml` 启动独立 `aiops-web` 容器，访问 `http://127.0.0.1/`。构建说明见 `web/Dockerfile` 与 `deployments/README.md`。

### Docker 镜像

```bash
make docker                    # 产出 aiops-api:$(VERSION)
AIOPS_VERSION=dev docker compose -f deployments/docker-compose.yml build api
docker build -f web/Dockerfile -t aiops-web:dev web   # 前端镜像（上下文=web/）
```

Compose 使用 `aiops-api:${AIOPS_VERSION:-dev}`、`aiops-web:${AIOPS_VERSION:-dev}` 作为镜像标签，与构建命令输出一致。

### AI Provider 配置

AI Provider 以数据库为事实源。API 启动时，如果 Provider 表为空，`configs/config.yaml` 中的 `ai.providers` 会作为种子写入；随后统一从数据库加载运行时注册表。数据库已有记录后，修改 YAML 不会覆盖现有配置，应通过 `POST /api/ai/providers` 或 AI 配置页面维护。Provider 密钥只保存密文，接口不回显明文。详见 `ops/ai-contract.md`。

## 平台约定

### 错误码与响应格式

后端所有 HTTP 接口返回统一结构（见 `pkg/transport/http/response.go`）：

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "8b6...",
  "data": { ... }
}
```

错误码定义在 `pkg/errors/code.go`，遵循「业务错误 → Code → HTTP 状态码」的双向映射。
新增业务错误请使用 `apperr.New` / `apperr.Wrap`，**不要直接返回 `error.New`**。

### DDD 限界上下文

每个限界上下文按 `internal/<context>/{domain,application,infrastructure,interfaces}` 组织，
分层依赖方向：`interfaces → application → domain ← infrastructure`。
新模块对外暴露通过实现 `internal/server.RouteRegistrar` 接口，并在 `cmd/api/main.go`
的 `registrars` 列表中注入。

完整说明见：`internal/identity/README.md`。

### 配置加载

按以下优先级合并（高优先级覆盖低优先级）：

1. 环境变量（前缀 `AIOPS_`，YAML 分段用「双下划线 `__`」分隔，
   例：`AIOPS_DATABASE__PASSWORD=xxx` 对应 `database.password`，
   `AIOPS_AUTH__JWT_SECRET=xxx` 对应 `auth.jwt_secret`）；
   **slice 类型**（如 `cors.allow_origins`）用英文逗号分隔多个值：
   `AIOPS_CORS__ALLOW_ORIGINS=http://localhost:5173,http://127.0.0.1:5173`；
   更推荐在 YAML 中以数组维护。`allow_credentials=true` 时禁止使用 `*` 作为 origin。
2. `--config` 命令行参数指定的文件，或默认的 `configs/config.yaml`；
3. `pkg/config.setDefaults` 注册的默认值（含 `database.auto_migrate: false`）。

`config.Config.Validate()` 会在 bootstrap 阶段拦截致命错误：
端口越界、数据库 host/name 为空、非 dev 环境使用占位/弱 JWT secret（含熵与字符多样性检查）等都会直接终止启动。

从 `aiops-api:1.2` 起，启用 Integration 凭据加密校验：非 dev 环境必须配置独立强密钥
`integration.credential_encryption_key`（环境变量 `AIOPS_INTEGRATION__CREDENTIAL_ENCRYPTION_KEY`），
不得为空、不得使用 `dev-integration-credential-key-change-me` 等占位值、不得与 `auth.jwt_secret`
相同。Kubernetes 部署需把该项放入 `aiops-api-secret`，详见 `deployments/kubernetes.md`。

### 日志

- 第一阶段使用 `zap` 单进程日志；
- 中间件 `Trace` 会把 `trace_id` 注入到 `request.Context`，所有 `logger.From(ctx)` 都会自动携带；
- 接入集中式日志（Loki / 华为云 LTS）的工作放在第二阶段。

### 数据库建表规范（强制）

所有表必须遵循以下约定，新增模块在评审时一律对照检查：

1. **主键统一为自增序列**：列名 `id`，类型 `BIGSERIAL`（PostgreSQL）。
   - 不允许使用业务标识（用户名、邮箱、UUID、订单号等）做主键。
2. **业务标识独立成列**：
   - 例如 `iam_user.user_id VARCHAR(36) UNIQUE`、`alert.alert_id`、`asset.asset_id`。
   - 跨上下文 / 对外 API 的引用一律走业务标识，**不要暴露 DB 自增 `id`**。
3. **时间戳必填且由程序维护**：
   - 每张表必须有 `created_at` / `updated_at`，类型 `TIMESTAMPTZ NOT NULL`，**禁止设置 DB DEFAULT、禁止使用触发器**。
   - 时间戳由 Go 程序通过 `pkg/database.BaseModel` 的 GORM `BeforeCreate` / `BeforeUpdate` 钩子在 INSERT/UPDATE 前设置：
     - INSERT：`created_at` 与 `updated_at` 同步置为 `time.Now()`（若上层显式赋值则保留）；
     - UPDATE：`updated_at` 始终被刷新为 `time.Now()`。
   - 主流程**不允许使用 `UpdateColumn` / `UpdateColumns` / 原生 `db.Exec` 更新**，否则钩子不会触发，需要手动维护时间戳。
   - **例外**：`schema_migrations.applied_at` 为迁移 runner 内部元数据，见 `ops/migration-contract.md`。
4. **新建模型示例**：

   ```go
   import "github.com/734965549/aiops/pkg/database"

   type AlertModel struct {
       database.BaseModel                                      // 提供 id / created_at / updated_at
       AlertID string `gorm:"column:alert_id;type:varchar(36);uniqueIndex;not null"`
       // ... 业务字段
   }
   func (AlertModel) TableName() string { return "alert" }
   ```

### 运维契约索引

- `ops/migration-contract.md`：数据库迁移责任边界、执行方式与回滚原则。
- `ops/health-contract.md`：`/healthz`、`/readyz` 的对外健康检查契约。
- `ops/auth-contract.md`：登录、JWT 签发与刷新、密钥约束。
- `ops/identity-api-contract.md`：身份、角色/权限管理、当前用户、统一授权校验、LDAP 导入。
- `ops/alert-contract.md`：告警源管理、Webhook ingest、状态流转。
- `ops/huawei-ces-alert-sync-contract.md`：华为云 CES 告警拉取、游标、租约、手动同步与可选 worker。
- `ops/runbook-contract.md`：处置预案模板、告警推荐、多步骤任务生成。
- `ops/execution-contract.md`：执行任务创建、确认、执行与时间线回写。
- `ops/execution-agent-contract.md`：执行介体、Agent 控制面、Command Spec、租约和日志回传；各章节需区分已实现与规划项。
- `ops/audit-contract.md`：操作审计写入、精确检索、当前页聚合、CSV 实际上限、认证审计和防篡改边界。
- `ops/dashboard-contract.md`：Dashboard 只读聚合、降级、推荐组装、前端跳转与验证边界。
- `ops/ai-contract.md`：Provider、工具网关、AI 助手、记忆/知识、分享、多模态、证据与 RDS 诊断。
- `ops/cloud-observability-contract.md`：云账号只读接管、统一观测查询、巡检策略和建议转执行契约。
- `ops/huawei-ces-sync-contract.md`：华为云 CES 资源同步范围、模式、批次、计数与对账契约。

## 当前能力摘要

### 已打通的 P0 闭环

1. **告警接入**：Alertmanager Webhook 入库、去重、状态流转（认领/处理/恢复/关闭）。
2. **资产匹配**：默认 `ops/huawei-ces-sync-contract.md` §9.1 标签匹配 + 用户可配置 glob 规则（`asset_match_rule`）。
3. **Runbook 推荐**：按告警标签/严重级别匹配预案，支持多步骤 dry-run 执行。
4. **执行确认**：`pending_confirm` → 人工 CONFIRM → 执行 → 结果回写告警时间线。
5. **Dashboard 汇总**：活跃告警、待确认执行、资产计数、最近执行与 Runbook 使用。
6. **审计追溯**：资产变更、告警处理、Runbook 启停、执行确认/拒绝、AI 分析等已接入动作可通过操作审计查询；登录/refresh/logout 走 Identity 认证审计独立接口。审计写入多为 best-effort，业务成功不等于审计必定落库。

### 认证与权限

- 本地账号、LDAP/AD、OAuth2/OIDC 登录；JWT access/refresh；RBAC + 数据权限 + AI 工具权限。
- 路由层 `AuthRequired` + 模块授权中间件 + `AuthorizationService` + AI 工具网关二次校验。
- 域账号须管理员预置绑定，不支持按用户名自动开户。

### 前端

- Vue 3 + Vite + Arco Design；路由懒加载；Dashboard vendor 分包（主业务 chunk < 100KB）。
- 主页面：驾驶舱、告警、资产与资源拓扑、Runbook、执行、Integration、Observability、Inspection、AI 助手、审计中心。
- 系统与身份页面：AI Provider/工具/知识/记忆配置、域账号导入、用户角色、角色权限、数据范围与 AI 工具权限管理。
- AI 助手同时提供独立工作台、全局浮层和登录后的只读分享页；工作台与浮层共享会话状态，消息排队属于前端内存能力，不是服务端持久队列。
- 路由守卫当前只校验登录状态，侧栏为静态配置；接口权限仍由后端强制校验，不能把“页面可进入”理解为“拥有全部操作权限”。
- 审计中心走 `GET /api/audits`，支持资源、用户、动作、Trace 精确筛选和详情；Resource/User/Trace 聚合仅基于当前页，CSV 实际最多导出第一页 100 条，资源类型快捷选项尚有实现差距。

### 已落地的 P1 权限管理能力

| 方向 | 说明 | 验证边界 |
|------|------|----------|
| 权限管理写接口 | 用户角色、角色权限、数据范围、AI 工具权限配置与变更审计 | `e2e-identity-access.ps1` 主要覆盖用户角色、viewer 403 和对应审计；其他写接口需结合单元测试、UI 与验收清单逐项验证 |

### 待完善（P1+）

| 方向 | 说明 |
|------|------|
| 菜单权限 | 按权限动态隐藏侧栏入口 |
| 更多告警源 | SigNoz 等其他平台接入（华为云 CES Webhook/拉取已落地） |
| K8s 同步 | K8s 资产自动发现（云资源同步与 CMDB 资源拓扑已落地） |
| 真实日志/链路 Provider | 华为 AOM/LTS、APM 与 SigNoz 真实适配（当前为 fake，真实凭据显式 unsupported） |
| MFA | 多因素认证 |

## 设计文档

- `docs/demo-flow.md` — 演示步骤与自动化验收
- `docs/release-checklist.md` — 上线前检查清单
- `docs/acceptance-checklist.md` — 模块验收明细
- `docs/AI运维平台整体流程与调用关系.md` — 全链路图、DDD 调用关系与边界说明
- `docs/AI运维平台调用关系图.md` — 关键模块与时序调用图
- `deployments/kubernetes.md` — Kubernetes 部署参考（外挂 PostgreSQL/Redis）；迁移与部分生产配置待按当前契约修订
- `docs/AI运维平台核心业务流程图.md`
- `docs/AI运维平台信息架构.md`
- `docs/AI运维平台技术架构设计.md`
- `docs/AI运维平台页面原型.md`

契约与复核记录：

- `ops/README.md`：全部运维契约索引及契约到代码的对应关系。
- `web/src/api/README.md`：前端 API 封装、页面入口和调用边界。
- `docs/documentation-review.md`：文档与当前实现的复核口径、已确认差距和逐模块更新队列。

## 云厂商只读接管与观测智能体现状

除 P0 闭环外，仓库已经落地 Integration 账号接入、华为云 CES 资源同步与告警拉取、Observability 统一查询、手动 Inspection、AI 只读工具循环，以及 Execution Agent 服务端控制面。这些能力不能继续统一写成“下一阶段新增”，但也不能把模块存在理解为所有 Provider 和自动化能力都已完成。

当前主要边界：

- 华为云指标、资源发现和告警规则已有真实适配；真实日志、链路、拓扑及 SigNoz/Prometheus 适配范围以 `ops/cloud-observability-contract.md` 为准，部分路径仍为 fake 或显式 unsupported。
- 资产同步与 CES 告警手动拉取已落地；告警后台拉取 worker 默认关闭，巡检定时 worker 尚未落地。
- AI 证据定稿快照已持久化并用于模型输入校验，但普通观测查询仍以摘要为主，快照没有公共详情查询 API/UI。
- Execution Agent 的注册、心跳、租约、日志与结果协议已落地；独立 Agent 程序和专门管理 UI 未随仓库提供。
- Notification 等未形成独立业务上下文的能力仍属于规划项。

AI/Agent 仍只负责分析、建议和生成计划。任何真实变更必须进入 Execution 模块，并接受权限、风险、确认和审计约束。

详细设计与实施边界见：

- `docs/cloud-observability-agent-roadmap.md`：DDD 上下文、阶段步骤、数据模型、工作流和验收策略；阅读时需区分已落地阶段与后续目标。
- `docs/AI运维平台整体流程与调用关系.md`：说明 P0、Integration、Observability、Inspection、Execution Agent 如何串联，改代码前建议先读。
- `ops/cloud-observability-contract.md`：云账号接入、指标/日志/链路查询、巡检策略和建议到执行的 API 契约。
- `ops/huawei-ces-sync-contract.md`：华为云 CES 资源同步稳定契约。
- `ops/huawei-ces-alert-sync-contract.md`：华为云 CES 告警拉取契约。
- `docs/adr-huawei-ces-sync.md`：华为云 CES 同步架构决策记录。
- `docs/huawei-ces-sync-runbook.md`：华为云 CES 同步运维步骤。
- `docs/huawei-ces-sync-backlog.md`：华为云 CES 同步已知缺口与待办。
- `ops/execution-agent-contract.md`：执行介体、Agent 控制面、Command Spec、租约、日志回传和确认后执行契约；已实现章节与规划章节分别标注。
