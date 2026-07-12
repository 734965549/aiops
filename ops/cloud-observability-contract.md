# 云厂商只读接管与观测智能体契约

> 本文是云厂商只读接管与观测智能体的稳定契约：已落地章节对本闭环具备约束力，改动接口/状态机/权限码/迁移行为时必须同步更新本契约；尚未落地的事项以章节内"实现状态"块和"后续阶段/规划"标注为准，章节级总览见 §0。
> 当前 P0 主链路仍以 `alert-contract.md`、`ai-contract.md`、`execution-contract.md` 为准；本契约覆盖 Integration、Observability、Asset Sync、Inspection 及 Recommendation 到 Execution 的衔接。
> 全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`，用于对齐 Integration、Observability、Inspection、Execution 之间的调用边界。

## 0. 章节落地状态

| 章节 | 落地状态 | 说明 |
| --- | --- | --- |
| §2 权限码 | 大部分落地 | `integrations`/`observability`/`inspections` 已种子化（迁移 0018/0019/0020）；`notifications` 权限码尚未种子化 |
| §3 统一响应 | 已落地 | 沿用平台统一 envelope |
| §4 Integration API | 已落地 | 账号 CRUD、凭据不回显、连通性检查（华为 CES 指标里程碑，见 §4.5） |
| §5 Observability API | 部分落地 | 指标查询（华为 CES 真实）已落地；`ak_sk` 下 logs/traces/topology 返回 `FAILED_PRECONDITION`，fake provider 全可用 |
| §5.5 Asset Sync | 部分落地 | CES `ces`/`hybrid` 主路径完成，EVS 详情增强有已知缺口（见 §5.5） |
| §6 Inspection API | 部分落地 | 策略 CRUD、手动触发、Finding/Recommendation 已落地；**定时 Worker 未落地**（`schedule` 字段已持久化但无 worker 消费） |
| §7 Recommendation 到 Execution | 已落地 | `POST /api/inspections/recommendations/:recommendation_id/execution` 已实现 |
| §8 审计动作 | 大部分落地 | Integration/Observability/Inspection/Asset Sync 审计已接入；`notification` 审计动作随通知模块待落地 |
| §9 错误码 | 已落地 | 沿用平台错误码 |
| §10 验收脚本 | 大部分落地 | `e2e-integration`/`e2e-observability`/`e2e-asset-sync`/`e2e-inspection`/`e2e-execution-agent` 已存在；`e2e-notification.ps1` 待补 |
| §11 运行配置 | 已落地 | 凭据加密密钥独立配置 |

> 维护者：改动"已落地"章节的对外接口、状态机、权限码、迁移行为时，必须同步更新本契约及对应 §0 状态；调整"未落地/规划"事项时更新对应章节内"后续阶段"标注即可。

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
  "extra_config": {
    "sync_mode": "ces",
    "resource_group_name": "全部资源",
    "max_resources": 20000,
    "region_projects": [
      { "region": "cn-south-1", "project_id": "pid-south", "resource_group_id": "rg-south", "resource_group_name": "南方全量" }
    ]
  },
  "enabled": true,
  "owner_team": "sre"
}
```

`extra_config` 只允许保存 provider 专属非敏感配置，例如华为云资产同步的 `sync_mode`、`resource_group_name`、`resource_group_id`、`enterprise_project_id`、`max_resources`、`region_projects`（region → project_id 映射数组，每项可选填 `resource_group_id` / `resource_group_name`；多区域账号按 region 选用对应 project_id 与资源组，未命中回落账号顶层 `project_id` / 全局资源组）。密钥、Token、AK/SK、密码等仍只能通过 `credential` 写入凭据仓库，不能进入 `extra_config`。

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
  "extra_config": {
    "sync_mode": "ces",
    "resource_group_name": "全部资源",
    "max_resources": 20000,
    "region_projects": [
      { "region": "cn-south-1", "project_id": "pid-south", "resource_group_id": "rg-south", "resource_group_name": "南方全量" }
    ]
  },
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

`extra_config` 省略时保留原扩展配置；传入 `{}` 时清空为默认配置。后端会拒绝明显敏感字段名，响应也不会回显敏感键。

### 4.4 删除或禁用

- Method: `DELETE`
- Path: `/api/integrations/accounts/:account_id`
- Auth: Bearer + `app:integrations:delete`

第一阶段账号行软删除（保留用于审计/巡检证据，不直接删除历史巡检证据）；`integration_credential_ref` 中的凭据密文会在删除时一并硬删除，不再保留。

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
> - `huawei_cloud` + `auth_type=ak_sk|agency` 的 logs/traces/topology/alerts：返回 `FAILED_PRECONDITION`（capability unsupported），**不**返回 fake 样本，避免误当作云端数据。
- `huawei_cloud` + `auth_type=ak_sk` 的 **assets**：`sync_mode=ces` 默认走 CES 资源分组/资源列表发现，同步范围为**指定资源分组**下资源（默认候选名“全部资源”，需用户在 CES 控制台预先创建；未命中即失败，不静默回退最大资源组）；`sync_mode=native` 保留 ECS/CCE/RDS/ELB 原生只读兼容路径；`auth_type=none` 仍为 fake。完整模式定义见 `ops/huawei-ces-sync-contract.md`。
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
| `CloudFullSyncPort` | `ListAllResources` | 云资源全量同步发现（Asset Sync 专用，不受交互查询 `limit <= 500` 限制） |
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

### 5.5 云资源同步（Asset Sync，阶段 2 部分落地：CES P0/P1 主路径完成，hybrid 增强存在已知缺口）

> **实现状态**：`huawei_cloud` + `auth_type=ak_sk` 默认 `sync_mode=ces`，走 CES 资源分组/资源列表发现，同步范围为指定资源分组下资源；`sync_mode=hybrid` 先按指定资源分组入库，再按权限调用 ECS/CCE/RDS/ELB/EVS/VPC/DCS/DMS 原生 API 补充详情；VPC 子资源（EIP/带宽/子网/对等连接）客户端与 mapper 已接入且匹配键成立，hybrid 增强实际生效；EVS 客户端虽已接入但匹配键不成立，当前不承诺详情增强命中；`sync_mode=native` 才走旧 ECS/CCE/RDS/ELB 兼容路径。`auth_type=none` 仍为 fake，供 CI/E2E。同步写 `asset_resource`（`source=cloud_sync`）与 `asset_sync_batch`。完整功能依赖迁移链：`0023`（`asset_resource`/`asset_sync_batch` 基表）、`0024`（`integration_account.extra_config` 存放 `sync_mode`/`max_resources`/`region_projects` 等配置）、`0025`（`asset_resource.labels`）、`0026`（含 `region` 的部分唯一索引，区分多区域同类型同云 ID）、`0028`（`asset_sync_batch` 账号级 running 互斥与租约自愈）、`0031`（`asset_sync_batch.summary` 结构化摘要，替代 `message` 作为协议解析来源）、`0032`（破坏性 DELETE：按 `application_id = 'cloud-' || trim(account_id)` 精确关联 `integration_account` 删除旧格式 `cloud-<account_id>` 应用及其关联 `asset_resource`/`asset_match_rule`，不处理 `alert_alert`/`inspection_policy` 由 `0039` 补全清理）、`0033`（`asset_sync_batch.triggered_by` 触发用户，用于 reap 审计归因）、`0034`（按 `labels->>'dim_name'` 把 `SYS.VPC` 子资源拆分为 `eip`/`bandwidth`/`subnet`/`peering`/`vpc`）、`0035`（按 rune 截取修复多字节账号字节版 `application_id`，无损改写为 rune 版并合并子表引用，纯 ASCII 账号无影响）、`0036`（云同步应用名称追加 `account_id`，避免多账号同名混淆，影响 `asset_application.name`）、`0037`（修复 legacy/new `application_id` 并存时的安全合并：先迁移去重子表引用再删旧应用，仅 legacy 时安全重命名，仅 new 时幂等）、`0038`（归一化反向格式云同步应用名 `<provider>-<account_id>-cloud` 为契约格式 `<provider>-cloud-<account_id>`，收敛 `ensureCloudApplication` 代码曾误用的反向格式，仅改 `asset_application.name`，不改 `application_id`）、`0039`（清理 `0032` DELETE 遗留的 `alert_alert`/`inspection_policy` 孤儿引用，按 `integration_account` 计算 old->new 映射改写为新格式，不依赖 `has_old`）、`0040`（创建持久视图 `v_asset_app_ref_integrity` 暴露指向不存在 `asset_application` 的孤儿引用，不修改数据不阻断迁移，验收方式 `SELECT * FROM v_asset_app_ref_integrity` 期望 0 行）、`0041`（legacy 应用收敛硬阻断守卫：若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用则 `CHECK(n=0)` 失败导致迁移终止，不修改业务数据；若 0041 阻断需排查 0032/0037 收敛失败或代码路径仍在创建旧格式应用，修复后由 `0042` 收口补建）、`0042`（补建 0039 改写后仍被引用但不存在的新格式 cloud application ID 对应的 `asset_application` 记录，字段与 `ensureCloudApplication` 一致，并将 `v_asset_app_ref_integrity` 作为硬验收 `CHECK(n=0)`，补建后仍有孤儿则迁移失败；幂等 `ON CONFLICT DO NOTHING`；依赖 pgcrypto `digest()`）；升级后需对已有接入账号重新触发同步，无需重新录入账号。
>
> **目标状态**：华为云真实账号资产同步以 CES 资源分组为主路径，同步范围为**指定资源分组**下资源（默认候选名“全部资源”需用户在 CES 控制台预先创建；不存在“CES 总览全量”隐式口径，未命中指定组即失败，不回退最大资源组），覆盖 EVS/VPC/OBS/DCS/DMS/RDS/ELB/ECS 等 CES 可见资源。完整实现顺序、分页、映射、权限和验收标准见 `ops/huawei-ces-sync-contract.md`。
>
> **同步模式**：`ces` 是默认推荐模式，仅依赖 CES 资源分组发现指定分组下资源；`hybrid` 是增强模式，先按指定资源分组入库，再按权限调用原生 API 补详情。当前已支持 ECS/RDS/DCS/DMS 与 VPC 子资源（EIP/带宽/子网/对等连接）的 label 增强；EVS 原生客户端虽已接入调用链，但因 CES 维度与原生资源匹配键不成立，详情增强尚未支持；`native` 仅为兼容旧 ECS/CCE/RDS/ELB 路径，不作为资源分组完整性验收口径。
>
> **当前增强状态**：`sync_mode=hybrid` 已落地指定资源分组发现 + 原生 API 增强路由；ECS 当前补充 `private_ip`、`flavor`、`vpc_id`、`az`，RDS 当前补充 `private_ip`、`vpc_id`、`subnet_id`、`flavor`，DCS 补充 `instance_name/engine/engine_version/capacity_gb/spec_code/private_ip/az/vpc_id/charging_mode/created_at`，DMS（Kafka+RocketMQ 合并）补充 `instance_name/engine/engine_version/spec_code/capacity_gb/vpc_id/charging_mode/created_at`（Kafka 含 `private_ip`、无 `az`；RocketMQ 含 `az`、无 `private_ip`），VPC 子资源补充：EIP `public_ip/private_ip/bandwidth_id/share_type/status/ip_type`、带宽 `size_mbps/share_type/charge_mode/status`、子网 `cidr/gateway_ip/vpc_id/az/available_ip_count`、对等连接 `request_vpc_id/accept_vpc_id/status`。EVS 当前仅保证 CES 基础 labels（`namespace/dim_name/resource_group` 等）入库，`volume_type/size_gb/attached_to` 等详情增强因匹配键不成立尚未支持，原因见 `docs/huawei-ces-sync-backlog.md` 的 EVS/VPC 缺口章节，或参考 `ops/huawei-ces-sync-contract.md` §21.4。增强按 `ProviderRef` 匹配并只新增缺失 label，不覆盖 CES 已有 label；增强失败不影响 CES 基础资源入库，批次 `message` 追加 `enriched=N enrichment_failed=type1,type2` 摘要。OBS 原生增强仍属于后续扩展（需另引 OBS SDK）。

#### 5.5.1 触发同步

- Method: `POST`
- Path: `/api/assets/sync`
- Auth: Bearer + `app:assets:write`

> **数据范围校验**：HTTP 中间件完成 RBAC（`app:assets:write`）后，application 层通过注入的 `AuthorizationPort` 按 `integration_account.owner_team`（及 `regions`）做数据范围二次校验。具备 `app:assets:write` 但数据范围不覆盖目标账号 `owner_team`/`region` 的用户将收到 `403 PERMISSION_DENIED`，不会创建同步批次。用户无数据范围或具备 `all` 范围时不做过滤。多 region 账号逐 region 校验，**所有目标 region 都必须命中** 才放行；只命中部分 region 仍然拒绝，以避免跨区域越权触发同步。

请求体：

```json
{
  "account_id": "acc_xxx"
}
```

响应 `data`（触发后立即返回 `running` 批次，同步在后台执行；`running` 批次尚未生成 `summary`、`message` 为空，二者因 `omitempty` 均省略）：

```json
{
  "batch_id": "sync-xxx",
  "integration_account_id": "acc_xxx",
  "provider": "huawei_cloud",
  "status": "running",
  "created_count": 0,
  "updated_count": 0,
  "completed_count": 0,
  "stale_count": 0,
  "failed_count": 0,
  "application_id": "cloud-12345678901234567-a1b2c3d4e5f6",
  "started_at": 1710000000,
  "created_at": 1710000000,
  "updated_at": 1710000000
}
```

> `application_id` 采用新格式 `cloud-<账号前17位>-<sha1(账号)前12位>`（由 `cloudApplicationID` 生成；前 17 位按字符（rune）截取，与 SQL `left(trim(...),17)` 语义一致；迁移 `0032` 把存量旧格式 `cloud-<完整账号ID>` 应用及其关联 `asset_resource`/`asset_match_rule` 破坏性删除，保留 `integration_account`，由后续同步重建新格式应用，`alert_alert`/`inspection_policy` 中的孤儿引用由 `0039` 改写为新格式；迁移 `0035` 把旧实现按字节截取的多字节账号 application_id 无损改写为 rune 版），不再是旧格式 `cloud-<完整账号ID>`。终态批次才会填充 `summary` 与 `message`。

`status`：`running` / `success` / `partial` / `failed`。触发同步立即返回 `running`；客户端应轮询 `GET /api/assets/sync/batches/:batch_id` 到终态（`success`/`partial`/`failed`）。云端已删除资源仅标记 `sync_status=stale`，不物理删除。

终态批次返回 `summary` 结构化摘要（对齐 `SyncBatchSummaryDTO`），`message` 仅保留人类可读排查说明。`summary` 字段如下（计数/标志字段固定输出，零值表示该阶段无相关计数；`string`/`[]string`/`scopes` 等可空字段带 `omitempty`，空值不输出）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `sync_mode` | string | 同步模式（`ces`/`hybrid`/`native` 等） |
| `resource_group_name` | string | 资源组名称 |
| `resource_group_id` | string | 资源组 ID |
| `projects` | string[] | 涉及 project 列表（兼容聚合，丢失归属关系） |
| `regions` | string[] | 涉及 region 列表（兼容聚合，丢失归属关系） |
| `ces_total` | int | CES 拉取的指标维度总数（各 scope 求和） |
| `raw_fetched_count` | int | **本轮预算内实际进入后续处理链路的原始行数**；只统计已被 `max_resources` 接住并进入映射/去重/落库流水线的行，不表示云端总返回数，尾部被 `max_resources` 裁掉的行不计入 |
| `mapped_count` | int | 命名空间映射成功条目数 |
| `unique_discovered_count` | int | 去重后唯一发现资源数 |
| `persisted_count` | int | 持久化成功资源数（由 Asset 层按真实 upsert 结果回填） |
| `completed_count` | int | 完成数（summary 汇总口径下等于 `persisted_count`） |
| `duplicate_count` | int | 去重丢弃的重复条目数 |
| `persist_failed_count` | int | 持久化失败条目数（含逐条回退定位的坏资源） |
| `discovered_count` | int | 发现资源数（兼容历史口径，取 provider 的 `discovered`） |
| `failed_scopes` | string[] | 失败 scope 列表 |
| `enriched_count` | int | 原生 API 增强成功资源数 |
| `enrichment_failed_count` | int | 增强失败类型数 |
| `enrichment_failed_types` | string[] | 增强失败类型列表 |
| `enrichment_warnings` | string[] | best-effort 增强缺失列表（如 `dms.kafka`、`dms.rocketmq`、`vpc.subnet_count`、`<type>.truncated`）；`<type>.truncated` 表示该类型原生 API 结果因达到 `max_resources` 上限被截断，增强数据可能不完整；不影响批次状态，独立于 `enrichment_failed_types`，不参与 partial 判定 |
| `enrichment_stage_error` | string | 增强阶段整体致命错误描述（如端口不可用、装配错误）；非空时驱动 partial 判定，但不递增 `enrichment_failed_count` |
| `writeback_failed_count` | int | label 回写阶段失败次数；大于 0 时驱动 partial 判定，但不递增 `enrichment_failed_count` |
| `unknown_namespace_count` | int | 未知命名空间计数（未命中类型映射） |
| `invalid_resource_count` | int | 非法/无法解析资源计数 |
| `max_resources_reached` | bool | 是否触及单批次资源上限（命中时禁止该 scope 执行 stale） |
| `product_names_empty` | bool | 资源组 `product_names` 是否为空（空时回落兜底白名单） |
| `query_failed_types` | string[] | scope 查询失败的类型（已从 `successful_types` 剔除） |
| `conversion_failed_types` | string[] | 资源转换失败的类型（禁止该类型执行 stale） |
| `scopes` | object[] | 逐 scope 明细，结构见下表 |

客户端应优先读取 `summary`，仅对历史旧数据允许解析 `message` 兜底。

`scopes[]` 按 `region/project_id/resource_group` 保留逐 scope 明细（对齐 `SyncBatchScopeSummaryDTO`），每个元素字段如下（`region` 必填；计数/标志字段固定输出，零值表示该 scope 无相关计数；其余可空字段带 `omitempty`，空值不输出）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `region` | string | 区域（必填） |
| `project_id` | string | project ID |
| `sync_mode` | string | 该 scope 同步模式 |
| `resource_group_id` | string | 资源组 ID |
| `resource_group_name` | string | 资源组名称 |
| `resource_group_selection` | string | 资源组选择方式 |
| `ces_total` | int | 该 scope CES 指标维度总数 |
| `raw_fetched_count` | int | **本轮预算内实际进入后续处理链路的原始行数**；只统计已被 `max_resources` 接住并进入映射/去重/落库流水线的行，不表示云端总返回数，尾部被 `max_resources` 裁掉的行不计入 |
| `mapped_count` | int | 命名空间映射成功条目数 |
| `unique_discovered_count` | int | 去重后唯一发现资源数 |
| `persisted_count` | int | 持久化成功资源数 |
| `duplicate_count` | int | 去重丢弃的重复条目数 |
| `persist_failed_count` | int | 持久化失败条目数 |
| `discovered_count` | int | 发现资源数（兼容历史口径） |
| `failed_scopes` | string[] | 失败 scope 列表 |
| `successful_types` | string[] | 查询/转换成功的类型列表 |
| `query_failed_types` | string[] | 查询失败类型（已从 `successful_types` 剔除） |
| `conversion_failed_types` | string[] | 转换失败类型 |
| `enriched_count` | int | 增强成功资源数 |
| `enrichment_failed_count` | int | 增强失败类型数 |
| `enrichment_failed_types` | string[] | 增强失败类型列表 |
| `enrichment_warnings` | string[] | best-effort 增强缺失列表（如 `dms.kafka`、`dms.rocketmq`、`vpc.subnet_count`、`<type>.truncated`）；`<type>.truncated` 表示该类型原生 API 结果因达到 `max_resources` 上限被截断，增强数据可能不完整；不影响批次状态，独立于 `enrichment_failed_types`，不参与 partial 判定 |
| `enrichment_stage_error` | string | 增强阶段整体致命错误描述（如端口不可用、装配错误）；非空时驱动 partial 判定，但不递增 `enrichment_failed_count` |
| `writeback_failed_count` | int | label 回写阶段失败次数；大于 0 时驱动 partial 判定，但不递增 `enrichment_failed_count` |
| `unknown_namespace_count` | int | 未知命名空间计数 |
| `invalid_resource_count` | int | 非法资源计数 |
| `max_resources_reached` | bool | 是否触及资源上限 |
| `product_names_empty` | bool | `product_names` 是否为空 |

多区域排查“哪个 region 失败、用了哪个 project/group”必须读取 `scopes[]`，不要依赖顶层 `regions`/`projects`/`resource_group_name`（它们是兼容聚合字段，会丢失归属关系）。失败 region 的 scope 同样会写入 `scopes[]`，并补齐 `project_id`/`sync_mode`/`resource_group_id`/`resource_group_name`，便于定位。

**stale 标记语义**：`ces`/`hybrid` 模式下资源组 `product_names` 是权威 scope，采用反向 stale 标记——把 account+region 下所有 `active` 的 cloud_sync 资源（排除当前批次）标记为 `stale`，但跳过不确定类型（查询失败/转换失败/持久化失败）。这样从资源组移除的类型其旧资产也会被标记 stale，避免永久保持 `active`。`native`/通用/fake 路径 scope 非权威，仅对查询成功的类型逐类型标记 stale。`product_names` 为空时回落兜底白名单（不完整），批次至少标记 `partial`，message 含 `product_names_empty=true`。

**partial 判定规则与计数不变式**：批次终态为 `partial` 的条件（满足任一即 partial）：`failed_count > 0`、`max_resources_reached`、`product_names_empty`，或任一 scope 出现 `enrichment_failed_count > 0`/`enrichment_failed_types` 非空/`enrichment_stage_error` 非空/`writeback_failed_count > 0`/`invalid_resource_count > 0`/`conversion_failed_types` 非空/`query_failed_types` 非空。`enrichment_warnings` 非空不触发 partial。计数不变式：`enrichment_failed_count == len(enrichment_failed_types)` 始终成立；`enrichment_stage_error` 与 `writeback_failed_count` 驱动 partial 但不递增 `enrichment_failed_count`，保持该不变式不被破坏；`enrichment_warnings` 独立于 `enrichment_failed_types`，不参与计数与 partial 判定。

**账号级并发互斥**：同一 `integration_account_id` 同一时刻仅允许一个 `running` 批次（迁移 `0028` 部分唯一索引 `(integration_account_id) WHERE status='running'`）。若已有 `running` 批次，`POST /api/assets/sync` 返回 `409 ALREADY_EXISTS`，`message` 为 `sync already in progress for this account`，前端应提示「该账号正在同步，请稍后重试」。

**异步生命周期与租约续租**：同步在后台 goroutine 执行（`runCtx` 派生自进程级 `shutdownCtx` + 30 分钟硬超时），与 HTTP 请求生命周期解耦。`running` 批次写入 `lease_expires_at`（单次窗口 TTL 5 分钟）；后台 goroutine 每 60 秒续租一次，把 `lease_expires_at` 推进到 `now+TTL`，保证正常同步不会因超时被 reap。终态写入与审计使用独立短 context（10 秒超时），即便 `runCtx` 取消（进程关闭/硬超时）也能落终态，不卡 `running`。进程崩溃遗留的 `running` 批次由下一次同步在插入前 reap 为 `failed`（`message=lease expired; previous sync batch interrupted`），实现自愈，不依赖 Redis。该约束避免并发批次交错执行 `MarkStaleByAccountScopeExceptBatch` 时把对方刚写入的资源错误标记为 stale。批次表 `triggered_by`（迁移 `0033`）持久化触发人：`TriggerSync` 创建 running 批次后立即写 `sync_started` 审计，payload 包括 `account_id`、`provider`、`sync_mode`、`regions`、`resource_group`、`resource_group_id`、`projects`、`scopes[]`、`fencing_token`、`triggered_by`，进程崩溃仍可据此按 region/project/group 还原原批次操作范围；reap 崩溃批次时写 `sync_reaped` 审计，actor 取 `triggered_by` 而非当次请求用户，当次请求用户记入 payload `reaped_by`。

#### 5.5.2 同步批次列表

- Method: `GET`
- Path: `/api/assets/sync/batches?page=1&page_size=20&account_id=acc_xxx`
- Auth: Bearer + `app:assets:read`

> **数据范围过滤**：application 层解析用户数据范围允许的 `owner_team` 集合，仓储通过子查询 `integration_account` 按 `owner_team IN (...)` 过滤批次（分页 count 准确）。用户无数据范围或具备 `all` 范围时不做过滤；具备 `team` 范围时仅返回归属团队匹配的批次；仅有非 `team`/`all` 范围（如 `region`）时列表返回空集（单批次详情 `GET /api/assets/sync/batches/:batch_id` 仍按完整数据范围校验，region 用户可按 `batch_id` 查看）。

响应使用统一 envelope，`data` 为 `pagination.PageData<SyncBatch>`：

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "items": [],
    "total": 0,
    "page": 1,
    "page_size": 20
  }
}
```

#### 5.5.3 资源注册表扩展字段

`GET /api/assets/applications/:application_id/resources` 的 `Resource` 项新增：

| 字段 | 说明 |
| --- | --- |
| `source` | `manual` / `cloud_sync` |
| `integration_account_id` | 来源接入账号 |
| `cloud_resource_id` | 华为稳定 ID（如 ECS instance_id） |
| `cloud_resource_type` | 云资源类型，如 `ecs` / `cce` / `rds` / `elb` / `evs` / `vpc` / `dcs` / `dms` / `obs`；CES 新增 namespace 映射出的类型应原样返回 |
| `region` | 区域 |
| `sync_status` | `active` / `stale`（仅 cloud_sync） |
| `last_synced_at` | Unix 秒 |
| `sync_batch_id` | 最近成功批次 |
| `labels` | 云同步标签对象，空时返回 `{}`。CES 同步写入 `namespace/dim_name/resource_group`；原生 API 增强按类型写入 ECS `private_ip/flavor/vpc_id/az`、RDS `private_ip/vpc_id/subnet_id/flavor`、DCS `engine/capacity_gb`、DMS `engine/spec_code`、EIP `public_ip/private_ip/bandwidth_id`、带宽 `size_mbps/share_type`、子网 `cidr/gateway_ip/vpc_id/az`、对等连接 `request_vpc_id/accept_vpc_id/status` 等；EVS 当前仅保证 CES 基础 labels，`volume_type/size_gb/attached_to` 等详情增强尚未支持；manual 资源为 `{}`。每次同步整体覆盖 |

审计：`resource_type=asset_sync_batch`，`action=sync_started`/`sync_finished`/`sync_reaped`（见 §8）。批次表 `triggered_by` 持久化触发人，`sync_reaped` 审计 actor 取 `triggered_by` 而非当次请求用户。

#### 5.5.4 资产注册表列表分页

应用、资源、匹配规则三个列表接口统一采用标准分页（与 §5.5.2 同一 `pagination.PageData` 形态）。`page` 从 1 开始，默认 `page_size=20`，最大 `page_size=100`；HTTP 层通过 `pagination.Query.Normalize` 修正，application 层也会兜底归一化，避免非 HTTP 调用绕过分页上限。

资源列表额外支持服务端筛选参数：`cloud_resource_type`、`region`、`sync_status`，均为精确匹配；参数为空时不生效。前端不得对这些字段做跨页本地假筛选。

| 接口 | Method | Path | Auth | 排序 |
| --- | --- | --- | --- | --- |
| 应用列表 | `GET` | `/api/assets/applications?page=1&page_size=10` | Bearer + `app:assets:read` | `created_at ASC, id ASC` |
| 资源列表 | `GET` | `/api/assets/applications/:application_id/resources?page=1&page_size=10&cloud_resource_type=ecs&region=cn-north-4&sync_status=active` | Bearer + `app:assets:read` | `created_at ASC, id ASC` |
| 匹配规则列表 | `GET` | `/api/assets/match-rules?page=1&page_size=10` | Bearer + `app:assets:read` | `priority DESC, created_at ASC, id ASC` |

响应使用统一 envelope，`data` 为 `pagination.PageData<T>`：

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "items": [],
    "total": 0,
    "page": 1,
    "page_size": 10
  }
}
```

- `items` 元素结构：应用列表为 `Application`；资源列表为 `Resource`（含 §5.5.3 云同步扩展字段）；匹配规则列表为 `MatchRule`。
- 资源列表按 `application_id` 过滤；`application_id` 缺省返回 `INVALID_ARGUMENT`。
- 未传 `page` / `page_size` 时分别回退为 `1` / `20`；`page_size > 100` 时按 `100` 返回。
- 列表接口只承诺返回当前页数据，不是完整字典接口。前端用于下拉选择、按 URL 业务 ID 回显或跨页定位时，不能假设当前页包含目标应用/资源；如需完整选择能力，应使用搜索型选择接口或补充按业务 ID 查询接口。
- 当前阶段尚未提供 `GET /api/assets/applications/:application_id` 或 `GET /api/assets/resources/:resource_id` 的公开详情接口；新增这类接口时必须同步更新本节、`web/src/api/README.md` 和对应权限/测试。

## 6. Inspection API

> **实现状态（阶段 4 第一步）**：下列 API 已落地；运行触发后同步执行 Observability 证据采集 + `EvidenceAnalyzer` 规则分析（fake/真实 provider 均可），生成 Finding/Recommendation 并引用 `evidence_id`。`Recommendation -> Execution` 接口已落地（见 §7，`POST /api/inspections/recommendations/:recommendation_id/execution`）。**定时 Worker 仍未落地**：策略 `schedule` 字段已持久化、`TriggerType="scheduled"` 枚举已定义，但尚无后台 worker 按 cron 自动触发巡检，当前仅支持手动触发。

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
| `asset_sync_batch` | `sync_started` / `sync_finished` / `sync_reaped` |
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

## 10. 验收脚本

| 脚本 | 范围 | 状态 |
| --- | --- | --- |
| `scripts/e2e-integration.ps1` | 账号 CRUD、凭据不回显、连通性检查、权限负向 | 已落地 |
| `scripts/e2e-observability.ps1` | 指标/日志/链路 fake provider 查询、EvidenceRef 生成 | 已落地 |
| `scripts/e2e-asset-sync.ps1` | 云资源同步、批次、stale 标记、cloud_resource_id 入库 | 已落地 |
| `scripts/e2e-inspection.ps1` | 策略 CRUD、手动触发、Finding/Recommendation、审计 | 已落地 |
| `scripts/e2e-notification.ps1` | 通知通道、发送记录、失败重试 | 待补（通知模块未落地） |
| `scripts/e2e-execution-agent.ps1` | Recommendation 创建 agent 模式任务、介体选择、fake agent 领取、日志和结果回传 | 已落地 |

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
