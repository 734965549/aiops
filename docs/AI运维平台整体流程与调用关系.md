# AI 运维平台整体流程与调用关系

这份文档是全项目的路线总图。读代码、改接口或新增模块前，先用这里的图判断请求会经过哪些限界上下文、在哪里做权限校验、在哪里写审计、哪里只能只读查询，以及哪里才允许触发真实执行。

## 1. 核心原则

- 平台不是让 AI 直接操作生产，而是让 AI 做分析、解释、建议和计划生成。
- 真实变更必须进入 Execution 模块，经过状态机、风险等级、人工确认和审计。
- 外部云、监控、日志、链路系统一律通过 Provider Adapter 接入，domain 与 application 不直接绑定厂商 SDK。
- 凭据只允许在 Integration infrastructure 层加密保存引用，不进入 Prompt、日志、审计明文或前端持久化状态。
- 所有受保护 API 都必须经过 Bearer Token、RBAC、数据范围或工具权限校验；401 与 403 语义必须区分清楚。

## 2. 平台总览图

```mermaid
flowchart LR
    subgraph Client["入口层"]
        UI["Vue 前端<br/>web/src/api/request.ts"]
        Webhook["外部告警 Webhook<br/>Alertmanager / 云告警"]
        Operator["运维人员 / AI 助手 / 巡检触发者"]
    end

    subgraph HTTP["HTTP 与鉴权层"]
        Server["cmd/api + internal/server<br/>Trace / Recovery / CORS"]
        Identity["identity<br/>登录 / Token / RBAC"]
    end

    subgraph P0["P0 主闭环"]
        Alert["alert<br/>告警接入 / 状态流转"]
        Asset["asset<br/>资产注册 / 匹配规则"]
        Runbook["runbook<br/>模板 / 推荐 / 多步骤计划"]
        Execution["execution<br/>任务状态机 / 确认 / 执行"]
        Dashboard["dashboard<br/>聚合态势"]
        Audit["audit<br/>审计查询 / 追溯"]
        AI["ai<br/>Provider / 工具网关 / 分析"]
    end

    subgraph Readonly["只读观测扩展"]
        Integration["integration<br/>账号 / 凭据引用 / 能力 / 连通性"]
        Observability["observability<br/>指标 / 日志 / 链路 / 拓扑"]
        Inspection["inspection<br/>策略 / 运行 / Finding / Recommendation"]
    end

    subgraph Infra["基础设施与外部系统"]
        DB["PostgreSQL<br/>业务表 / 迁移"]
        Redis["Redis<br/>缓存 / 可选依赖"]
        Provider["Provider Adapter<br/>Huawei / SigNoz / Prometheus / fake"]
        Credential["Credential Vault<br/>密文或外部 Secret 引用"]
        ExecAgent["Execution Agent / Medium<br/>确认后才能派发"]
    end

    UI --> Server
    Webhook --> Server
    Operator --> Server
    Server --> Identity

    Identity --> Alert
    Identity --> Asset
    Identity --> Runbook
    Identity --> Execution
    Identity --> Dashboard
    Identity --> AI
    Identity --> Integration
    Identity --> Observability
    Identity --> Inspection

    Webhook --> Alert
    Alert --> Asset
    Alert --> Runbook
    Alert --> AI
    Runbook --> Execution
    AI --> Execution
    Execution --> Dashboard
    Alert --> Dashboard
    Asset --> Dashboard
    Runbook --> Dashboard
    Alert --> Audit
    Runbook --> Audit
    Execution --> Audit
    AI --> Audit

    Integration --> Credential
    Integration --> Provider
    Integration --> DB
    Observability --> Integration
    Observability --> Provider
    Observability --> DB
    Inspection --> Observability
    Inspection --> AI
    Inspection --> Execution
    Inspection --> DB
    Inspection --> Audit

    Execution --> ExecAgent
    ExecAgent --> Execution
    Execution --> DB
    Audit --> DB
    Identity --> DB
    Server --> Redis
```

## 3. 启动装配关系

`cmd/api/main.go` 是当前模块化单体的装配入口。它按“仓储 -> 应用服务 -> HTTP Handler -> RouteRegistrar”的顺序把各上下文接入统一 HTTP 引擎。

```mermaid
flowchart TD
    Main["cmd/api/main.go"] --> Bootstrap["bootstrap.Init<br/>配置 / 日志 / DB / Redis"]
    Bootstrap --> Identity["identity 装配<br/>UserRepo / AccessControl / AuthService"]
    Identity --> Audit["audit 装配<br/>OperationAuditService"]
    Audit --> Core["P0 模块装配<br/>alert / asset / runbook / execution / dashboard / ai"]
    Audit --> Observe["观测扩展装配<br/>integration / observability / inspection"]
    Core --> Registrars["RouteRegistrar 列表"]
    Observe --> Registrars
    Registrars --> Server["server.NewEngine"]
    Server --> HTTP["Gin HTTP Server"]
```

## 4. 分层调用规则

每个限界上下文都遵循同一条依赖方向：

```text
interfaces/http -> application -> domain <- infrastructure
```

| 层 | 负责内容 | 禁止事项 |
| --- | --- | --- |
| `interfaces/http` | 绑定 DTO、提取 actor、调用 application、使用 `httpx.OK/Fail` 输出 | 直接操作 GORM、云 SDK、Redis |
| `application` | 编排用例、事务、权限意图、审计 hook、跨上下文 Port | 写 Gin 逻辑，依赖具体厂商 SDK |
| `domain` | 实体、值对象、状态机、领域错误、仓储接口 | import infrastructure，感知 HTTP 或 Gin |
| `infrastructure` | GORM 仓储、Provider Adapter、Credential Vault、审计 recorder | 把明文凭据、原始云错误直接暴露给上层或前端 |

跨上下文调用不要穿透到对方 infrastructure。需要复用能力时，在调用方 application 层定义 Port，再由 infrastructure 提供 adapter。

## 5. 当前主要调用链

### 5.1 告警到执行的 P0 闭环

```text
外部告警
  -> internal/alert/interfaces/http
  -> alert application 标准化、去重、状态流转
  -> asset application / 匹配规则绑定应用和资源
  -> runbook application 按告警上下文推荐模板
  -> execution application 创建待确认任务
  -> 用户确认后 Execution 状态机推进
  -> Dashboard 聚合结果
  -> Audit 记录关键动作
```

这条链路是当前最重要主线。任何 Integration、Observability、Inspection、Execution Agent 改动，都不能破坏这条验收链路。

### 5.2 云账号注册与连通性测试

```text
前端 integrations 页面
  -> /api/integrations/accounts
  -> integration/interfaces/http Handler
  -> AccountService
  -> CredentialVault 加密凭据，只保存引用或密文
  -> integration_* 仓储与 UnitOfWork 原子提交
  -> Provider checker 做字段级连接检查（当前不访问外部服务）
  -> integration_check_result / integration_capability
  -> audit: integration_account create/update/delete/check（事务外 best-effort）
```

关键点：

- API 响应只返回 `has_credential`，不返回 AK/SK/Token；更新留空凭据会保留，删除账号会硬删除密文。
- checker 当前只做字段校验，`status=ok` 不代表网络、云端鉴权或全部 capabilities 实测通过；checker 失败结果落库后 HTTP 仍可成功，必须检查 `data.status`。
- 连通性失败的 `message` 必须脱敏；capabilities 是默认声明，真实凭据账号不能返回伪造的云端数据。

### 5.3 统一观测查询

```text
前端 observability 页面 / 巡检服务
  -> /api/observability/metrics/query 或 logs/traces/topology
  -> Observability QueryService
  -> IntegrationAccountPort 解析账号摘要
  -> 校验账号启用状态、provider、capability
  -> ProviderRegistry 按 provider 获取 ProviderEntry
  -> MetricQueryPort / LogSearchPort / TraceQueryPort 等小 Port
  -> obs_evidence_ref 保存证据引用
  -> audit: observability_query
```

关键点：

- Observability 账号摘要端口只传 `credential_ref_id`，真实华为 Provider 在服务端 infrastructure 边界另行解密，不明文进入 DTO/审计/前端。
- 华为 AK/SK 的指标、资源发现/同步、告警规则和告警拉取已有真实 Adapter；真实 logs/traces/topology unsupported，agency 不降级 fake。SigNoz/Prometheus 当前仍为 fake。
- 普通查询保存脱敏证据摘要，普通 Metrics 另有查询参数但无 payload；AI Metrics 先 collecting，定稿快照与 hash 保存成功后才进入模型，失败不作为支持性引用。
- 当前没有公共证据详情 API/UI；fake/mock 只用于前端、巡检和 CI 联调，不能记为真实云验收。

### 5.4 巡检到建议

```text
前端 inspections 页面
  -> 创建 InspectionPolicy
  -> 手动触发 Run
  -> RunService 读取 policy.scope/account_id/checks
  -> EvidenceAnalyzer 通过 ObservabilityQueryPort 收集指标/日志/链路证据
  -> 生成 EvidenceSummary
  -> 当前由固定阈值规则产出 Finding（不是 AI Agent）
  -> 为 Finding 创建 Recommendation
  -> 同一请求同步返回 success / partial / failed
  -> audit: inspection_policy / inspection_run / inspection_recommendation（best-effort）
```

关键点：

- 外部数据源部分失败时保留已采集证据，允许 `partial`；全部证据失败可返回 HTTP 200 + run.status=failed，需检查业务状态。
- Finding 带证据引用、固定置信度和不确定性；当前 affected_resources 仍是固定示例，schedule/agent_profile 不代表定时或 AI Agent 已落地。
- 采集固定最近 1 小时、cn-north-4、SYS.ECS 且无 dimensions，真实华为指标会失败；fake E2E 与真实云巡检分开验收。

### 5.5 建议转执行与执行介体

```text
Recommendation
  -> RecommendationService
  -> ExecutionCreatorPort
  -> execution application 创建 Execution Task
  -> 风险等级和权限校验
  -> medium/high/critical 进入 pending_confirm
  -> 用户输入确认文本后才能 dispatch
  -> Execution Agent / Medium 拉取已确认任务
  -> 日志和结果回传
  -> Execution timeline + Audit
```

关键点：

- Recommendation 只能创建 agent 模式任务，medium_id 必填，没有自动介体选择；中高风险进入 pending_confirm 后才能由 Agent lease。
- 执行命令必须来自 Command Spec 并绑定参数 schema 校验；未显式传 Spec 时 Inspection 有标题默认值和磁盘兜底，调用方需复核。
- Recommendation 只更新 execution_created，不保存 task_id 或完成回写；任务创建与状态更新非同一事务，重试前需核对 Execution/审计。
- 执行日志要脱敏，stdout/stderr 摘要不能包含 Token、密码、AK/SK。

### 5.6 AI Provider 与工具网关

```text
Provider 管理
  -> ai_provider 数据库为持久化事实源
  -> 当前 API 进程 ProviderRegistry 为调用事实源
  -> 类型 a/b/c Executor；无在线健康检查或跨实例配置广播

直接 /api/ai/tools/invoke
  -> 入口 app:ai.tools:invoke 与数据范围
  -> Gateway.Validate 再查操作/工具权限和 confirmed
  -> 指定 Provider 外部转发

AI 会话 / Agent Loop
  -> Gateway.Validate 只做 RBAC 与工具权限
  -> 8 个内置工具由 Alert/Observability/Integration/Asset Port 本地执行
  -> 对象或账号数据范围在具体 Port 二次校验
  -> 类型 b ChatClient 单独调用模型
```

关键点：

- `api_key` 只保存密文，列表只返回 has_api_key；headers 原样保存和返回，不能当秘密字段。enabled 不等于运行时健康，保存配置也不执行测试连接。
- `alarm.analyze` 在告警分析中交给 Provider，在对话中只读取本地告警快照；同一 tool_code 不代表同一执行路径。
- Provider CRUD 只有应用日志；直接调用审计不记录 allowed/reason，且 allowed=false 时 result 仍可能是 success。对话工具审计才包含完整观测状态。
- 通用 Gateway 不直接创建或执行 Execution Task；执行计划仍从既有 Execution 创建入口进入状态机。

### 5.7 AI 会话与受限 Agent Loop

```text
创建/读取当前用户会话
  -> 发送消息，先按 client_message_id 重放或处理冲突
  -> 用户消息与会话 touch 同事务保存
  -> gather 本地只读观测
  -> 类型 b 模型回答或 tool_requests
  -> 最多 4 轮追加只读工具，逐轮复核权限
  -> 预算/无进展时先强制收敛
  -> finalizeAnswer 生成 actions 并校验 execution.tasks.propose 权限
  -> 助手消息另一事务保存
  -> chat_message 与 tool_invoke 审计
```

关键点：

- 会话详情只返回最近 200 条，模型历史另取最近 12 条；用户与助手消息不是同一事务，失败或取消不会自动删除用户消息。
- 幂等重放不比较请求内容，degraded/incomplete 也会重放；新的提问必须使用新 key。服务端没有会话级串行锁。
- Loop 为 8 个本地只读工具、单轮最多 4 个不同参数申请、80000 Provider token 和默认 600 秒回合预算。强制收敛成功可正常返回，只有仍失败才 agent_loop_exhausted。
- 规则体检的资源分组会先展开实例，并按每条规则最多 3 个实例抽样；曲线定稿最多 288 点，峰值时刻与越限时长在降采样前计算保留。小时级聚合与 Loop 预算仍会限制覆盖，必要时需缩窗复查。
- execution.tasks.propose 在 finalize 阶段只做 action 权限校验并进入 tool_calls/审计，不创建任务；真实任务仍走 Execution 创建接口。
- SSE 在取消检查点前不写 assistant，最终 answer 落库后的推送失败不回滚。同步前端 90 秒封装与默认 600 秒回合预算不一致。

## 6. Provider Port 对应关系

| Application Port | 调用者 | 实现位置 | 对应能力 | 备注 |
| --- | --- | --- | --- | --- |
| `IntegrationAccountPort` | Observability | `internal/observability/infrastructure/integration` | 账号摘要 | 只返回 `credential_ref_id`，不解密 |
| `MetricQueryPort` | Observability QueryService | `internal/observability/infrastructure/provider/*` | `metrics` | CES / Prometheus / SigNoz 都经此端口 |
| `LogSearchPort` | Observability QueryService | provider adapter | `logs` | 结果摘要要脱敏 |
| `TraceQueryPort` | Observability QueryService | provider adapter | `traces` | Trace/Span 查询 |
| `TopologyQueryPort` | Observability QueryService | provider adapter | `topology` | `topology` 独立于 `traces` |
| `AssetDiscoveryPort` | Asset Sync / Agent 工具 | provider adapter | `assets` | 资源同步复用 |
| `AlertRuleQueryPort` | Agent 工具 / Observability | provider adapter | `alerts` | 查询告警规则，不修改 |
| `ObservabilityQueryPort` | Inspection | `internal/inspection/infrastructure/observability` | 证据收集 | 包装 QueryService |
| `ExecutionCreatorPort` | Inspection / AI | execution adapter | 执行提议 | 只创建待确认任务 |

## 7. 权限、审计与迁移检查点

| 模块 | 关键权限 | 关键审计资源 | 迁移 |
| --- | --- | --- | --- |
| Integration | `app:integrations:read/create/update/delete/check` | `integration_account` | `0018_init_integration.up.sql` |
| Observability | `app:observability:read` | `observability_query` | `0019_init_observability.up.sql` |
| Inspection | `app:inspections:read/write` | `inspection_policy` / `inspection_run` / `inspection_recommendation` | `0020_init_inspection.up.sql` |
| Execution Agent | `app:execution_agents:*`（继续按契约收敛） | `execution_agent` / `execution_medium` | `0022_init_execution_agent.up.sql` |
| P0 主链路 | alert / asset / runbook / execution / audit 各自权限 | 告警、执行、AI 工具调用 | `0007` 到 `0017` |

新增 API 或状态机时，必须同步更新：

- `ops/*.md` 契约。
- `web/src/api/README.md` 前端调用说明。
- `docs/acceptance-checklist.md` 或对应验收脚本说明。
- migration 权限种子与 admin 角色绑定。

## 8. 维护检查清单

改代码前逐项检查：

1. Handler 是否直接访问 GORM、Redis、云 SDK？如果是，应下沉到 application/infrastructure。
2. application 是否直接 import 具体华为云、SigNoz SDK？如果是，应改为 Port。
3. domain 是否感知 HTTP、Gin、基础设施细节？如果是，应移出 domain。
4. Observability 是否解密凭据？如果是，应移到具体 provider adapter 的受控边界内。
5. AI/Agent 是否绕过 Execution 直接执行？如果是，应改为创建待确认任务。
6. 写操作是否具备权限、审计、trace_id？缺任一项都要补齐。
7. API 是否返回自增 `id` 或明文密钥？如果是，应改为业务 ID 或脱敏字段。

## 9. 文档入口

- 总体演进：`docs/cloud-observability-agent-roadmap.md`
- 稳定契约：`ops/*.md`
- P0 流程图：`docs/AI运维平台核心业务流程图.md`
- 技术架构：`docs/AI运维平台技术架构设计.md`
- 前端 API：`web/src/api/README.md`
