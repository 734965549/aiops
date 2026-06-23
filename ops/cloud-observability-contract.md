# 云厂商只读接管与观测智能体契约草案

> 本文是 P1+ 演进契约草案，用于指导后续实现。当前 P0 已落地链路仍以 `alert-contract.md`、`ai-contract.md`、`execution-contract.md` 为准。
> 粤语版全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`，用嚟对齐 Integration、Observability、Inspection、Execution 之间嘅调用边界。

## 1. 范围

本契约覆盖：

- 云厂商和可观测系统账号只读接入。
- 资源、拓扑、指标、日志、链路、告警的统一查询。
- 巡检策略、巡检运行、巡检发现和 AI 建议。
- Agent 工具调用权限、审计和安全边界。
- 建议通知与生成 Execution Task 的衔接。

不覆盖：

- 云厂商写操作。
- 绕过 Execution 的自动变更。
- 在 Prompt、日志、审计或前端状态中暴露密钥。

## 2. 权限码

| 模块 | 权限码 | 说明 |
| --- | --- | --- |
| integrations | `app:integrations:read` | 查看接入账号、能力和连通性 |
| integrations | `app:integrations:create` | 创建接入账号 |
| integrations | `app:integrations:update` | 更新、禁用接入账号 |
| integrations | `app:integrations:delete` | 删除接入账号 |
| integrations | `app:integrations:check` | 连通性测试 |
| observability | `app:observability:read` | 查询指标、日志、链路和拓扑 |
| inspections | `app:inspections:read` | 查看巡检策略、运行、发现和建议 |
| inspections | `app:inspections:write` | 创建/更新巡检策略，手动触发巡检 |
| notifications | `app:notifications:read` | 查看通知通道和发送记录 |
| notifications | `app:notifications:write` | 配置通知通道和模板 |

AI 工具权限建议：

| tool_code | mode | 说明 |
| --- | --- | --- |
| `cloud.resources.list` | readonly | 查询资源 |
| `cloud.metrics.query` | readonly | 查询指标 |
| `cloud.logs.search` | readonly | 查询日志 |
| `cloud.traces.query` | readonly | 查询链路 |
| `cloud.topology.get` | readonly | 查询拓扑 |
| `inspection.runs.create` | confirm | 手动触发巡检 |
| `notification.messages.send` | confirm | 对外发送建议 |
| `execution.tasks.propose` | confirm | 从建议生成执行任务 |
| `execution.media.list` | readonly | 查询可用执行介体 |
| `execution.tasks.dispatch` | deny | Agent 不得直接分发或执行任务 |

## 3. 统一响应

所有接口沿用平台统一响应：

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {}
}
```

成功码必须是字符串 `"OK"`。

## 4. Integration API

### 4.1 创建接入账号

- Method: `POST`
- Path: `/api/integrations/accounts`
- Auth: Bearer + `app:integrations:create`

请求体：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `account_id` | string | 否 | 业务 ID；为空时后端生成 |
| `name` | string | 是 | 显示名称 |
| `provider` | string | 是 | `huawei_cloud` / `signoz` / `prometheus`（第一阶段占位，不含 `custom`） |
| `auth_type` | string | 是 | `ak_sk` / `agency` / `api_token` / `none`；须与 `provider` 兼容，见下表 |
| `regions` | string[] | 否 | 区域列表 |
| `project_id` | string | 否 | 华为云 project_id |
| `credential` | object | 视 auth_type | 凭据输入，仅写入时接收，不在响应中返回；`auth_type=none` 时可省略或传空对象 |
| `enabled` | boolean | 否 | 默认 true |
| `owner_team` | string | 否 | 数据归属团队 |
| `description` | string | 否 | 描述 |

`provider` 与 `auth_type` 兼容矩阵（创建/更新时校验，不兼容返回 `INVALID_ARGUMENT`）：

| provider | 支持的 auth_type |
| --- | --- |
| `huawei_cloud` | `none`, `ak_sk`, `agency` |
| `signoz` | `none`, `api_token` |
| `prometheus` | `none`, `api_token`, `ak_sk` |

示例：

```json
{
  "name": "华为云生产账号",
  "provider": "huawei_cloud",
  "auth_type": "ak_sk",
  "regions": ["cn-north-4"],
  "project_id": "project-xxx",
  "credential": {
    "access_key": "AK...",
    "secret_key": "SK..."
  },
  "enabled": true,
  "owner_team": "sre"
}
```

响应 `data`：

```json
{
  "account_id": "acc_xxx",
  "name": "华为云生产账号",
  "provider": "huawei_cloud",
  "auth_type": "ak_sk",
  "regions": ["cn-north-4"],
  "project_id": "project-xxx",
  "has_credential": true,
  "enabled": true,
  "owner_team": "sre",
  "created_at": 1710000000,
  "updated_at": 1710000000
}
```

### 4.2 列表

- Method: `GET`
- Path: `/api/integrations/accounts?page=1&page_size=20&provider=huawei_cloud&enabled=true`
- Auth: Bearer + `app:integrations:read`

响应使用 `pagination.PageData` 形态。

### 4.3 更新

- Method: `PUT`
- Path: `/api/integrations/accounts/:account_id`
- Auth: Bearer + `app:integrations:update`

`credential` 省略时保留原凭据；传入时替换并写审计。

### 4.4 删除或禁用

- Method: `DELETE`
- Path: `/api/integrations/accounts/:account_id`
- Auth: Bearer + `app:integrations:delete`

第一阶段建议软删除或禁用，不直接删除历史巡检证据。

### 4.5 连通性测试

- Method: `POST`
- Path: `/api/integrations/accounts/:account_id/check`
- Auth: Bearer + `app:integrations:check`

响应：

```json
{
  "status": "ok",
  "provider": "huawei_cloud",
  "capabilities": ["assets", "metrics", "logs", "traces", "topology", "alerts"],
  "checked_at": 1710000000,
  "message": "ok"
}
```

失败时 `message` 只能是脱敏摘要，不返回 AK/SK、Token、原始请求头或完整云端响应。

**实现状态（华为 CES 指标里程碑）**：

| provider / auth_type | 检查方式 | 说明 |
| --- | --- | --- |
| `huawei_cloud` + `auth_type=none` | 字段校验 | 无需凭据，直接返回默认能力 |
| `huawei_cloud` + `auth_type=ak_sk` | **字段校验（准真实）** | 校验 `access_key`/`secret_key` 非空、`regions` 非空；**尚未**调用 IAM/CES 做在线鉴权 |
| `huawei_cloud` + `auth_type=agency` | 字段校验 | 校验 `agency_name`/`domain_name` 非空、`regions` 非空 |
| `signoz` / `prometheus` | 字段校验 | 校验 token 或 `base_url` 等非空 |

真实云 API 连通性（如 CES `ShowMetricData` 探活）可在后续阶段替换 `HuaweiCloudChecker`，不影响 Observability HTTP 契约。

### 4.6 Provider 能力声明

连通性检查与 `integration_capability` 表使用下列 capability 字符串；Observability QueryService 按能力校验后再路由 Provider Port。

| capability | 说明 | 典型 Provider |
| --- | --- | --- |
| `metrics` | 指标时序 | `huawei_cloud` / `signoz` / `prometheus` |
| `logs` | 日志搜索 | `huawei_cloud` / `signoz` |
| `traces` | 链路 Span | `huawei_cloud` / `signoz` |
| `topology` | 服务/资源拓扑 | `huawei_cloud`（资源关系）、`signoz`（trace 派生） |
| `alerts` | 告警规则 | 全部 |
| `assets` | 云资源发现 | `huawei_cloud` |

**topology 与 traces 的关系**：`topology` 是独立能力，不隐含于 `traces`。Trace 派生拓扑的 Provider（如 SigNoz）在连通性检查时同时声明 `traces` 与 `topology`；来自 CMDB 或云资源关系的拓扑 Provider 可只声明 `topology` 而不声明 `traces`。`QueryTopology` 只校验 `topology` 能力，不复用 `traces`。

默认占位能力（fake / 连通性 checker）见 `internal/integration/domain/capability.go` 的 `DefaultCapabilitiesForProvider`；`prometheus` 默认仅 `metrics` + `alerts`，不含 `topology`。

## 5. Observability API

> **实现状态（阶段 3 指标里程碑 — 华为 CES 已落地）**：
>
> - **前端** `/integrations`：可创建 `huawei_cloud` + `auth_type=ak_sk` + `regions` + `project_id` 账号；凭据仅写入、不回显。
> - **连通性** `/api/integrations/accounts/:account_id/check`：`huawei_cloud` + `ak_sk` 当前为**字段级校验**（见 §4.5），不调用云端 API。
> - **指标查询** `/api/observability/metrics/query`：`huawei_cloud` + `auth_type=ak_sk` 走真实 CES `ShowMetricData`；返回平台标准 `MetricSeries`，生成 `evidence_id` 并写 `observability_query` 审计。
> - `signoz` / `prometheus`：全部为 fake adapter（确定性样本），CI 无需外部密钥。
> - `huawei_cloud` + `auth_type=none`：全部能力仍为 fake，便于无云账号联调。
> - `huawei_cloud` + `auth_type=ak_sk|agency` 的 logs/traces/topology/assets/alerts：返回 `FAILED_PRECONDITION`（capability unsupported），**不**返回 fake 样本，避免误当作云端数据。
> - 响应、日志、审计中不出现 AK/SK、`Authorization` header 或原始敏感云端报错；凭据在 API 进程装配时经 `CredentialProvider(integration_credential_ref + vault)` 解密，见 `cmd/api/main.go`。

### 5.0 Provider Port（Application 层）

Observability application 通过以下 Port 屏蔽厂商差异；infrastructure 按 `provider` 注册 `ProviderEntry`，QueryService 按能力对小 Port 做类型断言（真实 adapter 只需实现其支持的能力，如 Prometheus 仅 metrics/alerts）：

| Port | 方法 | 说明 |
| --- | --- | --- |
| `MetricQueryPort` | `QueryMetrics` | 指标时序 |
| `LogSearchPort` | `SearchLogs` | 日志搜索（脱敏摘要） |
| `TraceQueryPort` | `QueryTraces` | 链路 Span |
| `TopologyQueryPort` | `QueryTopology` | 服务拓扑（需账号声明 `topology` 能力） |
| `AssetDiscoveryPort` | `ListResources` | 云资源发现（阶段 2 HTTP / Asset Sync 复用） |
| `AlertRuleQueryPort` | `ListAlertRules` | 告警规则（阶段 2 Agent 工具复用） |

账号解析依赖 `IntegrationAccountPort`（adapter 包装 Integration 仓储），QueryService 编排：能力校验 → Provider Port → `obs_evidence_ref` → 审计。

第一阶段 fake provider 覆盖 `huawei_cloud`、`signoz`、`prometheus`；CI 无需云密钥。`huawei_cloud` 在 API 装配时已注入 `CredentialProvider`（integration credential repo + vault），`auth_type=ak_sk` 可走真实 CES 指标查询；`signoz`/`prometheus` 仍为 fake。

查询服务在 application 层统一归一化默认值和上限后再调用 Provider Port。`ListResources` 与 `ListAlertRules` 的 `limit` 默认 100、最大 500；超过上限返回 `INVALID_ARGUMENT`，不得把超大 limit 透传给真实 provider。

### 5.1 查询指标

- Method: `POST`
- Path: `/api/observability/metrics/query`
- Auth: Bearer + `app:observability:read`

请求体：

```json
{
  "account_id": "acc_xxx",
  "provider": "huawei_cloud",
  "region": "cn-north-4",
  "namespace": "SYS.ECS",
  "metric": "cpu_util",
  "dimensions": {
    "instance_id": "ecs-xxx"
  },
  "from": 1710000000,
  "to": 1710003600,
  "period": 60,
  "aggregator": "avg"
}
```

`period` 默认 60 秒，最小 10 秒；单次时间窗口最大 7 天，最多返回 1440 个点，超过限制返回 `INVALID_ARGUMENT`。

响应：

```json
{
  "series": [
    {
      "metric": "cpu_util",
      "unit": "Percent",
      "labels": {
        "instance_id": "ecs-xxx"
      },
      "points": [
        { "ts": 1710000000, "value": 42.1 }
      ]
    }
  ],
  "evidence_id": "ev_xxx"
}
```

### 5.2 搜索日志

- Method: `POST`
- Path: `/api/observability/logs/search`
- Auth: Bearer + `app:observability:read`

请求体：

```json
{
  "account_id": "acc_xxx",
  "provider": "huawei_cloud",
  "service": "payment-service",
  "resource_id": "res_xxx",
  "keyword": "timeout",
  "trace_id": "",
  "from": 1710000000,
  "to": 1710003600,
  "limit": 100
}
```

`limit` 默认 100，最大 500；超过上限返回 `INVALID_ARGUMENT`。响应只返回脱敏日志摘要和原始日志引用，不默认返回完整敏感日志。`obs_evidence_ref.summary` 不保存 `keyword` 原文，只可保存 `has_keyword`、`keyword_hash`、`keyword_len` 等脱敏摘要字段。

### 5.3 查询链路

- Method: `POST`
- Path: `/api/observability/traces/query`
- Auth: Bearer + `app:observability:read`

请求体：

```json
{
  "account_id": "acc_xxx",
  "provider": "signoz",
  "service": "payment-service",
  "operation": "POST /pay",
  "error_only": true,
  "min_latency_ms": 500,
  "from": 1710000000,
  "to": 1710003600,
  "limit": 50
}
```

`limit` 默认 50，最大 1000；超过上限返回 `INVALID_ARGUMENT`。

### 5.4 查询拓扑

- Method: `GET`
- Path: `/api/observability/topology?application_id=app_xxx&from=1710000000&to=1710003600`
- Auth: Bearer + `app:observability:read`

响应包含节点、边、调用量、错误率、P95/P99 等摘要字段。

## 6. Inspection API

> **实现状态（阶段 4 第一步）**：下列 API 已落地；运行触发后同步执行 Observability 证据采集 + `EvidenceAnalyzer` 规则分析（fake/真实 provider 均可），生成 Finding/Recommendation 并引用 `evidence_id`。定时 Worker 与 `Recommendation -> Execution` 接口留待后续阶段。

### 6.0 策略 scope 补充

契约示例未显式列出 `account_id`，但巡检需调用 Observability 查询，因此 `scope` JSON 必须包含：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `account_id` | string | 是 | Integration 接入账号业务 ID |
| `provider` | string | 否 | 默认取账号 provider |
| `environment` | string | 否 | 环境标签 |
| `application_ids` | string[] | 否 | 目标应用 |
| `resource_types` | string[] | 否 | 资源类型过滤 |

### 6.1 创建巡检策略

- Method: `POST`
- Path: `/api/inspections/policies`
- Auth: Bearer + `app:inspections:write`

请求体：

```json
{
  "name": "生产核心应用巡检",
  "enabled": true,
  "schedule": "*/5 * * * *",
  "scope": {
    "environment": "prod",
    "application_ids": ["app_xxx"],
    "resource_types": ["host", "pod", "service"]
  },
  "checks": [
    "metrics.cpu",
    "metrics.memory",
    "metrics.disk",
    "traces.latency",
    "traces.error_rate",
    "logs.error_burst"
  ],
  "agent_profile": "sre_default",
  "notification_policy_id": "np_xxx"
}
```

### 6.2 手动触发巡检

- Method: `POST`
- Path: `/api/inspections/policies/:policy_id/runs`
- Auth: Bearer + `app:inspections:write`

响应：

```json
{
  "run_id": "run_xxx",
  "policy_id": "policy_xxx",
  "status": "pending",
  "trigger_type": "manual",
  "created_at": 1710000000
}
```

### 6.3 查询巡检运行

- Method: `GET`
- Path: `/api/inspections/runs/:run_id`
- Auth: Bearer + `app:inspections:read`

状态机：

```text
pending -> running -> success|partial|failed|cancelled
```

### 6.4 查询发现和建议

- Method: `GET`
- Path: `/api/inspections/findings?run_id=run_xxx&risk_level=high`
- Auth: Bearer + `app:inspections:read`

`Finding` 必须包含：

- `finding_id`
- `risk_level`
- `summary`
- `affected_resources`
- `evidence_refs`
- `recommendations`
- `confidence`
- `uncertainty`

## 7. Recommendation 到 Execution

- Method: `POST`
- Path: `/api/inspections/recommendations/:recommendation_id/execution`
- Auth: Bearer + `app:executions:create`

请求体：

```json
{
  "execution_mode": "agent",
  "medium_id": "med-prod-jumpbox-01",
  "command_spec_id": "cmd_linux_disk_usage",
  "arguments": {
    "mount_point": "/"
  },
  "confirm_intent": "create_task_only"
}
```

要求：

- 只能创建 Execution Task，不能直接执行。
- medium/high/critical 风险必须进入 `pending_confirm`。
- 生成任务后写入 Recommendation 时间线和审计。
- `execution_mode=agent` 时，必须绑定执行介体或满足可解释的自动介体选择策略。
- `command_spec_id` 必须存在且启用，`arguments` 必须通过参数 schema 校验。
- Agent 只能提出执行计划；实际分发由 Execution 状态机在确认后触发。

执行介体、执行代理、Command Spec、租约和日志回传契约见 `execution-agent-contract.md`。

## 8. 审计动作

| resource_type | action |
| --- | --- |
| `integration_account` | `create` / `update` / `delete` / `check` |
| `observability_query` | `metrics_query` / `logs_search` / `traces_query` / `topology_get` |
| `inspection_policy` | `create` / `update` / `enable` / `disable` / `delete` |
| `inspection_run` | `create` / `start` / `finish` / `cancel` |
| `inspection_recommendation` | `create` / `notify` / `create_execution` |
| `notification` | `send` / `retry` / `fail` |
| `execution_medium` | `select` |
| `execution_agent` | `dispatch` |

审计 payload 只能保存参数摘要、计数、业务 ID、风险等级和脱敏错误，不保存密钥、Token、完整日志正文。

## 9. 错误码

| code | HTTP | 说明 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 400 | 参数错误 |
| `UNAUTHENTICATED` | 401 | 未登录或 token 无效 |
| `PERMISSION_DENIED` | 403 | 无权限 |
| `NOT_FOUND` | 404 | 账号、策略、运行或建议不存在 |
| `ALREADY_EXISTS` | 409 | 业务 ID 重复 |
| `FAILED_PRECONDITION` | 412 | 账号禁用、能力不支持、状态不允许 |
| `RESOURCE_EXHAUSTED` | 429 | 云 API 或平台限流 |
| `UNAVAILABLE` | 503 | 外部 Provider 不可用 |
| `INTERNAL` | 500 | 内部错误 |

## 10. 验收脚本规划

| 脚本 | 范围 |
| --- | --- |
| `scripts/e2e-integration.ps1` | 账号 CRUD、凭据不回显、连通性检查、权限负向 |
| `scripts/e2e-observability.ps1` | 指标/日志/链路 fake provider 查询、EvidenceRef 生成 |
| `scripts/e2e-inspection.ps1` | 策略 CRUD、手动触发、Finding/Recommendation、审计 |
| `scripts/e2e-notification.ps1` | 通知通道、发送记录、失败重试 |
| `scripts/e2e-execution-agent.ps1` | Recommendation 创建 agent 模式任务、介体选择、fake agent 领取、日志和结果回传 |

没有真实云账号的 CI 环境应使用 fake provider 或本地 mock server，避免将云厂商密钥放入测试环境。

## 11. 运行配置

凭据加密密钥与 JWT 独立配置，避免轮换 JWT 导致历史凭据无法解密：

```yaml
integration:
  credential_encryption_key: "<独立强密钥>"
  credential_encryption_key_version: 1   # 写入密文首字节，便于后续多版本轮换
```

环境变量：`AIOPS_INTEGRATION__CREDENTIAL_ENCRYPTION_KEY`、`AIOPS_INTEGRATION__CREDENTIAL_ENCRYPTION_KEY_VERSION`。

非 dev 环境 `Validate()` 会拒绝空密钥、弱密钥、以及与 `auth.jwt_secret` 相同的值。

`auth_type=none` 时 `credential` 可省略；若需保存 `base_url` 等非密钥连接参数，可传入 `credential` 对象，后端仍会加密存储但不强制密钥字段。
