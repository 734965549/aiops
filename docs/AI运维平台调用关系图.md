# AI 运维平台调用关系图

本文档用 Mermaid 图补充说明平台当前已落地的主要调用关系。更完整的边界说明见 `docs/AI运维平台整体流程与调用关系.md`。

## 1. 后端运行时调用总图

```mermaid
flowchart LR
    Client["前端 / Webhook / Agent"] --> Server["internal/server<br/>Trace / Auth / RBAC / Recovery"]
    Server --> Handler["interfaces/http<br/>Handler / DTO / httpx.OK/Fail"]
    Handler --> App["application<br/>用例编排 / 事务 / 审计 hook / Port"]
    App --> Domain["domain<br/>实体 / 状态机 / Repository 接口"]
    App --> Port["跨上下文 Port"]
    Domain --> RepoContract["Repository 接口"]

    RepoContract --> InfraRepo["infrastructure/persistence<br/>GORM Repository"]
    Port --> Adapter["infrastructure adapter<br/>Alert / Asset / Runbook / Execution / Provider"]

    InfraRepo --> DB["PostgreSQL"]
    Handler --> Authz["identity AuthorizationService"]
    Authz --> AccessRepo["iam_* 权限表 / Redis 授权缓存"]
    Adapter --> External["外部系统<br/>Provider / Agent / Redis"]
    App --> Audit["audit Recorder"]
    Audit --> AuditDB["audit_operation"]
```

## 2. P0 告警到执行闭环

```mermaid
sequenceDiagram
    participant AM as Alertmanager/Webhook
    participant AH as Alert HTTP
    participant AS as Alert IngestService
    participant Asset as Asset Matcher
    participant AR as Alert Repo
    participant UI as Web UI
    participant RB as Runbook Service
    participant EX as Execution Service
    participant AU as Audit Service
    participant DB as PostgreSQL

    AM->>AH: POST /api/alerts/webhooks/:source_id
    AH->>AS: 标准化 / 鉴权 / 去重
    AS->>Asset: 按 labels 匹配应用与资源
    Asset-->>AS: application_id / resource_id
    AS->>AR: 创建或更新告警与时间线
    AS->>AU: 记录 ingest 审计
    AR->>DB: 写 alert / alert_event

    UI->>AH: 查看告警详情 / 认领 / AI 分析
    UI->>RB: GET /api/runbooks/recommendations?alert_id=
    RB-->>UI: 推荐 Runbook 模板与步骤
    UI->>EX: POST /api/executions/tasks
    EX->>AU: 创建执行任务审计
    EX-->>UI: pending_confirm / pending_execute
    UI->>EX: POST /confirm, confirm_text=CONFIRM
    EX->>AU: 确认审计
    UI->>EX: POST /execute
    EX->>AR: 回写 execution_started / execution_finished
    EX->>AU: 执行结果审计
```

## 3. AI Provider、工具调用与执行边界

### 3.1 直接工具调试：授权后转发给指定 Provider

```mermaid
sequenceDiagram
    participant UI as 工具调用调试页
    participant AIH as AI HTTP Handler
    participant GW as Tool Gateway
    participant Authz as AuthorizationService
    participant Provider as 类型 a/b/c Provider Executor
    participant AU as Audit Service

    UI->>AIH: POST /api/ai/tools/invoke
    Note over AIH: 入口先校验 app:ai.tools:invoke 与路由数据范围
    AIH->>GW: Invoke(provider_id, tool_code, resource, action, confirmed)
    GW->>Authz: 可选操作权限 + 对象范围 + AI 工具权限 + confirmed
    alt allowed=false
        GW-->>AIH: allowed=false, reason
        AIH->>AU: best-effort tool_invoke（当前仍记 result=success）
        AIH-->>UI: HTTP 200 + OK，展示业务拒绝原因
    else allowed=true
        GW->>Provider: 调用请求指定的 Provider，不自动切换
        Provider-->>GW: Executor data；类型 b 也可能 degraded=true
        GW-->>AIH: allowed=true, provider, data
        AIH->>AU: best-effort tool_invoke
        AIH-->>UI: 返回结果
    end
```

Gateway 自身不创建 Execution Task。`execution.tasks.propose` 没有平台内置工具实现，直接调用只能转发给外部 Provider；这不能替代 Execution 状态机。

### 3.2 对话内置只读工具：本地执行，模型单独调用

```mermaid
sequenceDiagram
    participant Chat as ChatService / Agent Loop
    participant GW as Tool Gateway
    participant Authz as AuthorizationService
    participant Port as Alert/Observability/Integration/Asset Port
    participant Model as 类型 b ChatClient
    participant AU as Audit Service

    Chat->>GW: Validate(app:ai:chat + tool_code, SkipDataScope=true)
    GW->>Authz: RBAC + AI 工具权限
    GW-->>Chat: allowed / mode / reason
    alt 工具权限通过
        Chat->>Port: 执行 8 个内置只读工具之一
        Note over Port: 在对象或账号 Port 二次校验数据范围
        Port-->>Chat: 只读观测 / 证据 / 稳定失败状态
    end
    Chat->>AU: 写详细 tool_invoke
    Chat->>Model: 携带观测做对话补全
    Model-->>Chat: answer 或 tool_requests
    Chat->>AU: 写 chat_message
```

告警分析是另一条路径：AnalyzeService 读取告警和证据后，通过 `Gateway.Invoke` 把 `alarm.analyze` 交给动态选择的 Provider。对话中的同名工具只是在本地读取告警快照，不能混为同一外部调用。

AI 回答中的“生成执行计划”由前端调用既有 Execution Task 创建接口；任务仍须经过 Execution 权限、风险、确认和审计，不能由上述工具图推断 Gateway 会直接执行。

### 3.3 会话消息、幂等与 Loop 收敛

```mermaid
sequenceDiagram
    participant UI as AI 助手工作台
    participant Chat as ChatService
    participant Repo as SessionRepository
    participant Loop as Agent Loop
    participant Final as finalizeAnswer
    participant AU as Audit Service

    UI->>Chat: 同步或 SSE 发送 content + client_message_id
    Chat->>Repo: 校验会话归属并查幂等键
    alt 已有 assistant
        Repo-->>Chat: 首次回答
        Chat-->>UI: 原样重放，不重新取数/调模型
    else 只有 user，没有 assistant
        Chat-->>UI: 409 / SSE error(FAILED_PRECONDITION)
    else 新提问
        Chat->>Repo: 事务一：user + title/context/updated_at
        Chat->>Loop: gather + 类型 b 模型 + 最多 4 轮只读工具
        Loop-->>Chat: answer，或预算/无进展后强制收敛
        Chat->>Final: 过滤证据、生成 actions、校验执行提议权限
        Final-->>Chat: execution.propose action + 权限 tool_call（不建任务）
        Chat->>Repo: 事务二：assistant + updated_at
        Chat->>AU: chat_message / tool_invoke（best-effort）
        Chat-->>UI: answer（落库后的最终推送失败不回滚）
    end
```

用户和助手消息不是同一事务；取消检查点之前已保存的用户消息会保留。Loop 的强制收敛成功可以正常回答，只有仍失败才进入 agent_loop_exhausted。

## 4. 只读观测与巡检链路

```mermaid
sequenceDiagram
    participant UI as Integrations/Observability/Inspections 页面
    participant IN as Integration Service
    participant Vault as CredentialVault
    participant OBS as Observability QueryService
    participant Provider as Provider Adapter
    participant IR as Inspection RunService
    participant EA as EvidenceAnalyzer
    participant EX as ExecutionCreatorPort
    participant AU as Audit Service

    UI->>IN: 创建接入账号
    IN->>Vault: 加密凭据，只保存引用
    IN->>AU: integration_account 审计

    UI->>OBS: 查询 metrics/logs/traces/topology
    OBS->>IN: IntegrationAccountPort 获取账号摘要
    OBS->>Provider: 只读查询，按 capability 路由
    Provider-->>OBS: 脱敏结果
    OBS->>AU: observability_query 审计
    OBS-->>UI: 数据 + evidence_id

    UI->>IR: 触发巡检 Run
    IR->>OBS: ObservabilityQueryPort 收集证据
    IR->>EA: 规则分析生成 Finding
    EA-->>IR: Recommendation
    IR->>AU: inspection_run / recommendation 审计
    opt 建议转执行
        IR->>EX: 创建 Execution Task
        EX-->>IR: task_id, pending_confirm
    end
```

## 5. Execution Agent 派发链路

```mermaid
sequenceDiagram
    participant UI as 执行页面
    participant EX as Execution Service
    participant CS as Command Spec Repo
    participant Lease as Lease Repo
    participant Agent as Execution Agent
    participant Log as Log Stream Repo
    participant AU as Audit Service

    UI->>EX: 创建任务 / 选择 medium / command spec
    EX->>CS: 校验 Command Spec 与参数 schema
    EX-->>UI: pending_confirm
    UI->>EX: 输入 CONFIRM
    EX-->>UI: pending_execute
    Agent->>EX: lease 下一个已确认任务
    EX->>Lease: 创建租约
    EX-->>Agent: lease_id + 受控 argv
    Agent->>EX: 上传 stdout/stderr chunk
    EX->>Log: 写脱敏日志片段
    Agent->>EX: 上报 exit_code / result_summary
    EX->>AU: 执行完成审计
    EX-->>UI: success / failed
```

## 6. 模块装配关系

```mermaid
flowchart TD
    Main["cmd/api/main.go"] --> Bootstrap["bootstrap.Init"]
    Bootstrap --> Identity["identity"]
    Identity --> Audit["audit"]
    Audit --> AI["ai"]
    Audit --> Alert["alert"]
    Audit --> Asset["asset"]
    Audit --> Runbook["runbook"]
    Audit --> Execution["execution"]
    Audit --> Integration["integration"]
    Audit --> Observability["observability"]
    Audit --> Inspection["inspection"]
    Alert --> Asset
    Alert --> Runbook
    Runbook --> Execution
    AI --> Execution
    Integration --> Observability
    Observability --> Inspection
    Inspection --> Execution
    Alert --> Dashboard["dashboard"]
    Asset --> Dashboard
    Runbook --> Dashboard
    Execution --> Dashboard
    Dashboard --> Server["server.RouteRegistrar"]
    Identity --> Server
    AI --> Server
    Alert --> Server
    Asset --> Server
    Runbook --> Server
    Execution --> Server
    Integration --> Server
    Observability --> Server
    Inspection --> Server
    Audit --> Server
```

## 7. 维护原则

- 图中实线表示当前代码或契约中已存在的调用方向；后续新增调用应优先补 application Port。
- AI、Inspection、Recommendation 只能创建待确认 Execution Task，不能直接派发或执行。
- Integration 负责凭据保存和账号能力；Observability 只拿脱敏账号摘要，不持有明文凭据。
- 前端 API 文档见 `web/src/api/README.md`，稳定运维契约见 `ops/*.md`。
