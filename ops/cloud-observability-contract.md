# 云厂商只读接管与观测智能体契约

> 本文是云厂商只读接管与观测智能体的稳定契约：已落地章节对本闭环具备约束力，改动接口/状态机/权限码/迁移行为时必须同步更新本契约；尚未落地的事项以章节内"实现状态"块和"后续阶段/规划"标注为准，章节级总览见 §0。
> 当前 P0 主链路仍以 `alert-contract.md`、`ai-contract.md`、`execution-contract.md` 为准；本契约覆盖 Integration、Observability、Asset Sync、Inspection 及 Recommendation 到 Execution 的衔接。
> 全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`，用于对齐 Integration、Observability、Inspection、Execution 之间的调用边界。

## 0. 章节落地状态

| 章节 | 落地状态 | 说明 |
| --- | --- | --- |
| §2 权限码 | 大部分落地 | `integrations`/`observability`/`inspections` 已种子化（迁移 0018/0019/0020）；`notifications` 权限码尚未种子化 |
| §3 统一响应 | 已落地 | 沿用平台统一 envelope |
| §4 Integration API | 已落地 | 账号 CRUD、凭据不回显、字段级连接检查（见 §4.5）；checker 不访问外部服务，capabilities 非实测 |
| §5 Observability API | 部分落地 | 华为 AK/SK 指标真实，真实 logs/traces/topology 不支持；SigNoz/Prometheus 为 fake；AI 定稿快照已落地，无公共证据详情 API/UI |
| §5.5 Asset Sync | 部分落地 | CES `ces`/`hybrid` 主路径完成，EVS 详情增强有已知缺口（见 §5.5） |
| §6 Inspection API | 部分落地 | 策略 CRUD、手动触发、Finding/Recommendation 已落地；**定时 Worker 未落地**（`schedule` 字段已持久化但无 worker 消费） |
| §7 Recommendation 到 Execution | 后端已落地 | 只允许 agent 且 medium_id 必填；无前端入口、自动介体、跨模块事务或完成回写，详见 §7 |
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

migration `0020` 另种子 `inspection.runs.create` 和 `inspection.findings.analyze` AI 工具权限，但当前没有对应可执行 AI 工具注册或 Handler；权限种子不代表工具能力已交付。

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

当前提供 6 个账号路由和 `read/create/update/delete/check` 五类权限。所有路由使用 `AuthorizeStatic`：只检查 RBAC，不按 `owner_team`、region 或其他对象字段自动应用数据范围过滤。migration `0018` 默认将五类权限绑定 admin，并迁移/移除旧 `app:integrations:write`。

Integration 只管理账号、凭据引用、默认能力声明和字段级检查；真实观测/资源能力由对应 Provider Adapter 单独验收。

### 4.1 创建接入账号

- Method: `POST`
- Path: `/api/integrations/accounts`
- Auth: Bearer + `app:integrations:create`

请求体：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `account_id` | string | 否 | 业务 ID；为空时后端生成 |
| `name` | string | 是 | 显示名称 |
| `provider` | string | 是 | 当前支持 `huawei_cloud` / `signoz` / `prometheus`，不含 `custom`；真实能力按 Provider Adapter 分开验收 |
| `auth_type` | string | 是 | `ak_sk` / `agency` / `api_token` / `none`；须与 `provider` 兼容，见下表 |
| `regions` | string[] | 否 | 区域列表 |
| `project_id` | string | 否 | 华为云 project_id |
| `credential` | object | 视 auth_type | 凭据输入，仅写入时接收，不在响应中返回；`auth_type=none` 时可省略或传空对象 |
| `enabled` | boolean | 否 | 默认 true |
| `owner_team` | string | 否 | 数据归属团队 |
| `description` | string | 否 | 描述 |
| `extra_config` | object | 否 | provider 专属非敏感配置；华为新建省略或传空对象时默认 `sync_mode=ces` |

`provider` 与 `auth_type` 兼容矩阵（创建/更新时校验，不兼容返回 `INVALID_ARGUMENT`）：

| provider | 支持的 auth_type |
| --- | --- |
| `huawei_cloud` | `none`, `ak_sk`, `agency` |
| `signoz` | `none`, `api_token` |
| `prometheus` | `none`, `api_token`, `ak_sk` |

非 none 创建必须提供对应凭据：AK/SK 为 `access_key + secret_key`，agency 为 `agency_name + domain_name`，api_token 接受 `api_token` 或 `access_token`；Prometheus 非 none 另需 `credential.base_url`。`none` 可省略凭据，但传入的非空配置仍会加密保存并使 `has_credential=true`。

regions 会去除空项、去重并排序。华为 region 只允许小写字母、数字和合法连字符；创建时的格式校验不代替 checker 对非 none 华为账号至少一个 region 的检查。

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

`extra_config` 只允许保存 provider 专属非敏感配置，例如华为云资产同步的 `sync_mode`、`resource_group_name`、`resource_group_id`、`enterprise_project_id`、`max_resources`、`region_projects`（region → project_id 映射数组，每项可选填 `resource_group_id` / `resource_group_name`；多区域账号按 region 选用对应 project_id 与资源组，未命中回落账号顶层 `project_id` / 全局资源组）。服务端递归拒绝明显敏感键，普通未知非敏感键可保留；`region_projects` 数组项的未知字段会被拒绝。密钥、Token、AK/SK、密码等仍只能通过 `credential` 写入凭据仓库，不能进入 `extra_config`。

响应 `data`：

```json
{
  "account_id": "acc-<uuid>",
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

账号、可选凭据引用和默认 capabilities 在同一事务创建。响应不包含 `credential`、密文、指纹或 `credential_ref_id`；`capabilities` 为 provider 默认声明，`last_check_status` 在首次检查前省略。

migration `0024` 增加 `extra_config`；`0029` 将仍为空配置的历史华为账号回填为 `sync_mode=native`，与新建账号默认 CES 区分。

### 4.2 列表

- Method: `GET`
- Path: `/api/integrations/accounts?page=1&page_size=20&provider=huawei_cloud&enabled=true`
- Auth: Bearer + `app:integrations:read`

查询参数：

- `page` 默认 1，`page_size` 默认 20、最大 100。
- `provider` 精确匹配，`enabled` 可选布尔筛选。
- `keyword` 对 name、account_id、owner_team 执行 ILIKE 包含匹配；当前前端没有关键字输入。

列表只包含 `deleted=false`，按 `created_at DESC, id DESC` 排序，响应使用 `pagination.PageData` 形态。每项包含当前 capabilities 和最近一次检查的 `last_check_status`，不包含检查历史或凭据内容。

`GET /api/integrations/accounts/:account_id` 使用同一读权限；已软删除账号按 404 处理。

### 4.3 更新

- Method: `PUT`
- Path: `/api/integrations/accounts/:account_id`
- Auth: Bearer + `app:integrations:update`

更新为部分字段语义：

- `credential` 省略或传空对象时保留原凭据；传入非空内容时替换。当前没有通过更新清空凭据的语义，切换到 `auth_type=none` 也不会自动删除旧密文。
- `regions` 省略时保留，传 `[]` 时清空；`extra_config` 省略时保留，传 `{}` 时恢复 provider 默认配置。
- provider/auth_type 变化会校验新凭据或已存凭据；provider 变化还会刷新默认 capabilities。后端允许 provider 更新，但当前前端编辑弹窗禁用了该字段。
- 更新凭据或 provider 时使用事务；审计在更新成功后 best-effort 写入，不与业务同事务。

后端会拒绝明显敏感字段名，响应也会清理旧数据中的敏感 extra_config 键。

### 4.4 删除或禁用

- Method: `DELETE`
- Path: `/api/integrations/accounts/:account_id`
- Auth: Bearer + `app:integrations:delete`

账号行软删除并同时设置 `enabled=false`，保留用于审计/巡检证据；`integration_credential_ref` 中的凭据密文会在同一事务中硬删除，不再保留。capabilities 和检查历史没有随删除清理，软删除账号不再出现在列表，详情/更新/检查按 404 处理，当前无恢复接口。

仅需停用时可通过 PUT 设置 `enabled=false`，此时凭据仍保留，但连通性检查返回 412 `FAILED_PRECONDITION`。

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

checker 出错时，服务生成 `status=failed` 的检查结果并保存脱敏 `message`；只要检查结果和 capabilities 成功落库，HTTP 仍为 200 且 `code="OK"`，调用方必须检查 `data.status`。账号不存在为 404，disabled 为 412，checker 未配置为 503，结果持久化失败才作为请求错误返回。

失败时 `message` 只能是脱敏摘要，不返回 AK/SK、Token、原始请求头或完整云端响应。每次检查写入 `integration_check_result`，账号 DTO 只返回最近一次 `last_check_status`，当前没有检查历史 API。

**实现状态（当前全部为字段级 checker）**：

| provider / auth_type | 检查方式 | 说明 |
| --- | --- | --- |
| `huawei_cloud` + `auth_type=none` | 字段校验 | 无需凭据，直接返回默认能力 |
| `huawei_cloud` + `auth_type=ak_sk` | 字段校验 | 校验 `access_key`/`secret_key` 非空、`regions` 非空；不调用 IAM/CES 做在线鉴权 |
| `huawei_cloud` + `auth_type=agency` | 字段校验 | 校验 `agency_name`/`domain_name` 非空、`regions` 非空 |
| `signoz` | 字段校验 | api_token 模式检查 token；不访问 SigNoz |
| `prometheus` | 字段校验 | 非 none 检查 `base_url`，api_token 另检查 token；不访问 Prometheus |

`status=ok` 不证明网络可达、凭据有效、云端权限足够或每项 capability 可用。真实云 API/观测平台探活可在后续替换对应 Checker，不影响 Observability HTTP 契约。

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

创建时直接写默认声明；检查时若 checker 未返回 capabilities，也会补默认声明，包括 `status=failed` 的结果。因而 capabilities 是后续模块的门控元数据，不是在线实测通过清单，真实支持范围仍须按 §5 和对应 Provider 契约验收。

### 4.7 凭据存储与轮换边界

- 使用独立密钥派生 AES-GCM 密钥、随机 nonce 加密 credential JSON，密文首字节为当前 key version。
- `integration_credential_ref.external_ref` 和 `store_type=external_ref` 为预留模型，当前写入路径固定 `local_encrypted`，没有外部 Secret Manager 实现。
- 当前 Vault 只持有当前密钥和版本；版本不匹配直接拒绝，未提供多版本解密、在线轮换或批量重加密工具。
- 创建账号/凭据/capabilities、更新凭据或 provider、删除账号/凭据、保存检查结果/capabilities 分别使用对应事务；审计不在这些事务内。

### 4.8 前端与审计边界

当前 `/integrations` 支持列表、Provider/启用筛选、创建/编辑、启停、删除、检查和 Asset Sync 入口；API 的 keyword、provider 更新、检查历史和凭据清空没有对应完整页面能力。

- 编辑不回显凭据，留空即保留；provider 在页面禁用修改。
- 页面缺少 `agency_name/domain_name`，agency 只能通过 API 完整配置。
- 检查 HTTP 成功后页面统一显示成功 Toast，即使 `data.status=failed`；默认 capabilities 非空时可能不显示失败 message，验收必须看业务 status，不能只看 Toast。
- 页面只有华为 extra_config 纯函数 Vitest，没有完整账号页面测试。
- 创建/更新/删除/检查写 `resource_type=integration_account`、同名 action，Payload 不含凭据；审计为业务事务后的 best-effort，失败不回滚业务。

## 5. Observability API

当前只有 metrics/logs/traces/topology 四个公共查询路由，统一要求 Bearer + `app:observability:read`，使用静态 RBAC；账号删除/禁用由 AccountAdapter 拒绝，但公共查询不按 owner_team/region 做对象级数据范围过滤。AI 专用采集链路的账号数据范围校验是独立边界。

Provider 当前支持矩阵：

| Provider / auth | metrics | logs / traces / topology | assets / alerts |
| --- | --- | --- | --- |
| `huawei_cloud + none` | fake | fake | fake |
| `huawei_cloud + ak_sk` | 真实 CES `ShowMetricData` | `FAILED_PRECONDITION`，不返回 fake | 真实资源发现/同步与 CES 规则；CES 告警拉取见 Alert 契约 |
| `huawei_cloud + agency` | unsupported | unsupported | unsupported |
| `signoz`（含 api_token） | fake | fake | fake 告警规则，无 assets Port |
| `prometheus`（含 api_token/ak_sk） | fake | 无对应 Port | fake 告警规则，无 assets Port |

capabilities 是 Integration 默认声明和 QueryService 的第一道门控，不是实测结果。真实凭据下不支持的能力必须显式失败，不能回退 fake。`signoz` 和 `prometheus` 当前即使配置 token 也仍返回确定性 fake 样本。

真实华为资源同步以 CES 资源分组为主路径：显式组未命中失败，默认候选名均未命中时进入 `all_groups` 并集；该并集不等于未分组资源全量。完整模式与 stale 门控见 `ops/huawei-ces-sync-contract.md`。

响应、日志、审计中不得出现 AK/SK、`Authorization` header 或原始敏感云端报错；真实华为凭据由服务端 `CredentialProvider(integration_credential_ref + vault)` 解密，见 `cmd/api/main.go`。

### 5.0 Provider Port（Application 层）

Observability application 通过以下 Port 屏蔽厂商差异；infrastructure 按 `provider` 注册 `ProviderEntry`，QueryService 按能力对小 Port 做类型断言（真实 adapter 只需实现其支持的能力，如 Prometheus 仅 metrics/alerts）：

| Port | 方法 | 说明 |
| --- | --- | --- |
| `MetricQueryPort` | `QueryMetrics` | 指标时序 |
| `LogSearchPort` | `SearchLogs` | 日志搜索（脱敏摘要） |
| `TraceQueryPort` | `QueryTraces` | 链路 Span |
| `TopologyQueryPort` | `QueryTopology` | 服务拓扑（需账号声明 `topology` 能力） |
| `AssetDiscoveryPort` | `ListResources` | 云资源发现，供 Asset Sync / AI 资源工具复用；无独立公共 HTTP 路由 |
| `CloudFullSyncPort` | `ListAllResources` | 云资源全量同步发现（Asset Sync 专用，不受交互查询 `limit <= 500` 限制） |
| `AlertRuleQueryPort` | `ListAlertRules` | 告警规则（AI 规则体检复用）；返回含 `metric/operator/threshold/unit/window_seconds` 可选字段以及 `resource_count/resources`（关联资源总数与前 3 个维度样本）。huawei `auth_type=none` 走 fake，`ak_sk` 走真实 CES V2 `ListAlarmRules`，`agency` 返回 unsupported |
| `AlarmRecordPullPort` | `PullAlarmRecords` | CES V2 告警记录拉取，供 Alert 模块使用；不是公共 Observability 路由 |

`rds_history_client.go` 另为 AI RDS for PostgreSQL 历史会话、等待事件和 Top SQL 提供只读客户端；它不属于公共 Observability HTTP 路由，实例和数据范围由 AI 调用链约束。

账号解析依赖 `IntegrationAccountPort`（adapter 包装 Integration 仓储），QueryService 编排：能力校验 → Provider Port → `obs_evidence_ref` → 审计。生产仓储已配置时，证据保存失败会终止查询；审计在证据之后 best-effort 写入。测试或未装配仓储的调用可只生成 evidence_id，不能据此证明生产已落库。

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

`period` 默认 60 秒，平台通用最小 10 秒；HTTP 查询单次时间窗口最大 7 天，按 window/period 估算最多 1440 点，超过限制返回 `INVALID_ARGUMENT`。AI 只读工具路径（`ops/ai-contract.md` §10.12.1）允许超过 7 天、最长 30 天的窗口，但 `period` 不得小于 3600（小时级），仍受 1440 点上限约束。

当前公共 HTTP Handler 不接收 `project_id`，前端类型也未暴露；多项目公共查询按账号顶层 project_id 或 `extra_config.region_projects` 的 region 映射解析。内部 `domain.MetricQuery` 和 AI 路径才支持显式 project_id，不能把早期示例当作公共逐请求覆盖能力。

华为真实指标另要求 region 属于账号、namespace 必填、1–4 个 dimensions，且最终 project_id 可解析。CES SDK 仅接受其枚举：当前服务实际可用 period 为 60/300/1200/3600/14400/86400，filter 为 average/avg/max/min/sum/variance/var。通用 period>=10 不保证真实 CES 可用，前端数值输入也不能替代该枚举校验。

响应 `unit` 优先取 provider 数据点单位；provider 未返回时按官方文档固定表推断（`rds001/rds002/rds039` 使用率、`rds072/rds083` 连接数使用率为 `Percent`，`rds045` 为 `MiB`，`rds073` 为 `s`，`rds042`/`active_connections` 为 `Count`，`oldest_transaction_duration` 为 `ms`）。未登记指标保持空字符串，不得按名称后缀臆造单位。

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
  "evidence_id": "ev-<uuid>"
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
- Path: `/api/observability/topology?account_id=acc_xxx&application_id=app_xxx&from=1710000000&to=1710003600`
- Auth: Bearer + `app:observability:read`

`account_id`、`from`、`to` 必填。响应包含节点、边、调用量、错误率和 P95 等摘要字段；当前 DTO 没有 P99。这里的运行时调用拓扑与 CMDB `/assets/topology` 是不同模型。

### 5.4.1 证据引用与 AI 定稿快照

普通成功查询生成 `ev-<uuid>` 并保存 query_hash、脱敏 summary 和 status=ok；Metrics 还保存 provider、trace_id、实际 query_params、资源标识、业务窗口和 collected_at，但 payload 为空。Logs 的 keyword 只保存 hash/len，不保存原文。历史 0019 证据在 0060 后仍保持 summary-only，不伪造 payload。

AI 指标使用 SnapshotRequired 时，初始记录为 collecting；只有定稿快照保存成功后才以 ok 进入模型支持性证据：

```text
采集 -> 统计/降采样/大小裁剪 -> SaveSnapshot -> payload/payload_hash -> 模型输入
```

- 对话路径每序列最多 288 点，`sampling.strategy=stride_first_last` 保留首末点；峰值时刻（`peak_at`）与破限片段/越限时长等统计基于完整序列计算并在裁剪后保留。
- payload 默认最多 32KiB，超限时先清 points、再清 metrics；original_count/retained_count 记录裁剪前后总点数，retained_count 在 payload/summary 而非独立列。
- payload_hash 是定稿 payload 经规范化 JSON/JSONB 的 SHA-256，不是云端原始响应 hash。
- OK 证据存储未配置、编码或保存失败会 fail-closed：转 error、清 metrics/evidence_id，不作为支持性 references。
- skipped/permission_denied/unsupported/error 可尽力保存失败快照用于追踪，但不能作为支持性引用。

`SnapshotStore` 也保存 `evidence_type=postgres_activity`、`schema_version=ai.postgres_activity.v1` 的 RDS 历史会话/等待事件/Top SQL 脱敏快照。详细诊断语义见 `ops/ai-contract.md`。

当前没有 `GET /api/observability/evidence/:id` 等公共详情接口或前端证据抽屉；普通 evidence_id 不保证存在 payload。

### 5.5 云资源同步（Asset Sync，阶段 2 部分落地：CES P0/P1 主路径完成，hybrid 增强存在已知缺口）

> **实现状态**：`huawei_cloud` + `auth_type=ak_sk` 默认 `sync_mode=ces`，走 CES 资源分组/资源列表发现，同步范围为指定资源分组下资源；`sync_mode=hybrid` 先按指定资源分组入库，再按权限调用 ECS/CCE/RDS/ELB/EVS/VPC/DCS/DMS 原生 API 补充详情；VPC 子资源（EIP/带宽/子网/对等连接）客户端与 mapper 已接入且匹配键成立，hybrid 增强实际生效；EVS 客户端虽已接入但匹配键不成立，当前不承诺详情增强命中；`sync_mode=native` 才走旧 ECS/CCE/RDS/ELB 兼容路径。`auth_type=none` 仍为 fake，供 CI/E2E。同步写 `asset_resource`（`source=cloud_sync`）与 `asset_sync_batch`。完整功能依赖迁移链：`0023`（`asset_resource`/`asset_sync_batch` 基表）、`0024`（`integration_account.extra_config` 存放 `sync_mode`/`max_resources`/`region_projects` 等配置）、`0025`（`asset_resource.labels`）、`0026`（含 `region` 的部分唯一索引，区分多区域同类型同云 ID）、`0028`（`asset_sync_batch` 账号级 running 互斥与租约自愈）、`0031`（`asset_sync_batch.summary` 结构化摘要，替代 `message` 作为协议解析来源）、`0032`（破坏性 DELETE：按 `application_id = 'cloud-' || trim(account_id)` 精确关联 `integration_account` 删除旧格式 `cloud-<account_id>` 应用及其关联 `asset_resource`/`asset_match_rule`，不处理 `alert_alert`/`inspection_policy` 由 `0039` 补全清理）、`0033`（增加 `asset_sync_batch.triggered_by` 列；当前运行时写入与原操作者归因尚未完整落地，见本节差距说明）、`0034`（按 `labels->>'dim_name'` 把 `SYS.VPC` 子资源拆分为 `eip`/`bandwidth`/`subnet`/`peering`/`vpc`）、`0035`（按 rune 截取修复多字节账号字节版 `application_id`，无损改写为 rune 版并合并子表引用，纯 ASCII 账号无影响）、`0036`（云同步应用名称追加 `account_id`，避免多账号同名混淆，影响 `asset_application.name`）、`0037`（修复 legacy/new `application_id` 并存时的安全合并：先迁移去重子表引用再删旧应用，仅 legacy 时安全重命名，仅 new 时幂等）、`0038`（归一化反向格式云同步应用名 `<provider>-<account_id>-cloud` 为契约格式 `<provider>-cloud-<account_id>`，收敛 `ensureCloudApplication` 代码曾误用的反向格式，仅改 `asset_application.name`，不改 `application_id`）、`0039`（清理 `0032` DELETE 遗留的 `alert_alert`/`inspection_policy` 孤儿引用，按 `integration_account` 计算 old->new 映射改写为新格式，不依赖 `has_old`）、`0040`（创建持久视图 `v_asset_app_ref_integrity` 暴露指向不存在 `asset_application` 的孤儿引用，不修改数据不阻断迁移，验收方式 `SELECT * FROM v_asset_app_ref_integrity` 期望 0 行）、`0041`（legacy 应用收敛硬阻断守卫：若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用则 `CHECK(n=0)` 失败导致迁移终止，不修改业务数据；若 0041 阻断需排查 0032/0037 收敛失败或代码路径仍在创建旧格式应用，修复后由 `0042` 收口补建）、`0042`（补建 0039 改写后仍被引用但不存在的新格式 cloud application ID 对应的 `asset_application` 记录，字段与 `ensureCloudApplication` 一致，并将 `v_asset_app_ref_integrity` 作为硬验收 `CHECK(n=0)`，补建后仍有孤儿则迁移失败；幂等 `ON CONFLICT DO NOTHING`；依赖 pgcrypto `digest()`）；升级后需对已有接入账号重新触发同步，无需重新录入账号。
>
> **当前资源组口径**：华为云真实账号资产同步以 CES 资源分组为主路径。显式指定 `resource_group_id` 或非默认 `resource_group_name` 时，未命中即失败；未指定或默认候选名（全部资源 / All resources / All Resources）均未命中时，进入 `all_groups`，同步该 region/project 下全部资源组的并集。该并集仍不等于 CES 总览中未分组资源的全量口径，stale 必须按 `all_groups` 的保守门控处理。完整实现顺序、分页、映射、权限和验收标准见 `ops/huawei-ces-sync-contract.md`。
>
> **同步模式**：`ces` 是默认推荐模式，仅依赖 CES 资源分组发现指定分组下资源；`hybrid` 是增强模式，先按指定资源分组入库，再按权限调用原生 API 补详情。当前已支持 ECS/RDS/DCS/DMS 与 VPC 子资源（EIP/带宽/子网/对等连接）的 label 增强；EVS 原生客户端虽已接入调用链，但因 CES 维度与原生资源匹配键不成立，详情增强尚未支持；`native` 仅为兼容旧 ECS/CCE/RDS/ELB 路径，不作为资源分组完整性验收口径。
>
> **当前增强状态**：`sync_mode=hybrid` 已落地指定资源分组发现 + 原生 API 增强路由；ECS 当前补充 `private_ip`、`flavor`、`vpc_id`、`az`，RDS 当前补充 `private_ip`、`vpc_id`、`subnet_id`、`flavor`，DCS 补充 `instance_name/engine/engine_version/capacity_gb/spec_code/private_ip/az/vpc_id/charging_mode/created_at`，DMS（Kafka+RocketMQ 合并）补充 `instance_name/engine/engine_version/spec_code/capacity_gb/vpc_id/charging_mode/created_at`（Kafka 含 `private_ip`、无 `az`；RocketMQ 含 `az`、无 `private_ip`），VPC 子资源补充：EIP `public_ip/private_ip/bandwidth_id/share_type/status/ip_type`、带宽 `size_mbps/share_type/charge_mode/status`、子网 `cidr/gateway_ip/vpc_id/az/available_ip_count`、对等连接 `request_vpc_id/accept_vpc_id/status`。EVS 当前仅保证 CES 基础 labels（`namespace/dim_name/resource_group` 等）入库，`volume_type/size_gb/attached_to` 等详情增强因匹配键不成立尚未支持，原因见 `docs/huawei-ces-sync-backlog.md` 的 EVS/VPC 缺口章节，或参考 `ops/huawei-ces-sync-contract.md` §21。增强按 `ProviderRef` 匹配并只新增缺失 label，不覆盖 CES 已有 label；增强失败不影响 CES 基础资源入库，批次 `message` 追加 `enriched=N enrichment_failed=type1,type2` 摘要。OBS 原生增强仍属于后续扩展（需另引 OBS SDK）。
>
> **2026-11-23 代码复核差距**：以下历史契约目标尚未完整落地，不能计入当前完成度：
> 1. Sync 触发和批次查询的数据范围校验未按 §5.5.1/§5.5.2 接入；AI 资源读取的独立校验不能替代证明。
> 2. 审计仍统一使用 `action=sync`，没有独立的 `sync_started/sync_finished/sync_reaped`，也没有完整的嵌套 summary/scopes 审计载荷。
> 3. `triggered_by` 未在 TriggerSync 写入，reap actor 仍取当次请求用户；过期批次只在下次同账号触发时内联 reap，未装配周期后台 reaper。
> 4. 新建云同步应用名仍可能为 `<provider>-cloud`，0036/0038 的历史迁移归一化不保证后续新建名称附带 account_id。

#### 5.5.1 触发同步

- Method: `POST`
- Path: `/api/assets/sync`
- Auth: Bearer + `app:assets:write`

> **当前数据范围边界**：HTTP 中间件完成 RBAC（`app:assets:write`）后，`SyncService` 校验账号 enabled 与 assets capability，但当前没有调用 `AuthorizationPort` 对 `integration_account.owner_team`/`regions` 做二次授权。因而不能宣称仅数据范围不覆盖的用户会收到 403，也不能宣称多 region 已逐一校验。AI 资源读取/发现有独立对象级校验，是另一条调用链。若后续补齐 Sync 授权，必须增加 owner_team、region、all/无范围、多 region 全命中和部分命中的专项测试。

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
| `resource_group_name` | string | 资源组名称（聚合口径下固定为 `全部资源`） |
| `resource_group_id` | string | 资源组 ID（聚合口径下为空） |
| `resource_group_count` | int | 本轮实际覆盖的资源组数量（聚合口径下 > 1） |
| `dimension_group_count` | int | 本轮覆盖的 dimension 级资源组数（按服务枚举 + 资源实际维度发现；顶层为各 scope 求和） |
| `group_coverage_mismatches` | string[] | dimension 组覆盖对账不符的组名（声明 total 大于实际发现且差值不可解释）；非空时 `scope_incomplete=true`，该 scope 禁止任何 stale |
| `scope_incomplete` | bool | 聚合口径下有资源组未被覆盖（含 dimension 组对账不符）；该 scope 禁止 stale 标记 |
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
| `enrichment_failed_count` | int | 增强失败类型数；恒等于去重后的 `enrichment_failed_types` 长度 |
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
| `resource_group_selection` | string | 资源组选择方式（`specified_id`/`specified_name`/`default_name`/`all_groups`） |
| `resource_group_count` | int | 该 scope 实际覆盖的资源组数量（聚合口径下 > 1） |
| `dimension_group_count` | int | 该 scope 覆盖的 dimension 级资源组数 |
| `group_coverage_mismatches` | string[] | 该 scope 内 dimension 组覆盖对账不符的组名；非空时 `scope_incomplete=true` |
| `scope_incomplete` | bool | 聚合口径下有资源组未被覆盖；该 scope 禁止 stale 标记 |
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

**stale 标记语义**：`ces`/`hybrid` 模式下 `resource_level=product` 且 `product_names` 非空的资源组是权威 scope，采用反向 stale 标记——把 account+region 下所有 `active` 的 cloud_sync 资源（排除当前批次）标记为 `stale`，但跳过不确定类型（查询失败/转换失败/持久化失败）。这样从资源组移除的类型其旧资产也会被标记 stale，避免永久保持 `active`。`resource_level=dimension` 的资源组（按服务枚举 + 资源实际维度发现）**不是权威 scope**，显式指定时只走逐类型 stale，不做区域级反向标记；聚合口径（`all_groups`）一律只走逐类型 stale。`native`/通用/fake 路径 scope 非权威，仅对查询成功的类型逐类型标记 stale。`product_names` 为空时回落兜底白名单（不完整），批次至少标记 `partial`，message 含 `product_names_empty=true`。`scope_incomplete=true`（资源组被跳过、层级未知或 dimension 组覆盖对账不符 `group_coverage_mismatches` 非空）时该 region 禁止任何 stale 标记。

**partial 判定规则与计数不变式**：批次终态为 `partial` 的条件（满足任一即 partial）：`failed_count > 0`、`max_resources_reached`、`product_names_empty`，或任一 scope 出现 `enrichment_failed_count > 0`/`enrichment_failed_types` 非空/`enrichment_stage_error` 非空/`writeback_failed_count > 0`/`invalid_resource_count > 0`/`conversion_failed_types` 非空/`query_failed_types` 非空。`enrichment_warnings` 非空不触发 partial。计数不变式：`enrichment_failed_count == len(enrichment_failed_types)` 始终成立；`enrichment_stage_error` 与 `writeback_failed_count` 驱动 partial 但不递增 `enrichment_failed_count`，保持该不变式不被破坏；`enrichment_warnings` 独立于 `enrichment_failed_types`，不参与计数与 partial 判定。

**账号级并发互斥**：同一 `integration_account_id` 同一时刻仅允许一个 `running` 批次（迁移 `0028` 部分唯一索引 `(integration_account_id) WHERE status='running'`）。若已有 `running` 批次，`POST /api/assets/sync` 返回 `409 ALREADY_EXISTS`，`message` 为 `sync already in progress for this account`，前端应提示「该账号正在同步，请稍后重试」。

**异步生命周期与租约续租**：同步在后台 goroutine 执行（`runCtx` 派生自进程级 `shutdownCtx` + 30 分钟硬超时），与 HTTP 请求生命周期解耦。`running` 批次写入 `lease_expires_at`（TTL 5 分钟），后台 goroutine 每 60 秒续租；终态写入与审计使用独立短 context（10 秒），保证取消后仍尽可能落终态。同账号 running 互斥与 fencing 防止并发批次交错污染 stale。进程崩溃遗留的 running 批次当前由**下一次同账号 TriggerSync** 在插入前 reap；没有装配周期后台 reaper，因此无人再次触发时不保证自动自愈。migration 0033 增加了 `triggered_by` 列，但当前 TriggerSync 未写入该字段，也没有独立的 `sync_started` 即时审计；reap 与终态审计统一使用 `action=sync`，reap actor 取当次请求用户。

#### 5.5.2 同步批次列表

- Method: `GET`
- Path: `/api/assets/sync/batches?page=1&page_size=20&account_id=acc_xxx`
- Auth: Bearer + `app:assets:read`

> **当前数据范围边界**：批次列表只按 `account_id` 和分页参数查询，当前没有按 `owner_team` 过滤；批次详情也只按 `batch_id` 读取，没有契约早期所述的完整数据范围校验。`app:assets:read` 仍是当前路由准入条件。若后续补齐过滤，需分别覆盖无范围、all、team、region-only 和单批次详情，不能只修改仓储 SQL 而遗漏 actor/user_id 传递。

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

审计：当前 `resource_type=asset_sync_batch`、`action=sync`，终态 payload 含 status、基础计数和部分同步摘要；内联 reap 额外带 `event=reap_expired_running`。`triggered_by` 运行时归因、三个独立 action、完整嵌套 summary/scopes 和周期 reaper 尚未按早期契约落地。

#### 5.5.4 资产注册表列表分页

应用、资源、匹配规则三个列表接口统一采用标准分页（与 §5.5.2 同一 `pagination.PageData` 形态）。`page` 从 1 开始，默认 `page_size=20`，最大 `page_size=100`；HTTP 层通过 `pagination.Query.Normalize` 修正，application 层也会兜底归一化，避免非 HTTP 调用绕过分页上限。

资源列表支持服务端筛选与排序；前端不得做跨页本地假筛选或假排序（列表只返回当前页）。

| 参数 | 语义 | 匹配方式 |
| --- | --- | --- |
| `cloud_resource_type` / `region` / `sync_status` | 云类型 / Region / 同步状态 | 精确匹配，空值不生效 |
| `source` | `manual` / `cloud_sync` | 精确匹配，空值不生效 |
| `keyword` | 资源名、资源类型、namespace、pod、instance、云账号、云资源 ID、云类型、Region | `ILIKE '%keyword%'` 跨字段模糊匹配；输入中的 `%` / `_` / `\` 转义为字面量 |
| `sort_by` | 排序字段，取值见下方白名单 | 未命中白名单时回退默认排序 |
| `sort_order` | `asc` / `desc`（兼容 `descend` / `descending`） | 非法值按 `asc` 处理 |

排序白名单：`name`、`resource_type`、`source`、`cloud_resource_type`、`integration_account_id`、`cloud_resource_id`、`region`、`sync_status`、`last_synced_at`、`namespace`、`pod`、`instance`、`sync_batch_id`、`created_at`、`updated_at`。默认 `created_at ASC, id ASC`；指定排序时固定追加 `id ASC` 作为稳定 tie-breaker。`ORDER BY` 只作用于取数查询，不进入 `COUNT(*)`。表头排序由 `sorterChange` 触发一次带 `sort_by` / `sort_order` 的重新查询，并重置到第 1 页。

| 接口 | Method | Path | Auth | 排序 |
| --- | --- | --- | --- | --- |
| 应用列表 | `GET` | `/api/assets/applications?page=1&page_size=10` | Bearer + `app:assets:read` | `created_at ASC, id ASC` |
| 资源列表 | `GET` | `/api/assets/applications/:application_id/resources?page=1&page_size=10&cloud_resource_type=ecs&region=cn-north-4&sync_status=active&keyword=elb&source=cloud_sync&sort_by=name&sort_order=asc` | Bearer + `app:assets:read` | 默认 `created_at ASC, id ASC`，可按 `sort_by` / `sort_order` 覆盖 |
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

#### 5.5.4.1 应用、资源和匹配规则写接口

以下写接口均要求 Bearer + `app:assets:write`：

| 接口 | Method/Path | 关键字段与语义 |
| --- | --- | --- |
| 创建应用 | `POST /api/assets/applications` | `id` 可选，`name` 必填；`environment/namespace/description` 可选 |
| 更新应用 | `PUT /api/assets/applications/:id` | 按业务 ID 更新名称、环境、namespace 和描述 |
| 删除应用 | `DELETE /api/assets/applications/:id` | 资源、匹配规则、未关闭告警或 Inspection Policy 引用按删除保护处理 |
| 创建资源 | `POST /api/assets/resources` | `application_id` 必填；`name/resource_type/namespace/pod/node/instance` 按注册表写入，云同步字段只读 |
| 更新资源 | `PUT /api/assets/resources/:id` | 更新注册表基础字段，不能借该接口伪造云同步归属 |
| 删除资源 | `DELETE /api/assets/resources/:id` | 存在匹配规则或资源关系时返回 `FAILED_PRECONDITION` |
| 创建匹配规则 | `POST /api/assets/match-rules` | `name/label_key/label_value_pattern/application_id` 必填；支持 priority、source_type、target_type、resource_id |
| 更新匹配规则 | `PUT /api/assets/match-rules/:id` | 按业务 ID 更新规则与 enabled 状态 |
| 删除匹配规则 | `DELETE /api/assets/match-rules/:id` | 删除后后续告警回退默认匹配 |

上述 `:id` 都是业务 ID 路径参数，不是数据库自增 ID。创建/更新/删除写 Operation Audit，审计失败不回滚主业务。应用删除还需遵循跨上下文引用完整性：未关闭告警阻断，已关闭告警可在事务内解除引用，Inspection Policy 引用阻断。

#### 5.5.5 资源筛选字典

列表接口不是字典接口（§5.5.4），资源页的云类型 / Region 下拉使用独立字典接口：

| 接口 | Method | Path | Auth |
| --- | --- | --- | --- |
| 资源筛选字典 | `GET` | `/api/assets/applications/:application_id/resource-facets` | Bearer + `app:assets:read` |

响应 `data`：

```json
{
  "cloud_resource_types": ["ecs", "elb"],
  "regions": ["cn-east-3", "cn-north-4"]
}
```

- 只统计该 `application_id` 下实际出现的非空值，按字典序升序去重；无数据时返回空数组，不返回 `null`。
- 字典仅作为下拉候选值，最终过滤仍必须通过 §5.5.4 的 `cloud_resource_type` / `region` 精确匹配参数下传，禁止前端用字典做本地筛选。
- `application_id` 为空白时返回 `INVALID_ARGUMENT`。

#### 5.5.6 CMDB 资源拓扑与手工关系（migration 0058）

CMDB 拓扑描述应用归属和资源配置关系，与 §5.4 的 Observability 运行时调用拓扑是不同领域模型，不复用 DTO、权限或 fake Provider 数据。

| 接口 | Method | Path | Auth |
| --- | --- | --- | --- |
| 查询应用资源拓扑 | `GET` | `/api/assets/topology?application_id=app-1` | Bearer + `app:assets:read` |
| 查询手工关系 | `GET` | `/api/assets/resource-relations?application_id=app-1` | Bearer + `app:assets:read` |
| 解析云资源 | `GET` | `/api/assets/resources/resolve?cloud_resource_id=rds-1&account_id=acc-1&region=cn-south-1&cloud_resource_type=rds` | Bearer + `app:assets:read` |
| 创建手工关系 | `POST` | `/api/assets/resource-relations` | Bearer + `app:assets:write` |
| 删除手工关系 | `DELETE` | `/api/assets/resource-relations/:relation_id` | Bearer + `app:assets:write` |

拓扑查询必须指定 `application_id` 或 `root_resource_id` 之一。`region` / `cloud_resource_type` 精确过滤；`keyword` 匹配资源业务 ID、名称、云资源 ID 和资源类型；`relation_type` 过滤手工边和云派生边，不影响 `contains`；`include_stale=true` 时才包含 stale 资源，默认 false。

根资源查询使用 `root_resource_id=<业务 ID>&depth=2&direction=both`：`depth` 默认 2、允许 1–3；`direction` 支持 `both` / `upstream` / `downstream`，分别表示双向、仅上游、仅下游。根节点始终保留，其他节点按层级和筛选条件扩展，支持跨应用链路。

- 应用→资源的 `contains` 边由 `asset_resource.application_id` 实时派生，`origin=registry`，不重复落库。
- 手工关系持久化于 `asset_resource_relation`，`origin=manual`，关系类型仅支持 `depends_on` / `attached_to` / `located_in` / `routes_to`。
- 创建时两端资源必须存在且不能相同，允许跨应用；`(source_resource_id, target_resource_id, relation_type)` 唯一，重复返回 `ALREADY_EXISTS`。响应同时返回两端 `source_application_id` / `target_application_id`。
- 云派生边 `origin=cloud_sync` 仅在查询时由已验证字段生成，不伪造持久化关系：ECS/RDS/DCS/DMS/Subnet/ELB 的 `vpc_id`、RDS 的 `subnet_id`、EVS 的 `attached_to`、EIP 的 `bandwidth_id`、Peering 的 `request_vpc_id` / `accept_vpc_id`。来源账号、Region、云资源类型和云资源 ID 必须唯一匹配；不跨账号/Region 猜边。
- 无法唯一解析的云关系端点不创建“幽灵节点”，计入 `meta.unresolved_relation_count`；无该字段的既有空实现按 0 处理。
- 资源仍存在源端或目标端关系时，删除资源返回 `FAILED_PRECONDITION`，必须先删除关系。
- 关系创建/删除写 `resource_type=resource_relation`、`action=create/delete` 审计，payload 包含两端业务 ID 与关系类型，不记录敏感 labels。
- 单次最多返回 299 个资源节点及其关联应用节点；超出时 `meta.truncated=true`，调用方必须继续用筛选或降低深度缩小范围，不得把结果描述为完整拓扑。

云资源解析用于把告警中的云 ID 转换为 CMDB 真实资源：`cloud_resource_id` 与 `account_id` 必填，`region` / `cloud_resource_type` 可选但必须精确匹配。响应 `data.status`：

- `matched`：唯一候选，`candidates[0]` 包含资源业务 ID、真实名称、应用业务 ID 和名称、云类型、Region 与同步状态，可跳转资产页。
- `unregistered`：无匹配，返回空数组，前端只能显示原始云 ID 与推断资源类型，不能编造资源名。
- `ambiguous`：多个候选，返回全部候选，前端提示需要确认，不能默认选择第一项。

解析接口只读，不创建应用或资源，不返回凭据，也不把其他账号或 Region 的同名云 ID 当作匹配。

拓扑响应 `data`：

```json
{
  "nodes": [
    { "node_id": "application:app-1", "entity_type": "application", "entity_id": "app-1", "name": "payment" },
    { "node_id": "resource:res-1", "entity_type": "resource", "entity_id": "res-1", "name": "gateway", "application_id": "app-1" }
  ],
  "edges": [
    { "edge_id": "contains:res-1", "source_node_id": "application:app-1", "target_node_id": "resource:res-1", "relation_type": "contains", "origin": "registry" }
  ],
  "meta": { "generated_at": 1760000000, "truncated": false, "node_count": 2, "edge_count": 1, "unresolved_relation_count": 0 }
}
```

创建关系请求：

```json
{
  "source_resource_id": "res-1",
  "target_resource_id": "res-2",
  "relation_type": "depends_on"
}
```

关系列表响应 `data` 为 `{ "items": [], "total": 0 }`；单项包含 `relation_id`、两端资源业务 ID、显示名和应用 ID、`relation_type`、`origin`、`created_at`、`updated_at`。跨应用关系在源应用和目标应用的关系列表中均可出现。删除成功返回 `{ "deleted": true }`。

前端独立路由为 `/assets/topology?application_id=app-1`：默认以根资源展示上下游分层链路，支持完整路径高亮、1–3 层扩展和循环依赖保护；“全景”使用力导向网络展示应用归属与资源关系；“列表”提供可访问性降级。节点详情可切换为新的根资源，手工关系表单可分别选择源资源和目标应用/资源。观测页对应入口明确命名为“调用拓扑”。

## 6. Inspection API

当前提供策略 CRUD、同步手动运行、Finding/Recommendation 和后端建议转执行接口。`EvidenceAnalyzer` 是固定阈值规则实现，不是 AI Agent；`agent_profile` 只是元数据。运行在同一 HTTP 请求内完成并通常返回 success/partial/failed 终态。

**自动化边界**：schedule 和 `trigger_type=scheduled` 已保存/定义，但没有定时 worker；cancelled 只在状态机中存在，没有取消 API；Notification、Recommendation accepted/dismissed 和 Execution 完成回写未落地。

**真实数据边界**：当前采集固定最近 1 小时、region=cn-north-4、Metrics namespace=SYS.ECS 且无 dimensions，真实华为 CES 指标会因此失败；真实 logs/traces 也不支持。现有可重复主路径主要是 `huawei_cloud + none` fake，不能笼统写成真实 Provider 巡检已完成。

### 6.0 策略 scope 与权限

| 字段 | 类型 | 必填 | 当前语义 |
| --- | --- | --- | --- |
| `account_id` | string | 是 | Integration 账号业务 ID；创建时只校验非空，运行时再由 Observability 校验存在/启用 |
| `provider` | string | 否 | 默认取账号 provider |
| `environment` | string | 否 | 仅保存，当前不参与证据查询 |
| `application_ids` | string[] | 否 | 去空/去重并校验注册表存在；仅第一个 ID 作为 service 字符串 |
| `resource_types` | string[] | 否 | 仅保存，当前不参与证据查询 |

所有 Inspection 路由使用静态 RBAC，不按 scope 做对象级数据范围过滤。`app:inspections:read/write` 默认由 0020 绑定 admin；建议转执行只要求 `app:executions:create`，不额外要求 inspections:write。

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
    "account_id": "acc_xxx",
    "provider": "huawei_cloud",
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

name 和至少一个合法 checks 必填，enabled 默认 true。agent_profile 默认 `sre_default` 但不驱动 AI；notification_policy_id 仅保存；schedule 没有 worker 消费。更新为部分字段语义，checks 省略保留、空数组拒绝。删除为软删除并清空 application_ids，历史 Run/Finding/Recommendation 保留。

其他管理接口：

| 方法 | 完整路径 | 权限 |
| --- | --- | --- |
| `GET` | `/api/inspections/policies` | `app:inspections:read` |
| `GET` | `/api/inspections/policies/:policy_id` | `app:inspections:read` |
| `PUT` | `/api/inspections/policies/:policy_id` | `app:inspections:write` |
| `DELETE` | `/api/inspections/policies/:policy_id` | `app:inspections:write` |

列表支持 page/page_size、enabled 和 name keyword，统一最大 page_size=100。

### 6.2 手动触发巡检

- Method: `POST`
- Path: `/api/inspections/policies/:policy_id/runs`
- Auth: Bearer + `app:inspections:write`

响应：

```json
{
  "run_id": "run-<uuid>",
  "policy_id": "pol-<uuid>",
  "status": "success",
  "trigger_type": "manual",
  "summary": "completed with 1 findings",
  "timeline": [],
  "created_at": 1710000000,
  "updated_at": 1710000000
}
```

该接口同步执行采集和分析，通常直接返回终态，不是异步 pending 任务。全部证据失败时也可在成功落库后返回 HTTP 200 + `status=failed`；调用方必须检查 run.status。disabled 策略返回 412。

### 6.3 查询巡检运行

- Method: `GET`
- Path: `/api/inspections/runs/:run_id`
- Auth: Bearer + `app:inspections:read`

状态机：

```text
pending -> running -> success|partial|failed|cancelled
```

`GET /api/inspections/runs` 支持 page/page_size、policy_id 和 status 筛选。success 表示所有计划检查均获得证据，可以有 0 个 Finding；partial 表示部分证据失败但至少有成功项；全部证据失败为 failed。cancelled 没有当前调用点。

Findings、Recommendations 和最终 Run 同事务保存；Run 创建、running 更新和 best-effort 审计不在该事务内。分析/保存失败时会尽量另写 failed 终态并返回错误。

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

列表支持 page/page_size、run_id、policy_id 和 risk_level。confidence/uncertainty 来自固定规则，不是模型评分。当前 affected_resources 固定为 `service/payment-service`，没有按 application_ids 展开真实资源；environment/resource_types 不参与采集。

规则阈值：CPU >=80 high、60–80 medium；内存 >=75 high；磁盘 >=70 medium；有错误 Span 为 high；有高延迟样本为 medium；error 日志摘要 >=2 为 medium。可执行建议对应 high/critical Finding 时，Recommendation 风险降为 medium。

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

当前要求与边界：

- Recommendation 必须 `can_create_execution=true`；已 execution_created 返回 409，不幂等返回原任务。
- 只能创建 Agent 模式 Execution Task，不能直接执行；execution_mode 省略默认 agent，其他模式返回 400。
- `medium_id` 在 HTTP 层必填，**没有自动介体选择**。
- `confirm_intent` 省略或必须为 create_task_only；medium/high/critical 进入 pending_confirm，low 按 Execution 规则进入 pending_execute。
- command_spec_id 省略时按标题选择默认只读 Spec，未命中兜底 `cmd_linux_disk_usage`；磁盘空 arguments 默认 mount_point=/。调用方必须复核默认值，最终 Spec 和参数 schema 由 Execution 校验。
- Adapter 固定 source_type=inspection、source_id=recommendation_id、operation_type=command；后续确认、lease、日志和结果由 Execution 负责。
- Recommendation 只更新 `execution_created`，不保存 task_id 或独立时间线；task_id 仅在本次响应和 create_execution 审计中返回。
- Execution 创建与 Recommendation 状态更新不在同一事务；状态更新失败时任务可能已存在，重试前需核对 Execution/审计，避免重复创建。
- accepted/dismissed、通知和 Execution 完成回写未落地，前端也无转执行入口。

执行介体、执行代理、Command Spec、租约和日志回传契约见 `execution-agent-contract.md`。

## 8. 审计动作

| resource_type | action |
| --- | --- |
| `integration_account` | `create` / `update` / `delete` / `check` |
| `asset_sync_batch` | 当前统一为 `sync`；payload 以 status 或 `event=reap_expired_running` 区分阶段。早期目标 `sync_started` / `sync_finished` / `sync_reaped` 尚未落地 |
| `observability_query` | `metrics_query` / `logs_search` / `traces_query` / `topology_get`；内部资源和规则采集另有 `resources_list` / `alerts_list` |
| `inspection_policy` | `create` / `update` / `enable` / `disable` / `delete` |
| `inspection_run` | 当前实际写入 `create` / `start` / `finish`；cancel 仅保留常量，无调用点 |
| `inspection_recommendation` | 当前实际写入 `create` / `create_execution`；notify 未实现 |
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
| `scripts/e2e-integration.ps1` | 使用 `huawei_cloud + none` 覆盖创建、列表/详情、检查 ok、更新、viewer 创建 403、删除 | 已落地；不覆盖真实凭据/网络、其他 auth_type、检查 failed、keyword、审计或完整 UI |
| `scripts/e2e-observability.ps1` | `huawei_cloud + none` 的指标/日志/链路/拓扑 fake 查询与 evidence_id | 已落地；不验证真实 CES、权限负向、审计或证据详情 |
| `scripts/e2e-ai-evidence.ps1` | fake 指标 + 本地 mock 模型 + Docker PostgreSQL，比对模型输入与定稿 payload 的规范化 SHA-256 | 已落地；不是实际 CES 验收，不覆盖公共证据详情 API/UI |
| `scripts/e2e-asset-sync.ps1` | 云资源同步、批次、stale 标记、cloud_resource_id 入库 | 已落地 |
| `scripts/e2e-inspection.ps1` | fake 账号下创建应用/账号/策略、同步运行、Run 详情、Findings/Recommendations 和删除清理 | 已落地；不覆盖更新/列表、partial/failed、审计、真实云、转 Execution 或 UI |
| `scripts/e2e-notification.ps1` | 通知通道、发送记录、失败重试 | 待补（通知模块未落地） |
| `scripts/e2e-execution-agent.ps1` | 直接创建 Medium 和 manual Agent 任务，注册/心跳、确认前后 lease、日志、结果及非法参数 | 已落地；不经过 Inspection Recommendation，不覆盖真实命令、审计或完整幂等 |

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

密钥版本字节只是当前密文格式标识。当前运行时只接受配置的当前版本，没有多版本解密和在线重加密流程；执行轮换前必须另行设计旧凭据可读与重加密方案，不能只修改 key/version 后假定历史凭据仍可使用。
