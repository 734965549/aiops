# Codex Project Instructions

> Generated from `.cursor/rules/*.mdc` so Codex and Cursor share the same project guidance.

<!-- Source: .cursor/rules/00-aiops-project-charter.mdc -->

# AI 运维平台项目宪章

本项目是面向企业运维场景的 AI 运维平台。当前已打通 P0 闭环：

```text
告警接入 -> 资产匹配 -> Runbook 推荐 -> 执行确认 -> Dashboard 汇总 -> 审计追溯
```

AI/Agent 在本仓库协作时，优先维护这个闭环的稳定性。任何新功能、重构或 UI 调整，都不能破坏已落地的主链路和验收脚本。

## 核心理念

- 平台不是“让 AI 直接操作生产”的工具，而是安全、可控、可审计的智能运维闭环。
- AI 负责分析、解释、建议和生成计划；真实执行必须进入 Execution 模块，由权限、风险、确认、审计共同约束。
- 外部系统通过适配器接入，业务代码不直接绑定具体云厂商、日志系统、监控系统或模型供应商。
- 第一阶段坚持 DDD 模块化单体，按限界上下文组织代码；后续可按模块拆分为独立服务。
- 关键行为必须可追踪：请求有 `trace_id`，写操作有审计，执行动作有状态机和时间线。

## 权威文档

改动前先查对应契约，不要凭记忆改接口：

- 产品和设计：`README.md`、`docs/AI运维平台技术架构设计.md`、`docs/AI运维平台信息架构.md`、`docs/AI运维平台核心业务流程图.md`
- 验收：`docs/acceptance-checklist.md`、`docs/demo-flow.md`、`docs/release-checklist.md`
- 运维/API 契约：`ops/*.md`
- 前端 API：`web/src/api/README.md`

当源码与文档不一致时，先判断是否是代码已经演进但文档未更新。涉及对外接口、状态机、权限码、迁移、健康检查时，必须同步更新契约文档。

## 全局硬约束

- 后端统一响应结构为 `code/message/trace_id/data`，成功码是字符串 `"OK"`，不是数字 `0`。
- 后端业务错误使用 `pkg/errors` 的 `apperr.New` / `apperr.Wrap`，HTTP 输出使用 `pkg/transport/http` 的 `OK` / `Fail` / `FailWith`。
- 受保护 API 必须走 Bearer Token、RBAC/数据权限/工具权限校验；401 与 403 语义要区分。
- 任意写操作、执行确认、告警处理、AI 工具调用等关键动作必须写审计或预留审计 hook。
- 不把密钥、Token、AK/SK、JWT secret、数据库密码写进代码、日志、Prompt、测试快照或前端持久化状态。
- 不暴露数据库自增 `id` 作为跨上下文或对外 API 的业务标识；对外使用 `*_id` 业务 ID。
- 不引入另一套迁移工具；本项目只使用自研 runner。

## 当前技术栈

- 后端：Go 1.22、Gin、GORM、PostgreSQL、Redis、Viper、zap、JWT。
- 前端：Vue 3、TypeScript、Vite、Pinia、Vue Router、Arco Design、Axios。
- 部署：Docker Compose；本地 dev 默认 API `8080`，前端 `5173`。

## 协作方式

- 先读相关 README/契约/测试，再改代码。
- 小步修改，保持模块边界清晰；不要借需求顺手做大规模无关重构。
- 新增能力时优先补齐对应测试、E2E 脚本或验收清单。
- 如果实现和契约发生冲突，优先停下来说明冲突点，并建议是改代码、改契约还是拆分阶段处理。

<!-- Source: .cursor/rules/10-go-backend-ddd.mdc -->

# Go 后端规则

## 分层与依赖

每个业务上下文按 `internal/<context>/{domain,application,infrastructure,interfaces}` 组织。

- `domain`：实体、值对象、状态机、领域错误、Repository 接口，不依赖 Gin/GORM/Redis。
- `application`：用例编排、事务边界、权限意图、审计 hook、跨模块端口接口。
- `infrastructure`：GORM/Redis/外部系统/审计 recorder 等实现。
- `interfaces/http`：Gin Handler、路由注册、DTO 绑定与响应转换。

依赖方向保持：

```text
interfaces -> application -> domain <- infrastructure
```

不要让 Handler 直接操作 GORM，也不要让 domain 直接 import infrastructure。

## 新模块接入

- 新上下文放在 `internal/<context>`，保持现有目录风格。
- HTTP 路由通过实现 `internal/server.RouteRegistrar` 暴露。
- 在 `cmd/api/main.go` 的 `registrars` 列表装配模块依赖。
- 跨上下文调用优先使用 application 层端口接口，不直接 import 对方 infrastructure。
- 共享基础能力才放进 `pkg/`；业务专属逻辑不要抽到 `pkg/`。

## HTTP 响应和错误

- 所有业务接口使用 `httpx.OK`、`httpx.Fail`、`httpx.FailWith`。
- 成功响应固定 `code="OK"`、`message="ok"`，并闭合 `trace_id`。
- 新增错误码先看 `pkg/errors/code.go` 和 HTTP 映射；业务错误用 `apperr.New` / `apperr.Wrap`。
- 不直接把数据库、Redis、外部 API 原始错误文案暴露给调用方；底层详情写日志。
- 分页接口沿用 `pkg/pagination` 和现有 `PageData` 形态。

## 权限与安全

- 受保护路由必须挂认证中间件，并使用模块授权或 application service 做权限校验。
- 权限码沿用 `app:<module>:<action>` 风格，例如 `app:executions:confirm`。
- AI 工具调用必须二次校验工具权限和确认状态，不能只依赖前端。
- 域账号登录只接受管理员预置绑定，不自动按用户名开户。
- JWT secret、provider api key、LDAP 密码等敏感值不能进入日志、响应、审计明文或测试 fixture。

## 审计与可观测

- 登录、权限变更、资产变更、告警状态流转、Runbook 启停、执行确认/拒绝/执行、AI 分析与工具调用都属于关键动作。
- 新增关键写操作时，添加审计 hook 或复用已有 recorder。
- 日志使用 `logger.From(ctx)`，不要绕过 request context；让 `trace_id` 自动进入日志。
- `/healthz` 只检查进程存活；`/readyz` 才检查 config、migration、db、redis。

## 测试要求

- domain 状态机、枚举、匹配规则、权限判断必须有单元测试。
- application 服务触及状态流转、审计、事务或幂等时必须补测试。
- HTTP Handler 新增接口时，补成功、鉴权失败、参数错误、权限不足等主路径测试。
- 修改共享包 `pkg/*` 时，跑或补对应包测试。

<!-- Source: .cursor/rules/20-database-migrations.mdc -->

# 数据库与迁移规则

## 迁移执行器

本项目只允许使用仓库内自研迁移 runner：`pkg/database/migrate.go`。

- 允许：`make migrate`、`make migrate-up`、`go run ./cmd/migrate -config configs/config.yaml`。
- 禁止：golang-migrate、`docker-entrypoint-initdb.d` 手工 `\i`、任何第三方迁移工具与 runner 混用。
- `database.auto_migrate=true` 仅限本地/dev/test，生产必须由 DBA 或发布流水线在 API 启动前显式迁移。
- `*.down.sql` 是人工回滚参考，不由 runner 自动执行。

## 迁移文件

- 文件名格式：`<version>_<name>.up.sql` / `<version>_<name>.down.sql`。
- 版本号使用 4 位递增编号，例如 `0015`；不要插队、复用或改写已发布版本。
- 同一上下文表名前缀保持一致：`iam_*`、`alert_*`、`asset_*`、`exec_*`、`audit_*` 等。
- up 脚本尽量幂等，种子数据使用 `ON CONFLICT`。
- 新增权限、角色、AI 工具权限、Dashboard 权限等种子数据时，同步考虑 admin 角色绑定。

## 建表硬约束

所有业务表遵守：

- 主键统一为 `id BIGSERIAL`。
- 业务标识独立唯一列，例如 `user_id`、`alert_id`、`asset_id`、`task_id`。
- 对外 API 和跨上下文引用使用业务 ID，不暴露自增 `id`。
- 每张业务表必须有 `created_at TIMESTAMPTZ NOT NULL` 和 `updated_at TIMESTAMPTZ NOT NULL`。
- 业务表时间戳由 Go 程序维护，禁止 DB DEFAULT，禁止触发器。
- 不随意添加数据库外键；若项目已有“应用层保证完整性”的模式，继续使用事务、唯一索引、存在性校验和 repository 约束。

例外：`schema_migrations.applied_at` 是 runner 元数据，可保留 `DEFAULT NOW()` 兜底。

## GORM 模型

- 新模型嵌入 `pkg/database.BaseModel`，由 hook 维护 `id/created_at/updated_at`。
- `TableName()` 明确返回表名。
- 不在主流程使用 `UpdateColumn` / `UpdateColumns` / 原生 `db.Exec` 更新业务表；这些会绕过 GORM hook。确需使用时，必须手动维护 `updated_at` 并在代码注释中说明原因。
- Repository 方法保持事务边界清楚；创建主从数据时用同一事务。

## 迁移验收

- 修改迁移 runner 或 SQL splitter 时，跑 `go test ./pkg/database/...`。
- 新增迁移后，确认 `/readyz` 的 migration 子项能正确反映 latest/applied/pending。
- 涉及前端权限报错提示时，同步检查 `web/src/api/request.ts` 是否仍能给出有效提示。

<!-- Source: .cursor/rules/30-web-frontend.mdc -->

# 前端规则

## 技术栈与目录

前端位于 `web/`，使用 Vue 3 + TypeScript + Vite + Pinia + Vue Router + Arco Design。

- API 封装放在 `web/src/api/`。
- 页面放在 `web/src/views/<module>/`。
- 全局状态放在 `web/src/stores/`。
- 路由放在 `web/src/router/`。
- 页面级组合逻辑可放在当前 view 的 `composables/`，避免过早抽全局工具。

## API 调用

- 所有后端请求经 `web/src/api/request.ts`，不要在页面里直接裸用 axios。
- 成功判断使用 `code === "OK"`，不要使用数字 `0`。
- `request.ts` 已处理 401 refresh、403 权限提示、503 未就绪提示；新增模块 API 保持这个错误处理链路。
- AI 工具调用即使 HTTP 200 且 `code="OK"`，也必须检查 `data.allowed`。
- 新增 API 文件时，同步更新 `web/src/api/README.md`。

## 权限与路由体验

- 未登录访问受保护页面应回到 `/login`。
- 403 不应造成白屏；页面应保留结构并给出可理解提示。
- 权限码、迁移依赖、接口路径以 `ops/*.md` 和后端实现为准。
- 菜单权限尚属 P1+ 能力，新增入口时不要假设完整动态菜单已经落地。

## UI 风格

- 这是运维控制台，优先清晰、紧凑、可扫描；避免营销页式大标题和装饰性视觉。
- 页面首屏应直接服务工作流：告警列表、任务列表、Dashboard 指标、表单或详情。
- 表格、筛选、状态 Tag、抽屉、确认弹窗、空状态、加载状态、错误状态要完整。
- 高风险执行动作必须有明显确认，不要只靠一个普通按钮。
- 敏感值如 provider `api_key` 只显示掩码和 `has_api_key`，不要在表单回显明文。

## 构建与验证

- 常规验证：`cd web && npm run build`。
- 修改 lint 相关代码时可跑 `cd web && npm run lint`。
- 联调时前端 dev server 默认 `http://127.0.0.1:5173`，`/api`、`/healthz`、`/readyz`、`/version` 代理到后端 `8080`。
- 如果 Windows 网盘映射路径导致 Vite 构建异常，先查看 `web/README.md` 和 `vite.config.ts` 中关于 `realpathSync` 的说明。

<!-- Source: .cursor/rules/40-contracts-and-verification.mdc -->

# 契约与验收规则

## 契约优先级

`ops/*.md` 是前后端联调和运维接入的稳定契约。修改接口、状态机、权限码、健康检查、迁移行为、执行流程时，必须同步更新对应契约。

常用映射：

- 认证/JWT/登录：`ops/auth-contract.md`
- 身份、角色、LDAP 导入、授权校验：`ops/identity-api-contract.md`
- 告警源、Webhook、状态流转：`ops/alert-contract.md`
- 执行任务、确认、状态机、时间线：`ops/execution-contract.md`
- Runbook 模板、推荐、多步骤任务：`ops/runbook-contract.md`
- AI provider、工具调用、allowed 语义：`ops/ai-contract.md`
- 健康检查与 readiness：`ops/health-contract.md`
- 数据库迁移：`ops/migration-contract.md`

## 主链路验收

主链路必须保持可跑：

```powershell
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
go run ./cmd/migrate
go test ./cmd/... ./internal/... ./pkg/...
cd web && npm run build
.\scripts\e2e-alert.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1
```

根据改动范围选择最小必要验证；触及共享模块、权限、迁移、状态机、执行闭环时扩大验证范围。

## 健康检查契约

- `/healthz` 是 liveness，只检查进程，顶层 `data.status` 为 `ok`。
- `/readyz` 是 readiness，检查 process/config/migration/db/redis；只有顶层 `data.status == "ready"` 才能接流量。
- `checks[*].status` 可为 `ok/down/degraded`；不要把子项 `degraded` 当作顶层已就绪。
- `redis.required=false` 时 Redis 异常可 degraded 且不阻塞顶层 ready；`redis.required=true` 时必须阻塞。

## 发布和配置

- 默认 Compose 不自动建表；生产不能依赖 `database.auto_migrate=true`。
- 生产环境必须关闭 bootstrap 管理员配置，使用 secrets 注入 JWT 和数据库密码。
- 不向生产暴露 PostgreSQL/Redis 端口。
- CORS `allow_credentials=true` 时禁止 `*` origin。
- 配置优先级：`AIOPS_` 环境变量 > `--config` YAML > 默认值。

## 文档维护

- README 用于项目入口和快速验收，不承载过细接口细节。
- `ops/*.md` 写稳定契约；`docs/*.md` 写设计、流程、验收、发布建议。
- 新增 E2E 脚本时，在 `docs/acceptance-checklist.md` 增加推荐验收顺序或模块检查项。

<!-- Source: .cursor/rules/50-aiops-safety-and-evolution.mdc -->

# AI 运维安全与演进规则

## 安全边界

- AI 助手不是执行器；它只能分析、建议、生成计划或创建待确认任务。
- 真实动作必须进入 Execution 模块，并经过状态机、权限、风险等级、人工确认和审计。
- 中高风险动作默认 `pending_confirm`，确认文本沿用 `CONFIRM` 语义，大小写敏感。
- 工具调用继承当前用户身份，不允许使用“系统超级权限”绕过 RBAC、数据范围或工具权限。
- 凭据不进入 Prompt，不进入执行日志，不进入审计明文。

## 执行闭环

Execution 第一阶段状态机：

```text
pending_confirm -> pending_execute -> running -> success|failed
```

低风险可创建后直达 `pending_execute`；medium/high/critical 必须确认。

来源为 alert 的执行任务要回写告警时间线：

- `execution_created`
- `execution_started`
- `execution_finished`

Runbook 多步骤任务要保留步骤顺序、dry-run 标记、输出和失败原因。

## AI Provider 与工具

- provider 列表不返回明文 `api_key`；已设置时显示 `****` 并返回 `has_api_key=true`。
- 更新 provider 时如果省略 `api_key`，保留原密钥。
- 工具调用响应 `allowed=false` 不是异常，而是业务拒绝；前端必须展示 `reason`。
- Provider 类型保持现有 `a/b/c` 语义，新增类型前先更新 `ops/ai-contract.md`。

## 设计建议

以下是项目演进建议，不要误当成已经全部落地的功能：

- P1 优先补齐审计中心 UI：筛选、导出、按 trace/resource/user 聚合查看。
- 权限管理写接口要和审计绑定，避免“能改权限但不可追溯”。
- 菜单权限应基于后端权限结果动态隐藏入口，但 API 仍必须服务端校验。
- 更多告警源、CMDB、K8s、日志/指标系统接入时，先定义 Provider/Adapter 接口，再接具体厂商。
- 真实执行代理上线前，先实现命令白名单、参数 schema 校验、超时、重试策略、幂等键和回滚计划。
- AI 输出建议应带证据来源、风险等级、不确定性说明，并区分“建议操作”和“可执行计划”。

## 反模式

- 不让 AI 或前端直接调用云 API、K8s API、SSH、数据库写操作。
- 不在页面里用“确认弹窗”替代后端确认状态机。
- 不把 runbook 步骤当作纯文本丢失结构化字段。
- 不为了演示方便绕过权限、审计或风险控制；演示数据也要走真实链路。
