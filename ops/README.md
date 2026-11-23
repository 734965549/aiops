# ops 契约说明

`ops` 目录保存平台对外接口、状态机、权限、迁移和运维契约。后端实现、前端联调、测试验收和运维接入应优先对齐这里的内容；源码已经演进时，必须同步更新契约，不能仅凭文档标题判断能力已经落地。

契约可能同时包含已实现章节和规划章节。阅读时应结合章节状态、根 README 的模块矩阵及实际代码路径；`draft`、`规划`、`未落地` 不得当作生产可用能力。E2E 脚本名称只证明有验收覆盖，不代表当前环境已执行通过。

## 当前契约

- `auth-contract.md`：登录、JWT、refresh token、登出和认证审计契约。
- `identity-api-contract.md`：身份、角色、权限、当前用户、统一授权校验、管理员用户预置与 LDAP 导入契约。
- `alert-contract.md`：告警源管理、Webhook ingest、状态流转、AI 分析入口和时间线契约。
- `huawei-ces-alert-sync-contract.md`：华为云 CES 告警拉取、游标、租约、手动同步、可选 worker 和审计契约。
- `runbook-contract.md`：处置预案模板、告警推荐、多步骤执行任务生成契约。
- `execution-contract.md`：告警处置执行任务创建、确认、执行与时间线回写契约。
- `execution-agent-contract.md`：Execution Medium、Agent 控制面、Command Spec、租约和日志回传契约；已实现控制面与规划能力需按章节区分。
- `audit-contract.md`：操作审计检索、Trace 聚合、资源画像与导出契约。
- `ai-contract.md`：AI Provider、工具网关、助手会话、Agent Loop、记忆/知识、分享、多模态、证据快照与 RDS 诊断契约。
- `health-contract.md`：`/healthz`、`/readyz`、`/version` 契约。
- `migration-contract.md`：数据库迁移 runner、版本文件、checksum 和回滚责任边界。
- `cloud-observability-contract.md`：Integration、Observability、Inspection、资产与建议转执行接口契约。
- `huawei-ces-sync-contract.md`：华为云 CES 资源同步范围、同步模式、批次、结构化摘要、计数和对账契约。

全项目串联关系见 `docs/AI运维平台整体流程与调用关系.md`，当前实现状态见根 `README.md` 和 `docs/documentation-review.md`。

## 契约使用方式

```text
开发 / 测试 / 运维
  -> 阅读 ops 契约及章节实现状态
  -> 对照代码、前端和迁移确认当前边界
  -> 前端按契约传参并处理业务拒绝
  -> 后端按契约实现接口、权限和审计
  -> 测试按契约验证结果并记录实际执行结论
  -> 文档、契约和验收脚本随接口变化同步更新
```

## 契约到实现的对应关系

| 契约 | 主要代码与前端 | 迁移/验证证据 | 当前边界 |
| --- | --- | --- | --- |
| `auth-contract.md`、`identity-api-contract.md` | `internal/identity`、`pkg/auth`、`web/src/api/identity*.ts` | `migrations/0001-0006`、`0015-0017`、`0044`；`e2e-identity-access.ps1` | E2E 主要覆盖用户角色主链路；动态菜单和 MFA 未落地 |
| `alert-contract.md` | `internal/alert`、`web/src/api/alert.ts`、`web/src/views/alerts` | `migrations/0007` 及后续 Alert 迁移；`e2e-alert.ps1` | Webhook 与状态流转已落地 |
| `huawei-ces-alert-sync-contract.md` | `internal/alert`、`internal/observability`、告警源页面 | `migrations/0046-0047`；`e2e-alert-sync.ps1` | 手动同步已落地；后台 worker 可选且默认关闭 |
| `runbook-contract.md` | `internal/runbook`、告警详情、`web/src/views/runbooks` | `migrations/0012`；`e2e-runbook.ps1` | 模拟路径生成结构化 Execution Task；前端仅列表/详情/启停，Command Spec 与 Runbook→Agent 展开未落地 |
| `execution-contract.md`、`execution-agent-contract.md` | `internal/execution`、`web/src/api/execution.ts` | `migrations/0011`、`0022`；`e2e-execution*.ps1` | 核心闭环和 Agent 控制面已落地；独立 Agent 进程和专门管理 UI 未提供 |
| `audit-contract.md` | `internal/audit`、`web/src/api/audit.ts`、`web/src/views/audits` | `migrations/0009`；跨模块 E2E 审计断言 | 后端分页查询；前端完成当前查询结果的聚合和 CSV 导出 |
| `ai-contract.md` | `internal/ai`、`cmd/ai-orchestrator`、`web/src/api/ai.ts`、`web/src/views/ai-assistant` | `migrations/0048-0057`、`0059-0062`；`e2e-ai-*.ps1`、`e2e-postgres-history.ps1` | type=b 是助手主路径；type=c 为可选参考服务；真实 RDS 验收需外部告警和云账号 |
| `cloud-observability-contract.md` | `internal/integration`、`internal/observability`、`internal/inspection`、`internal/asset` 及对应前端 | `migrations/0018-0020` 及后续 Asset/拓扑/证据迁移；对应 E2E | fake 与真实 Provider 分开验收；定时巡检 worker、建议转执行前端入口未落地 |
| `huawei-ces-sync-contract.md` | `internal/asset`、`internal/observability/infrastructure/provider/huawei`、资产与接入页 | `migrations/0023-0045`、`0058`；`e2e-asset-sync.ps1`、可选 `e2e-asset-sync-real.ps1` | fake E2E 不等于真实云对账；原生增强、Sync 数据范围、审计生命周期、周期 reaper 和云应用命名仍有差距 |
| `health-contract.md` | `internal/server`、`internal/bootstrap`、`pkg/config` | 后端单元测试和 `/readyz` 抽检 | liveness 与 readiness 语义不能混用 |
| `migration-contract.md` | `pkg/database`、`cmd/migrate`、`cmd/api -migrate`、`migrations` | `go test ./pkg/database/...`、migration readiness | 只能使用自研 runner；当前版本顺序以 `migrations/README.md` 为准 |

## 关键实施约束

- AI 工具调用即使 HTTP 200 且 `code="OK"`，仍必须检查 `data.allowed`；业务拒绝不能被当成调用成功。
- AI、Inspection 或 Recommendation 只允许创建待确认 Execution Task，不能跳过 Execution 状态机。
- Provider 凭据、注册令牌、JWT secret 和数据库密码不得进入接口明文响应、日志、审计或文档示例。
- 真实云验收与 fake/mock 验收必须分开记录；未实际执行的脚本不得写成“已通过”。
- 新增或修改接口、状态机、权限码、迁移和健康检查时，同步更新对应契约、前端 API 说明和验收清单。
