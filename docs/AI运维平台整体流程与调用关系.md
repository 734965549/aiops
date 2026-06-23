# AI 运维平台整体流程同调用关系（粤语版）

呢份文档用嚟做全项目嘅“路线总图”。睇代码或者改接口之前，先用呢张图判断请求会经过边几个上下文、边度做权限、边度写审计、边度只可以读、边度先可以执行真实动作。

## 1. 核心原则

- 平台唔系俾 AI 直接操作生产，而系用 AI 做分析、解释、建议同计划生成。
- 真实变更一定要入 Execution，上状态机、风险等级、人工确认同审计。
- 外部云、监控、日志、链路系统一律经 Provider Adapter 接入，domain 同 application 唔直接绑厂商 SDK。
- 凭据只可以喺 Integration infrastructure 层加密保存引用，唔可以入 Prompt、日志、审计明文、前端持久化状态。
- 所有受保护 API 都要经过 Bearer Token、RBAC、数据范围/工具权限校验；401 同 403 语义要分清。

## 2. 一张图串起成个平台

```mermaid
flowchart LR
    subgraph Client["入口层"]
        UI["Vue 前端<br/>web/src/api/request.ts"]
        Webhook["外部告警 Webhook<br/>Alertmanager / 云告警"]
        AgentUser["AI 助手 / 巡检触发者"]
    end

    subgraph HTTP["HTTP 与鉴权层"]
        Server["cmd/api + internal/server<br/>Trace / Recovery / CORS"]
        Identity["identity<br/>登录 / Token / RBAC"]
    end

    subgraph P0["P0 已落地主闭环"]
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
        ExecAgent["Execution Agent / Medium<br/>确认后先可派发"]
    end

    UI --> Server
    Webhook --> Server
    AgentUser --> Server
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
    Execution --> Audit
    Alert --> Audit
    Runbook --> Audit
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

## 3. 分层调用规矩

每个限界上下文都跟同一条依赖方向：

```text
interfaces/http -> application -> domain <- infrastructure
```

实际含义：

| 层 | 做咩 | 唔应该做咩 |
| --- | --- | --- |
| `interfaces/http` | 绑定 DTO、抽 actor、调 application、用 `httpx.OK/Fail` 输出 | 唔直接操作 GORM、云 SDK、Redis |
| `application` | 编排用例、事务、权限意图、审计 hook、跨上下文 Port | 唔写 Gin 逻辑，唔依赖具体厂商 SDK |
| `domain` | 实体、值对象、状态机、领域错误、仓储接口 | 唔 import infrastructure，唔知道 HTTP |
| `infrastructure` | GORM 仓储、Provider Adapter、Credential Vault、审计 recorder | 唔把明文凭据、原始云错误直接暴露出去 |

跨上下文唔直接穿透到对方 infrastructure。需要调用对方能力时，喺 application 层定义 Port，再喺 infrastructure 做 adapter。

## 4. 当前主要调用链

### 4.1 告警到执行嘅 P0 闭环

```text
外部告警
  -> internal/alert/interfaces/http
  -> alert application 标准化、去重、状态流转
  -> asset application / 匹配规则绑定应用同资源
  -> runbook application 按告警上下文推荐模板
  -> execution application 创建待确认任务
  -> 用户确认后 Execution 状态机推进
  -> Dashboard 聚合结果
  -> Audit 记录关键动作
```

呢条链路系当前最重要主线。任何 Integration、Observability、Inspection、Execution Agent 改动，都唔可以破坏呢条验收链路。

### 4.2 云账号注册同连通性测试

```text
前端 integrations 页面
  -> /api/integrations/accounts
  -> integration/interfaces/http Handler
  -> AccountService
  -> CredentialVault 加密凭据，只保存引用/密文
  -> integration_* 仓储同 UnitOfWork 原子提交
  -> Provider checker 做只读连通性测试
  -> integration_check_result / integration_capability
  -> audit: integration_account create/update/check
```

关键点：

- API 响应只返 `has_credential`，唔返 AK/SK/Token。
- 连通性失败 message 要脱敏。
- `huawei_cloud`、`signoz`、`prometheus` 第一阶段可以由 fake/checker 占位。

### 4.3 统一观测查询

```text
前端 observability 页面 / 巡检服务
  -> /api/observability/metrics/query 或 logs/traces/topology
  -> Observability QueryService
  -> IntegrationAccountPort 解析账号摘要
  -> 校验账号启用状态、provider、capability
  -> ProviderRegistry 按 provider 拿 ProviderEntry
  -> MetricQueryPort / LogSearchPort / TraceQueryPort 等小 Port
  -> obs_evidence_ref 保存证据引用
  -> audit: observability_query
```

关键点：

- Observability 只拿账号摘要同 `credential_ref_id`，唔解密凭据。
- 华为 CES/APM/AOM、SigNoz、Prometheus 只系 adapter，唔进入 domain。
- fake provider 要保持确定性同脱敏，方便前端、巡检、CI 先跑通。

### 4.4 巡检到建议

```text
前端 inspections 页面
  -> 创建 InspectionPolicy
  -> 手动触发 Run
  -> RunService 读取 policy.scope/account_id/checks
  -> EvidenceAnalyzer 透过 ObservabilityQueryPort 收集指标/日志/链路证据
  -> 生成 EvidenceSummary
  -> 规则或 AI 分析产出 Finding
  -> 为 Finding 建 Recommendation
  -> 运行状态 success / partial / failed
  -> audit: inspection_policy / inspection_run / inspection_recommendation
```

关键点：

- 外部数据源部分失败时，应该尽量保留已采集证据，允许 `partial`。
- Finding 要带证据引用，Recommendation 要讲清楚原因、风险、可否转执行。
- AI 分析只可以消费脱敏证据，唔可以拿凭据。

### 4.5 建议转执行同执行介体

```text
Recommendation
  -> RecommendationService
  -> ExecutionCreatorPort
  -> execution application 创建 Execution Task
  -> 风险等级同权限校验
  -> medium/high/critical 进入 pending_confirm
  -> 用户输入确认文本后先 dispatch
  -> Execution Agent / Medium 拉取已确认任务
  -> 日志同结果回传
  -> Execution timeline + Audit
```

关键点：

- Agent 可以提议任务，但唔可以直接 dispatch 或执行。
- 执行命令必须来自 Command Spec，同参数 schema 校验绑定。
- 执行日志要脱敏，stdout/stderr 摘要唔可以包含 Token、密码、AK/SK。

## 5. Provider Port 对应关系

| Application Port | 调用者 | 实现位置 | 对应能力 | 备注 |
| --- | --- | --- | --- | --- |
| `IntegrationAccountPort` | Observability | `internal/observability/infrastructure/integration` | 账号摘要 | 只返 `credential_ref_id`，唔解密 |
| `MetricQueryPort` | Observability QueryService | `internal/observability/infrastructure/provider/*` | `metrics` | CES / Prometheus / SigNoz 都经呢个口 |
| `LogSearchPort` | Observability QueryService | provider adapter | `logs` | 结果摘要要脱敏 |
| `TraceQueryPort` | Observability QueryService | provider adapter | `traces` | Trace/Span 查询 |
| `TopologyQueryPort` | Observability QueryService | provider adapter | `topology` | `topology` 独立于 `traces` |
| `AssetDiscoveryPort` | Asset Sync / Agent 工具 | provider adapter | `assets` | 后续资源同步复用 |
| `AlertRuleQueryPort` | Agent 工具 / Observability | provider adapter | `alerts` | 查询告警规则，唔修改 |
| `ObservabilityQueryPort` | Inspection | `internal/inspection/infrastructure/observability` | 证据收集 | 包装 QueryService |
| `ExecutionCreatorPort` | Inspection / AI | execution adapter | 执行提议 | 只创建待确认任务 |

## 6. 权限、审计同迁移检查点

| 模块 | 关键权限 | 关键审计资源 | 迁移 |
| --- | --- | --- | --- |
| Integration | `app:integrations:read/create/update/delete/check` | `integration_account` | `0018_init_integration.up.sql` |
| Observability | `app:observability:read` | `observability_query` | `0019_init_observability.up.sql` |
| Inspection | `app:inspections:read/write` | `inspection_policy` / `inspection_run` / `inspection_recommendation` | `0020_init_inspection.up.sql` |
| Execution Agent | `app:execution_agents:*`（按契约继续收敛） | `execution_agent` / `execution_medium` | `0022_init_execution_agent.up.sql` |
| P0 主链路 | alert / asset / runbook / execution / audit 各自权限 | 告警、执行、AI 工具调用 | `0007` 到 `0017` |

新增 API 或状态机时，要同步改：

- `ops/*.md` 契约。
- `web/src/api/README.md` 前端调用说明。
- `docs/acceptance-checklist.md` 或对应验收脚本说明。
- migration 权限种子同 admin 角色绑定。

## 7. 维护时点样判断边界有冇走歪

改代码前可以逐项问：

1. Handler 有冇直接碰 GORM、Redis、云 SDK？有就错。
2. application 有冇直接 import 具体华为云/SigNoz SDK？有就错。
3. domain 有冇知道 HTTP、Gin、GORM tag 以外基础设施细节？有就错。
4. Observability 有冇解密凭据？有就错，解密只应该喺具体 provider adapter 所属安全边界内按需处理。
5. AI/Agent 有冇绕过 Execution 直接执行？有就错。
6. 写操作有冇权限、审计、trace_id？冇就要补。
7. API 有冇返回自增 `id` 或明文密钥？有就要改成业务 ID 或脱敏字段。

## 8. 文档入口

- 总体演进：`docs/cloud-observability-agent-roadmap.md`
- 稳定契约：`ops/cloud-observability-contract.md`
- P0 流程图：`docs/AI运维平台核心业务流程图.md`
- 技术架构：`docs/AI运维平台技术架构设计.md`
- 前端 API：`web/src/api/README.md`
