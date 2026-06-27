# 云厂商只读接管与观测智能体演进设计

## 1. 背景与目标

当前平台已经完成 P0 闭环：

```text
告警接入 -> 资产匹配 -> Runbook 推荐 -> 执行确认 -> Dashboard 汇总 -> 审计追溯
```

下一阶段的目标不是继续扩展单纯 Webhook 告警接入，而是让平台以只读权限接管云厂商和可观测系统的数据面，形成主动观察、持续巡检、链路诊断和优化建议能力。

目标链路：

```text
云账号只读授权
  -> 资源/拓扑/指标/日志/链路/告警同步
  -> 观测智能体周期性巡检
  -> 异常检测与根因候选分析
  -> 生成建议、报告、工单或告警
  -> 高风险动作进入 Execution 确认与审计
```

该能力必须继续遵守项目安全边界：

- AI/Agent 只能分析、解释、建议、生成计划。
- 真实变更动作必须进入 Execution 模块。
- 云厂商凭据只能用于只读数据采集和查询，不能进入 Prompt、日志、审计明文或前端持久化状态。
- 所有 Agent 工具调用必须走 RBAC、数据范围、工具权限和审计。

## 2. 当前能力与目标能力边界

| 能力 | 当前状态 | 目标状态 |
| --- | --- | --- |
| 告警接入 | 已支持 Alertmanager 和通用 Webhook；接入源类型包含 `huawei_ces`、`signoz` 等枚举 | 保留 Webhook 作为事件入口，同时支持云厂商告警规则、历史告警和事件列表只读拉取 |
| 资产管理 | 手工注册应用/资源，可通过匹配规则关联告警 | 支持从云账号、Kubernetes、CMDB、Signoz 自动发现和增量同步资源 |
| 指标分析 | `include_metrics` 只是 AI 分析请求参数，尚未拉取真实指标 | 通过 Provider Adapter 查询 CES、AOM、Prometheus、Signoz 指标，并保存巡检证据 |
| 日志/链路 | 规划中，未形成统一查询端口 | 通过统一 Observability Port 查询日志、Trace、Span、服务拓扑 |
| AI 分析 | 支持针对单条告警调用 `alarm.analyze`，失败时本地摘要兜底 | 支持 Agent 按巡检计划调用多类工具，生成带证据的建议、报告和工单 |
| 通知发送 | 未形成独立 Notification 模块 | 建立通知通道，支持企业微信、飞书、邮件、Slack/Webhook 等发送建议 |

## 3. DDD 限界上下文设计

| 上下文 | 目录建议 | 职责 | 依赖方向 |
| --- | --- | --- | --- |
| Integration | `internal/integration` | 云账号、可观测系统账号、凭据引用、Provider 配置、连通性测试 | interfaces -> application -> domain <- infrastructure |
| Asset Sync | `internal/asset` 扩展或 `internal/assetsync` | 资源发现、资源快照、拓扑关系同步；优先复用 Asset 应用/资源注册表 | application 依赖 Integration 端口，不直接依赖具体云 SDK |
| Observability | `internal/observability` | 统一指标、日志、链路、拓扑查询抽象；屏蔽 Huawei CES/AOM/APM、Signoz、Prometheus 差异 | domain 定义查询模型，infrastructure 实现各 Provider |
| Inspection | `internal/inspection` | 巡检计划、巡检任务、巡检结果、证据包、建议状态机 | application 编排 Observability/Asset/AI/Notification 端口 |
| Agent | `internal/ai` 扩展或 `internal/agent` | Agent 策略、工具编排、MCP 工具注册、分析上下文构造 | 只通过工具网关访问受控端口 |
| Notification | `internal/notification` | 通知通道、模板、发送记录、重试、脱敏 | 不直接耦合巡检业务，接收 application 层命令 |
| Execution Mediation | `internal/execution` 扩展 | 执行介体、执行代理、命令规格、任务分发、日志回传 | 只接受已确认 Execution Task，不接受 Agent 直接执行 |

第一阶段保持模块化单体，不拆独立服务。跨上下文调用使用 application 层端口接口；禁止 Handler 或 Agent 直接操作云 SDK、GORM、Redis。

目录形态：

```text
internal/
  integration/
    domain/
    application/
    infrastructure/
      huawei/
      signoz/
      credential/
    interfaces/http/
  observability/
    domain/
    application/
    infrastructure/
      huawei_ces/
      huawei_aom/
      huawei_apm/
      signoz/
      prometheus/
    interfaces/http/
  inspection/
    domain/
    application/
    infrastructure/
    interfaces/http/
    worker/
  notification/
    domain/
    application/
    infrastructure/
    interfaces/http/
  execution/
    domain/
      medium.go
      agent.go
      command_spec.go
    application/
      dispatch_service.go
      command_policy.go
    interfaces/http/
      agent_handler.go
```

## 4. 核心领域对象

### 4.1 Integration

| 对象 | 说明 |
| --- | --- |
| `IntegrationAccount` | 外部账号接入配置，包含 `account_id`、`provider`、`auth_type`、`regions`、`enabled`、`owner_team` |
| `CredentialRef` | 凭据引用，只保存加密后的密文或外部 Secret 引用，不向 API 返回明文 |
| `ProviderCapability` | Provider 支持能力，如 `resources`、`metrics`、`logs`、`traces`、`alerts`、`topology` |
| `ConnectivityCheck` | 连通性测试结果，包含状态、错误码、脱敏错误摘要、检查时间 |

### 4.2 Observability

| 对象 | 说明 |
| --- | --- |
| `MetricQuery` | 指标查询条件：provider、account、region、namespace、metric、dimensions、time_range、period |
| `MetricSeries` | 指标时间序列，必须携带单位、聚合方式、采样粒度和来源 |
| `LogQuery` | 日志查询条件：service、resource、keyword、trace_id、time_range、limit |
| `TraceQuery` | 链路查询条件：service、operation、trace_id、latency、error_only、time_range |
| `TopologySnapshot` | 服务、资源、调用关系、依赖关系快照 |
| `EvidenceRef` | 巡检证据引用，记录来源、查询参数 hash、采样窗口、摘要 |

### 4.3 Inspection

| 对象 | 说明 |
| --- | --- |
| `InspectionPolicy` | 巡检策略：目标范围、频率、数据源、检查项、Agent 策略 |
| `InspectionRun` | 一次巡检执行记录：状态、触发方式、开始/结束时间、结果摘要 |
| `InspectionFinding` | 巡检发现：风险等级、影响范围、证据、建议、置信度 |
| `Recommendation` | 可追踪建议：标题、原因、建议动作、风险、是否可生成 Execution |
| `AgentRun` | Agent 工具调用过程：工具列表、输入摘要、输出摘要、token/耗时、审计引用 |

### 4.4 Execution Mediation

| 对象 | 说明 |
| --- | --- |
| `ExecutionMedium` | 执行介体，例如跳板机、诊断 VM、目标机器、K8s 诊断 Pod、云厂商受控命令通道 |
| `ExecutionAgent` | 部署在介体或网络域内的执行代理，主动拉取已确认任务并回传日志 |
| `CommandSpec` | 命令规格，包含命令模板、参数 schema、风险、超时、输出脱敏规则 |
| `ExecutionLease` | 执行租约，防止多个代理重复执行同一任务 |
| `ExecutionLogStream` | 执行日志流，记录 stdout/stderr 摘要、序号、脱敏状态和证据引用 |

## 5. Provider Adapter 与 MCP 工具边界

Agent 不直接拿云 AK/SK 调接口，而是通过受控工具调用。工具由平台后端注册、鉴权、审计、限流，再由 Provider Adapter 执行只读查询。

工具命名建议：

```text
cloud.accounts.list
cloud.resources.list
cloud.resources.get
cloud.topology.get
cloud.metrics.query
cloud.logs.search
cloud.traces.query
cloud.alerts.list
cloud.events.list
inspection.policies.list
inspection.runs.create
notification.messages.send
execution.tasks.propose
execution.tasks.dispatch
execution.media.list
```

厂商专属能力放在 Adapter 层，不直接暴露给业务流程：

```text
huawei.ces.query_metrics       -> cloud.metrics.query
huawei.aom.query_logs          -> cloud.logs.search
huawei.apm.query_traces        -> cloud.traces.query
signoz.query_metrics           -> cloud.metrics.query
signoz.query_traces            -> cloud.traces.query
```

工具调用控制：

| 控制点 | 要求 |
| --- | --- |
| 身份 | 工具调用继承当前用户或系统巡检主体，不能使用无归属的超级身份 |
| 权限 | RBAC + 数据范围 + AI 工具权限三层校验 |
| 只读 | 云厂商 Adapter 第一阶段只实现 GET/List/Query，不实现写操作 |
| 审计 | 记录工具名、调用者、资源范围、参数摘要、结果摘要、耗时和 trace_id |
| 脱敏 | AK/SK、Token、Authorization、原始日志敏感字段不得进入 Prompt 和审计明文 |
| 限流 | 按账号、区域、工具、用户设置限流，避免打爆云 API |

### 5.3 执行类工具边界

Agent 可以提出执行计划，但不能直接执行。执行类工具必须拆成两类：

| 工具 | 能力 | 是否允许 Agent 直接调用 |
| --- | --- | --- |
| `execution.tasks.propose` | 根据建议生成待确认 Execution Task | 允许，但必须进入待确认状态 |
| `execution.media.list` | 查询可选执行介体和健康状态 | 允许，只读 |
| `execution.tasks.dispatch` | 将已确认任务分发给执行代理 | 不允许 Agent 直接调用，只能由 Execution application 层状态机触发 |

执行命令必须匹配平台预置 Command Spec，AI 只能填充参数，不能提交未受控的自由 shell 字符串。

## 6. 华为云接入设计

### 6.1 只读 IAM 权限建议

华为云接入应使用独立 IAM 用户或委托，按最小权限授权：

| 能力 | 权限范围 |
| --- | --- |
| 资源发现 | ECS、CCE、ELB、RDS、VPC 等资源只读 |
| 指标查询 | CES 指标、告警规则、告警历史只读 |
| 日志查询 | AOM/LTS 日志只读 |
| 链路查询 | APM 应用、调用链、事务、拓扑只读 |
| 成本分析 | 预算/账单只读，若纳入成本优化 |

禁止授权：

- 云资源创建、删除、重启、扩缩容。
- 安全组、路由、数据库、密钥、IAM 权限变更。
- 任何生产写操作。

### 6.2 Huawei Provider Adapter

```text
IntegrationAccount(huawei_cloud)
  -> HuaweiCredentialProvider
  -> HuaweiResourceAdapter
  -> HuaweiCESAdapter
  -> HuaweiAOMAdapter
  -> HuaweiAPMAdapter
```

Adapter 职责：

- 统一处理认证、region/project_id、重试、限流、错误映射。
- 将华为云原始字段转换为平台领域对象。
- 只返回业务 ID、资源名、标签、指标点、日志摘要、Trace 摘要。
- 底层错误写统一日志，对外返回平台错误码。

## 7. 主流程设计

### 7.1 云账号接入

```text
管理员创建 IntegrationAccount
  -> 保存凭据引用或密文
  -> 连通性测试
  -> 读取 ProviderCapability
  -> 写审计 integration_account_create
```

### 7.2 资源同步

```text
AssetSync Worker
  -> 读取启用的 IntegrationAccount
  -> 调用 Provider Adapter list resources
  -> 归一化为资产快照
  -> upsert 应用/资源/拓扑关系
  -> 记录同步批次和差异
```

同步策略：

- 增量同步优先，无法增量时按 region 分批全量扫描。
- 云资源业务 ID 使用云厂商稳定 ID，如 instance_id，不暴露 DB 自增 id。
- 自动同步资源标记 `source=cloud_sync`，手工注册资源标记 `source=manual`。
- 删除策略第一阶段使用 `stale` 标记，不直接删除历史资源。

### 7.3 指标/日志/链路查询

```text
InspectionService
  -> 根据资源和策略构造查询
  -> ObservabilityService 查询 metrics/logs/traces
  -> 生成 EvidenceRef
  -> 传递脱敏摘要给 AI Agent
```

### 7.4 观测智能体巡检

```text
InspectionScheduler
  -> 创建 InspectionRun
  -> 加载目标资源和策略
  -> 查询指标、日志、链路、历史告警、变更
  -> Agent 分析瓶颈和故障候选
  -> 写 InspectionFinding / Recommendation
  -> 必要时创建平台告警或通知
  -> 写审计和 Dashboard 汇总
```

### 7.5 建议到执行

AI 只能生成建议或执行计划：

```text
Recommendation
  -> 用户选择或确认执行介体
  -> 用户确认生成 Execution Task
  -> Execution 风险等级判定
  -> pending_confirm
  -> 人工 CONFIRM
  -> pending_execute
  -> Execution 分发给指定介体上的执行代理
  -> 执行代理回传日志和结果
  -> 时间线、审计、Recommendation 状态回写
```

不得出现：

```text
Agent -> 云厂商写 API
Agent -> SSH/K8s/DB 直接执行
前端确认弹窗 -> 绕过 Execution 状态机
AI 生成自由命令字符串 -> 执行代理直接运行
```

### 7.6 执行介体选择

介体选择必须可解释：

```text
Recommendation
  -> medium_selector(environment/region/network_zone/capabilities)
  -> 查询可用 ExecutionMedium
  -> 运维人员在确认页查看并确认介体
  -> Execution Task 绑定 medium_id
  -> Agent 根据 medium_id 和能力领取任务
```

执行确认页必须展示：

- 介体名称、类型、环境、网络域、健康状态。
- 自动匹配原因。
- Command Spec 名称、参数、风险和超时。
- 命令是否只读、是否会修改目标系统。
- 日志脱敏和失败接管方式。

## 8. 数据库与迁移规划

迁移版本从当前最新版本后递增，禁止插队或改写已发布迁移。

| 阶段 | 迁移建议 | 表前缀 | 说明 |
| --- | --- | --- | --- |
| 云账号接入 | `0018_init_integration.up.sql` | `integration_*` | 账号、凭据引用、能力、连通性检查 |
| 观测查询 | `0019_init_observability.up.sql` | `obs_*` | 查询记录、证据引用、拓扑快照 |
| 巡检策略 | `0020_init_inspection.up.sql` | `inspection_*` | 策略、运行、发现、建议 |
| 通知 | `0021_init_notification.up.sql` | `notification_*` | 通道、模板、发送记录 |
| 执行介体 | `0022_init_execution_agent.up.sql` | `exec_*` | 介体、代理、命令规格、租约、日志流 |
| 权限种子 | 同阶段迁移内同步 | `iam_*` | 新增 `app:integrations:*`、`app:observability:*`、`app:inspections:*`、`app:notifications:*` |

所有业务表必须遵守项目数据库硬约束：

- `id BIGSERIAL` 自增主键。
- 独立业务 ID，如 `account_id`、`policy_id`、`run_id`、`finding_id`。
- `created_at`、`updated_at` 由 Go 程序维护，不使用 DB DEFAULT。
- 不随意添加数据库外键，使用应用层事务和 repository 约束保证一致性。

## 9. API 与前端页面规划

详细 API 草案见 `ops/cloud-observability-contract.md`。

全项目串联图见 `docs/AI运维平台整体流程与调用关系.md`。该文档将 P0 告警闭环、Integration 账号接入、Observability 查询、Inspection 巡检、Recommendation 转 Execution、Execution Agent 派发关系放在同一张图中，方便评审调用边界。

| 页面 | 路由建议 | 说明 |
| --- | --- | --- |
| 云账号接入 | `/integrations/accounts` | 云厂商、Signoz、Prometheus 等账号配置和连通性检查 |
| 观测数据源 | `/observability/sources` | 数据源能力、健康状态、最近同步 |
| 巡检策略 | `/inspections/policies` | 巡检范围、频率、检查项、通知策略 |
| 巡检结果 | `/inspections/runs` | 运行历史、发现、证据、AI 建议 |
| 建议中心 | `/inspections/recommendations` | 可采纳建议、风险、生成执行入口 |
| 通知通道 | `/notifications/channels` | 企业微信、飞书、邮件、Webhook 等 |
| 执行介体 | `/executions/media` | 跳板机、诊断 VM、目标机代理、K8s 诊断 Pod 管理 |
| 命令规格 | `/executions/command-specs` | 受控命令模板、参数 schema、风险、超时、脱敏规则 |

与现有页面关系：

- Dashboard 增加巡检健康、今日发现、待处理建议、接入账号健康。
- 资源与应用增加来源、云资源 ID、同步状态、拓扑入口。
- 告警详情增加巡检证据和相关 Trace/Log/Metric 引用。
- AI 运维助手增加可引用的数据源范围和工具调用审计。
- 审计中心增加 Integration、Observability、Inspection、Notification 资源类型筛选。
- 执行确认页增加介体选择、命令规格和代理健康状态。

## 10. 里程碑步骤

### 阶段 1：接入底座与安全边界

交付：

- `internal/integration` 上下文。
- 云账号接入 API、只读凭据保存、连通性测试。
- 权限码和审计动作。
- 前端账号接入页面。

验收：

- 新建/编辑/禁用/删除账号均写审计。
- 响应不返回明文密钥。
- 无权限用户访问返回 403。
- 连通性测试失败时不暴露底层敏感错误。

### 阶段 1.5：Provider Port 与 fake Observability（当前优先）

在真实华为云 SDK 接入前，先把 application 层 Provider Port 定下来，并用 fake adapter 跑通查询链路：

交付：

- `internal/observability` 上下文。
- Application 层 Port：`MetricQueryPort`、`LogSearchPort`、`TraceQueryPort`、`TopologyQueryPort`、`AssetDiscoveryPort`、`AlertRuleQueryPort`。
- `ObservabilityProvider` + `ProviderRegistry`；infra 第一阶段为 `FakeProvider`（`signoz` / `prometheus` 全 fake；`huawei_cloud` 在 `auth_type=none` 时全 fake）。
- HTTP API：`/api/observability/metrics/query`、`/logs/search`、`/traces/query`、`/topology`。
- 权限 `app:observability:read`、审计 `observability_query`、证据表 `obs_evidence_ref`（迁移 `0019`）。
- `IntegrationAccountPort` 适配器复用 Integration 账号与凭据引用（`credential_ref_id`），业务层不直接依赖 GORM/云 SDK。
- `cmd/api/main.go` 装配：`huaweiobs.NewCredentialProvider(integCredentialRepo, integVault)` + `obsprovider.DefaultRegistry(huaweiObsCreds)`。

验收：

- 无真实云账号时，fake provider 可返回确定性样本数据。
- 查询写审计且生成 `evidence_id`；凭据与原始敏感日志不进响应/审计明文。
- 无 `app:observability:read` 返回 403。
- `AssetDiscoveryPort` / `AlertRuleQueryPort` 在 application 层可调用（供后续 Asset Sync 与 Agent 工具注册），HTTP 可在阶段 2 暴露。

后续替换路径：`infrastructure/provider/huawei_ces` 等实现同一组 Port，注册到 `ProviderRegistry`，不影响 application 与 HTTP 契约。

**阶段 3 指标里程碑（已落地）**：`huawei_cloud` + `auth_type=ak_sk` 的 `QueryMetrics` 已走真实 CES；前端可创建带 `project_id` 的华为账号；连通性检查为字段校验；同账号的 logs/traces/topology/assets/alerts 对真实凭据返回 unsupported，对 `auth_type=none` 仍为 fake。

**本阶段验收（华为 CES 指标）**：

- 前端 `/integrations` 可创建 `huawei_cloud` + `ak_sk` + `region` + `project_id` 账号。
- `/api/integrations/accounts/:account_id/check` 对 `ak_sk` 做字段级校验（凭据非空、regions 非空）；真实 IAM/CES 探活留待后续。
- `/api/observability/metrics/query` 用该账号查询 `SYS.ECS` / `cpu_util` 等 CES 指标时返回真实 CES 数据点（非 fake）；响应含标准 `MetricSeries`、`evidence_id`，写审计。
- 响应/日志/审计不含 AK/SK、`Authorization`、原始敏感报错。
- 单测覆盖：`metric_mapper`、`credential` 缺失/错误、`CESClient` mock、`QueryService` 经真实 `huawei.Adapter` 集成路径。

### 阶段 2：资源同步与拓扑（已落地）

交付：

- Huawei 资源只读 Adapter（ECS/CCE/RDS/ELB `ListResources`，`auth_type=ak_sk`）。
- 资源同步批次、`stale` 标记（迁移 `0023`）。
- Asset 模块扩展同步来源字段 + `POST /api/assets/sync`。
- 前端 `/assets` 与 `/integrations` 展示来源、云资源 ID、同步批次。

补充目标：

- 华为云资产同步的口径为**指定 CES 资源分组**下资源（默认候选名“全部资源”需用户在 CES 控制台预先创建；CES 官方 API 只返回用户创建的资源分组，不存在“总览全量”隐式口径，未命中指定组即失败，不回退最大资源组）。
- 产品分层采用 `ces` / `hybrid` / `native` 三种同步模式：`ces` 为 P0/P1 默认模式，同步范围为指定资源分组下资源；`hybrid` 为 P2 增强模式，在指定资源分组发现后补充原生云服务详情；`native` 仅兼容旧 ECS/CCE/RDS/ELB 路径。
- 当前 ECS/CCE/RDS/ELB 原生 API 同步只作为阶段 2 已落地能力、`hybrid` 增强路径或 `native` 兼容路径，不再作为完整性判断口径。
- CES 资源分组同步的实现顺序、权限、分页、资源映射、stale 语义和验收标准见 `docs/huawei-ces-asset-sync-plan.md`。

验收：

- `sync_mode=ces` 时，仅授予 CES 只读权限的华为云账号即可按 CES 控制台“全部资源”口径同步到资产注册表。
- `sync_mode=hybrid` 时，先保证 CES 资源完整入库，再按已授权的 ECS/RDS/ELB/EVS/VPC/OBS 等原生 API 补充详情；增强失败不影响基础资源入库。
- `sync_mode=native` 保留旧 ECS/CCE/RDS/ELB 同步路径，但不承诺与 CES 控制台数量一致。
- 同步失败不影响已有 P0 告警闭环。
- 删除云端资源时平台标记 stale，不直接删除历史数据。
- `scripts/e2e-asset-sync.ps1` 在 CI 可跑（fake provider）。

### 阶段 3：指标/日志/链路统一查询

交付：

- `internal/observability` 上下文（**阶段 1.5 已落地 Port + fake provider + HTTP + 审计**）。
- Huawei CES 指标查询（**已落地真实 metrics**：`auth_type=ak_sk` + `CredentialProvider` → CES `ShowMetricData`；`auth_type=none` 仍为 fake）。
- Huawei AOM/LTS 日志查询（**待替换** fake `LogSearchPort`）。
- Huawei APM 和 Signoz Trace 查询（**待替换** fake `TraceQueryPort`）。
- EvidenceRef 证据引用（`0019` 已建 `obs_evidence_ref` 表）。

**当前边界**：真实凭据的 `huawei_cloud` 账号调用 logs/traces/topology/assets/alerts 时返回 `capability unsupported`，不会返回 fake 样本；`signoz`/`prometheus` 仍为全 fake。

验收：

- 同一资源可查询最近 30m/1h/6h 指标。
- 查询参数和结果摘要写审计。
- 查询结果可被巡检任务引用。

### 阶段 4：巡检策略与智能体分析

交付：

- `internal/inspection` 上下文。
- 手动触发巡检 API。
- 定时巡检 Worker。
- Agent 工具编排，生成 Finding 和 Recommendation。

验收：

- 巡检运行有状态机和时间线。
- AI 建议包含证据来源、风险等级、置信度和不确定性说明。
- 工具调用失败时保留部分结果，状态为 degraded 或 partial。

### 阶段 5：通知与执行闭环

交付：

- `internal/notification` 上下文。
- 通知通道和模板。
- 建议发送到外部 IM/邮件/Webhook。
- 从 Recommendation 创建 Execution Task，并选择执行介体。
- `ops/execution-agent-contract.md` 中定义的介体、代理、命令规格和租约模型。

验收：

- 通知发送有重试、失败记录和审计。
- 中高风险建议只生成待确认执行任务。
- Execution 完成后回写 Recommendation 状态。
- 禁用介体、离线代理、未确认任务均不能执行。
- AI 生成的命令必须匹配 Command Spec，参数 schema 不通过时拒绝创建任务。

### 阶段 5.1：执行介体与诊断命令

交付：

- `internal/execution` 扩展 ExecutionMedium、ExecutionAgent、CommandSpec、ExecutionLease。
- fake agent 或本地 agent，用于开发环境验证领取任务、日志回传、结果回传。
- 第一批只读诊断 Command Spec：磁盘、CPU、内存、网络连通性、服务状态、日志采样。

验收：

- 运维人员能在执行确认页选择 jumpbox 或 target_host 介体。
- `pending_confirm` 任务不能被代理领取。
- `pending_execute` 任务被单个代理租约领取，重复领取被拒绝。
- stdout/stderr 实时回传，服务端二次脱敏。
- 执行失败时保留日志、exit_code、错误摘要和人工接管提示。

### 阶段 6：多云与可观测平台扩展

交付：

- 阿里云、腾讯云、AWS 或 Kubernetes Adapter。
- Signoz 深度接入。
- 成本优化、容量预测、SLO 巡检策略。

验收：

- Provider Adapter 可插拔。
- 新 Provider 不影响已有华为云链路。
- 权限和审计模型不因 Provider 增加而分叉。

## 11. 验证策略

| 范围 | 验证 |
| --- | --- |
| domain | 状态机、枚举、只读能力、风险等级、建议状态必须有单元测试 |
| application | 账号接入、同步批次、巡检运行、审计 hook、幂等必须有测试 |
| infrastructure | Adapter 使用 fake server 或 mock SDK，覆盖限流、认证失败、超时 |
| interfaces/http | 成功、401、403、参数错误、凭据不回显 |
| worker | 可重复执行、失败重试、部分失败降级、上下文取消 |
| frontend | 表单脱敏、403 不白屏、巡检运行状态、证据抽屉 |
| E2E | 新增 `e2e-integration.ps1`、`e2e-inspection.ps1` |
| execution agent | fake agent 覆盖介体选择、租约、日志、结果、幂等和脱敏 |

## 12. 文档同步要求

实现任一阶段时，需要同步更新：

- `ops/cloud-observability-contract.md`
- `ops/execution-agent-contract.md`
- `ops/ai-contract.md`
- `ops/identity-api-contract.md`
- `ops/migration-contract.md`
- `docs/acceptance-checklist.md`
- `docs/release-checklist.md`
- `web/src/api/README.md`

涉及接口、状态机、权限码、迁移、健康检查和审计字段时，必须先更新契约再改代码。
