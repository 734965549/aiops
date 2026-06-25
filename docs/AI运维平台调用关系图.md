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

## 3. AI 工具调用与执行边界

```mermaid
sequenceDiagram
    participant UI as AI 助手页面
    participant AIH as AI HTTP Handler
    participant GW as Tool Gateway
    participant Authz as AuthorizationService
    participant Provider as AI Provider Executor
    participant EX as Execution Service
    participant AU as Audit Service

    UI->>AIH: POST /api/ai/tools/invoke
    AIH->>GW: Invoke(tool_code, resource, action, confirmed)
    GW->>Authz: 校验 RBAC / 数据范围 / AI 工具权限
    alt allowed=false
        GW-->>AIH: allowed=false, reason
        AIH-->>UI: 展示业务拒绝原因
    else 只读工具
        GW->>Provider: 调用受控 provider executor
        Provider-->>GW: 脱敏工具结果
        GW->>AU: 写 AI 工具调用审计
        GW-->>AIH: allowed=true, result
    else 需要执行
        GW->>EX: 只创建 Execution Task
        EX-->>GW: task_id, pending_confirm
        GW->>AU: 写任务提议审计
        GW-->>AIH: 返回待确认任务
    end
```

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
