# AI 运维平台

> 面向智能化、自动化、闭环化的企业级 AI 运维平台。
> 当前阶段：**P0 业务闭环已打通**，支持告警接入 → 资产匹配 → Runbook 推荐 → 执行确认 → Dashboard 汇总 → 审计追溯的完整演示链路。

## 模块状态

| 模块 | 后端 API | 前端页面 | E2E 脚本 | 说明 |
|------|----------|----------|----------|------|
| Identity 认证与权限 | ✅ | 登录 / 域账号导入 / 权限管理 | `e2e-identity-access.ps1` | JWT、RBAC、LDAP/OAuth、权限管理、审计 |
| Alert 告警中心 | ✅ | `/alerts` | `e2e-alert.ps1` | Alertmanager ingest、状态流转、AI 分析入口 |
| Asset 资源与应用 | ✅ | `/assets` | `e2e-asset.ps1` | 应用/资源 CRUD、可配置匹配规则 |
| Runbook 处置预案 | ✅ | `/runbooks` | `e2e-runbook.ps1` | 模板管理、告警推荐、多步任务生成 |
| Execution 自动化执行 | ✅ | `/executions` | `e2e-execution.ps1` | 人工确认、执行代理、时间线回写 |
| Dashboard 首页驾驶舱 | ✅ | `/dashboard` | API 抽检 | 告警/执行/资产/Runbook 聚合摘要 |
| Audit 审计中心 | ✅ API | `/audits` | UI 查询/导出 | 关键操作审计写入、筛选、详情查看与 CSV 导出 |
| AI 运维助手 | ✅ API | `/ai-assistant` | — | Provider 管理、告警分析、工具调用 |

**文档**：
- [演示流程](docs/demo-flow.md) — 10 步完整闭环演示
- [上线检查清单](docs/release-checklist.md) — 发布前必查项
- [验收清单](docs/acceptance-checklist.md) — 模块级验收明细
- [Kubernetes 部署说明](deployments/kubernetes.md) — 外挂 PostgreSQL/Redis 的 K8s 部署参考

## 快速验收

```powershell
# 环境
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
go run ./cmd/migrate

# 测试与构建
go test ./cmd/... ./internal/... ./pkg/...
cd web && npm run build

# 业务链路 E2E
.\scripts\e2e-alert.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1
.\scripts\e2e-identity-access.ps1
```

## 目录结构

```
aiops/
├── cmd/                      # 程序入口（按可执行文件分子目录）
│   ├── api/                  # HTTP API 主入口
│   └── migrate/              # 数据库迁移独立入口（make migrate）
├── internal/                 # 平台内部实现，禁止外部直接 import
│   ├── bootstrap/            # 配置/日志/数据库装配
│   ├── server/               # Gin 引擎、HTTP server、RouteRegistrar 接口
│   ├── version/              # 版本号信息（编译期注入）
│   ├── identity/             # 认证、RBAC、LDAP/OAuth
│   ├── alert/                # 告警接入、状态流转、资产绑定
│   ├── asset/                # 应用/资源注册、匹配规则
│   ├── runbook/              # 处置预案模板与推荐
│   ├── execution/            # 执行任务编排与确认
│   ├── dashboard/            # 首页聚合摘要
│   ├── audit/                # 操作审计查询
│   └── ai/                   # AI Provider、工具网关、告警分析
├── pkg/                      # 跨模块共享库（可被 internal 与未来 cmd 复用）
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
| Go | 1.22+ | 后端运行时 |
| Node.js | 18+ | 前端运行时 |
| Docker / Docker Desktop | 任意近期版本 | 启动 PostgreSQL + Redis |
| Make | 可选 | 使用根目录 `Makefile` 简化命令 |

### 1. 启动中间件

| 模式 | 命令 | 适用场景 |
| --- | --- | --- |
| **仅中间件** | `docker compose -f deployments/docker-compose.yml up -d postgres redis` | 本地 `go run ./cmd/api` 联调（推荐） |
| **全栈（默认）** | `docker compose -f deployments/docker-compose.yml up -d` | 容器化 API + PG + Redis |
| **全栈 dev 就绪** | `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d` | 自动迁移 + bootstrap 管理员 + 挂载 `config.yaml` |

```bash
# 仅 PostgreSQL + Redis（本地 go run 联调常用）
docker compose -f deployments/docker-compose.yml up -d postgres redis

# 全栈：API 会启动，但默认不建表 → /readyz 可能 not_ready
docker compose -f deployments/docker-compose.yml up -d

# 全栈 dev 就绪：推荐首次容器联调使用（AUTO_MIGRATE + admin 账号）
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
```

> **注意**：默认 Compose **不会**自动建表。PostgreSQL 未挂载 `docker-entrypoint-initdb.d`，且 `database.auto_migrate` 在代码默认值、`config.example.yaml`、`.env.example` 与主 `docker-compose.yml` 中均为 `false`；仅当显式设为 `true` 时 `bootstrap.Init` 才会执行迁移。全栈默认模式需先 `make migrate` 或叠加 `docker-compose.dev.yml`。

> **安全**：Compose 主文件将 PostgreSQL（5432）、Redis（6379）映射到宿主机；PG 默认 `aiops/aiops`，完整迁移会种子 Web 登录 `admin/admin123`，**仅限本机开发或受控初始化**。生产环境勿发布 DB/Redis 端口，须 `redis.required=true`、通过 secrets 注入 JWT 与数据库密码，关闭 bootstrap，并在发布后立即改密或禁用默认账号。

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

当前迁移版本（`0001` → `0017`，顺序不可打乱）：

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
- 登录：`POST http://127.0.0.1:8080/api/identity/login`，请求体 `{"username":"admin","password":"admin123"}`
- 刷新 token：`POST http://127.0.0.1:8080/api/identity/refresh`，请求体 `{"refresh_token":"..."}`
- 当前用户（鉴权）：`GET http://127.0.0.1:8080/api/identity/me`，需要 `Authorization: Bearer <access_token>`

任意接口的响应均带 `X-Trace-Id`，并在标准 `context.Context` 中闭合，
业务下层可通过 `middleware.TraceIDFromContext(ctx)` 取到完整链路 ID。

> 完整迁移会通过 `0016_seed_default_admin_user` 种子默认管理员 `admin/admin123`，并把 `admin` 角色绑定到当前全部权限；
> `auth.bootstrap_username` / `auth.bootstrap_password` 仅保留为 dev/test 兼容链路。
> 生产环境必须留空 bootstrap 配置，并在发布后立即改密或禁用默认账号。

### 4. 启动前端

```bash
cd web
npm install      # 首次
npm run dev
```

默认监听 `http://127.0.0.1:5173`。`/api`、`/healthz`、`/readyz`、`/version` 已通过 vite 反向代理到后端 8080。

### Docker 镜像

```bash
make docker                    # 产出 aiops-api:$(VERSION)
AIOPS_VERSION=dev docker compose -f deployments/docker-compose.yml build api
```

Compose 使用 `aiops-api:${AIOPS_VERSION:-dev}` 作为镜像标签，与 `make docker` 输出一致。

### AI Provider 配置

`configs/config.example.yaml` 中 `ai.providers` 会在 API 启动时载入内存注册表。若纯环境变量启动、无 YAML，可在登录后通过 `POST /api/ai/providers` 手动创建。详见 `ops/ai-contract.md`。

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
- `ops/identity-api-contract.md`：角色/权限只读查询、当前用户、统一授权校验、LDAP 导入。
- `ops/alert-contract.md`：告警源管理、Webhook ingest、状态流转。
- `ops/execution-contract.md`：执行任务创建、确认、执行与时间线回写。
- `ops/runbook-contract.md`：处置预案模板、告警推荐、多步骤任务生成。
- `ops/ai-contract.md`：AI 模块 provider 管理、工具调用与前端交互契约。

## 当前能力摘要

### 已打通的 P0 闭环

1. **告警接入**：Alertmanager Webhook 入库、去重、状态流转（认领/处理/恢复/关闭）。
2. **资产匹配**：默认 §9.1 标签匹配 + 用户可配置 glob 规则（`asset_match_rule`）。
3. **Runbook 推荐**：按告警标签/严重级别匹配预案，支持多步骤 dry-run 执行。
4. **执行确认**：`pending_confirm` → 人工 CONFIRM → 执行 → 结果回写告警时间线。
5. **Dashboard 汇总**：活跃告警、待确认执行、资产计数、最近执行与 Runbook 使用。
6. **审计追溯**：登录、资产变更、告警处理、Runbook 启停、执行确认/拒绝、AI 分析等关键操作均可查询。

### 认证与权限

- 本地账号、LDAP/AD、OAuth2/OIDC 登录；JWT access/refresh；RBAC + 数据权限 + AI 工具权限。
- 路由层 `AuthRequired` + 模块授权中间件 + `AuthorizationService` + AI 工具网关二次校验。
- 域账号须管理员预置绑定，不支持按用户名自动开户。

### 前端

- Vue 3 + Vite + Arco Design；路由懒加载；Dashboard vendor 分包（主业务 chunk < 100KB）。
- 页面：登录、驾驶舱、告警、资产（含匹配规则 Tab）、Runbook、执行、AI 助手、审计中心、域账号导入、权限管理。
- 审计中心走 `GET /api/audits`，支持资源、用户、动作筛选，详情抽屉和 CSV 导出。

### 已验收的 P1 能力

| 方向 | 说明 | 验收 |
|------|------|------|
| 权限管理写接口 | 用户角色、角色权限、数据范围、AI 工具权限配置与变更审计 | `e2e-identity-access.ps1` |

### 待完善（P1+）

| 方向 | 说明 |
|------|------|
| 菜单权限 | 按权限动态隐藏侧栏入口 |
| 更多告警源 | 华为云 CES、SigNoz 等 |
| CMDB / K8s 同步 | 资产自动发现与拓扑 |
| MFA | 多因素认证 |

## 设计文档

- `docs/demo-flow.md` — 演示步骤与自动化验收
- `docs/release-checklist.md` — 上线前检查清单
- `docs/acceptance-checklist.md` — 模块验收明细
- `deployments/kubernetes.md` — Kubernetes 部署说明（外挂 PostgreSQL/Redis）
- `docs/AI运维平台核心业务流程图.md`
- `docs/AI运维平台信息架构.md`
- `docs/AI运维平台技术架构设计.md`
- `docs/AI运维平台页面原型.md`

契约与前端 API 说明：

- `ops/auth-contract.md`
- `ops/identity-api-contract.md`
- `ops/alert-contract.md`
- `ops/execution-contract.md`
- `ops/runbook-contract.md`
- `ops/ai-contract.md`
- `web/src/api/README.md`
