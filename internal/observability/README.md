# internal/observability 模块说明

Observability 提供统一只读观测查询、内部资源/告警采集 Port 和证据账本。它通过 `IntegrationAccountPort` 读取启用账号摘要，通过 ProviderRegistry 路由厂商 Adapter，不让厂商 SDK 进入 domain。稳定契约见 `ops/cloud-observability-contract.md` §5；AI 定稿证据的消费语义另见 `ops/ai-contract.md`。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 公共查询 | 指标、日志、链路、运行时调用拓扑四个路由 | 统一 `app:observability:read` 静态 RBAC；无对象级数据范围过滤 |
| 华为真实查询 | AK/SK 的 CES 指标、资源发现/全量同步、告警规则及 CES 告警拉取 | 真实日志/链路/拓扑未实现；agency 不降级为 fake |
| fake 查询 | 华为 none 的全部能力；SigNoz 和 Prometheus 当前 Adapter | 返回确定性样本，不是外部平台数据，不能用于真实验收 |
| 内部 Port | 资源发现/全量同步、告警规则、CES 告警拉取、RDS 历史会话客户端 | 没有对应的独立公共 HTTP 路由 |
| 证据引用 | 查询 ID、query hash、脱敏 summary；指标另有查询参数与窗口 | 普通指标没有定稿 payload；其他普通查询以摘要为主 |
| AI 定稿快照 | `ai.metric_evidence.v1` 与 `ai.postgres_activity.v1` payload/hash | 没有公共证据详情 API 或前端证据抽屉 |
| 前端 | 四类查询表单、结果表格、证据 ID、指标 AI 上下文 | 指标只展示首条序列；无快照内容展示和完整页面测试 |

## 2. 公共 HTTP 路由与权限

| 方法与路径 | 主要输入 |
| --- | --- |
| `POST /api/observability/metrics/query` | account_id、可选 provider、region/namespace/metric/dimensions、from/to、period/aggregator |
| `POST /api/observability/logs/search` | account_id、可选 provider、service/resource_id/keyword/trace_id、from/to/limit |
| `POST /api/observability/traces/query` | account_id、可选 provider、service/operation/trace_id、error_only/min_latency_ms、from/to/limit |
| `GET /api/observability/topology` | 必填 account_id/from/to，可选 provider/application_id |

所有路由要求 Bearer + `app:observability:read`，migration `0019` 默认绑定 admin。`provider` 省略时使用账号 provider，显式值与账号不一致返回 `INVALID_ARGUMENT`。

当前公共指标 Handler **没有 `project_id` 字段**，`web/src/api/observability.ts` 也未暴露该字段。多项目公共查询依靠账号顶层 project_id 或 `extra_config.region_projects` 解析；内部 `domain.MetricQuery` 和 AI 路径才支持显式 project_id。早期契约把 project_id 放进公共请求示例属于实现与文档差距，不能据此认定公共 API 已支持逐请求覆盖。

AccountAdapter 会拒绝已删除账号（404）和 disabled 账号（412），再检查默认/已存 capabilities。HTTP 中间件只做静态 RBAC，不根据 owner_team/region 做数据范围校验。AI 指标采集、资源读取等专用链路另有账号或对象级授权，不能把那些校验反推到公共 Observability 查询。

## 3. Provider 支持矩阵

| Provider / auth | metrics | logs / traces / topology | assets / alerts | 数据性质 |
| --- | --- | --- | --- | --- |
| `huawei_cloud + none` | fake | fake | fake | 确定性联调样本 |
| `huawei_cloud + ak_sk` | 真实 CES `ShowMetricData` | `FAILED_PRECONDITION`，不返回 fake | 真实资源发现/同步与 CES 规则；告警拉取另有 Alert 链路 | 真实云只读 Adapter，按能力分别验收 |
| `huawei_cloud + agency` | unsupported | unsupported | unsupported | 不降级为 fake |
| `signoz`（含 api_token） | fake | fake | fake 告警规则；无 assets Port | 当前未连接真实 SigNoz |
| `prometheus`（含 api_token/ak_sk） | fake | 无对应 Port；默认 capabilities 也不声明 | fake 告警规则；无 assets Port | 当前未连接真实 Prometheus |

capabilities 只是 Integration 默认声明和 QueryService 的第一道门控，不代表 Adapter 已实现。即使华为 AK/SK 默认声明 logs/traces/topology/alerts，真实 Adapter 仍按上表逐项放行或拒绝。

fake 日志不会回显 keyword 原文，而是带 hash/len；fake Trace/Topology 也是构造样本。真实凭据下不支持的能力会显式失败，不会悄悄切换 fake。云错误会映射为参数、not found、rate limit 或 unavailable 等平台错误，不向客户端透传凭据、请求头或原始云端响应。

## 4. 查询归一化与厂商限制

公共 QueryService 统一执行：

```text
输入归一化
  -> 启用账号与 provider/capability 校验
  -> Provider 小 Port 类型断言与调用
  -> obs_evidence_ref
  -> observability_query 审计（best-effort）
```

主要限制：

- Metrics：from/to 必须有效；period 默认 60、平台最小 10；公共窗口最长 7 天，按 window/period 估算最多 1440 点。AI 内部路径可显式 AllowLongWindow，最长 30 天且 period 至少 3600。
- Logs：limit 默认 100、最大 500。
- Traces：limit 默认 50、最大 1000。
- 内部 resources/alert rules：limit 默认 100、最大 500。

华为真实指标还有独立约束：region 必须合法且属于账号配置，namespace 必填，dimensions 为 1–4 个，最终 project_id 必须可解析。CES SDK 只接受其 period/filter 枚举；当前服务实际可用的 period 为 60/300/1200/3600/14400/86400，filter 支持 average/avg/max/min/sum/variance/var。平台通用的“period>=10”并不保证真实 CES 可用，例如 period=10 会进入通用校验但被真实 CES 拒绝。前端允许 10–3600 的数值输入，因此真实账号仍需按 CES 枚举选择。

指标 unit 优先使用 Provider 返回，缺失时只按已登记的官方指标映射推断，未登记保持空，不按名称后缀猜测。

## 5. 内部采集 Port 不等于公共 API

`QueryService` 和 Observability infrastructure 还提供：

- `AssetDiscoveryPort` / `CloudFullSyncPort` / `CloudEnrichmentPort`：供 Asset Sync 与 AI 资源工具使用；交互 resources/alerts limit 上限与全量同步配额不同。
- `AlertRuleQueryPort`：供 AI 规则列表和批量规则体检使用，包含单位、关联资源计数和最多 3 个资源维度样本。
- `AlarmRecordPullPort`：供 Alert CES V2 拉取使用。
- `rds_history_client.go`：供 AI RDS for PostgreSQL 历史会话、等待事件和 Top SQL 采集使用，查询由 AI 侧数据范围与实例条件约束。

这些能力没有 `/api/observability/resources`、`/api/observability/alerts`、`/api/observability/evidence/:id` 等公共路由。CMDB `/assets/topology` 也是独立注册表拓扑，不复用本模块 fake 运行时调用拓扑。

## 6. 普通证据引用

每次成功查询生成 `ev-<uuid>`，在生产仓储已配置时先保存证据，再写审计并返回结果。证据保存失败会终止该查询响应；审计失败则被忽略。测试或未装配仓储的调用可只生成 ID 而不持久化，不能用这种装配证明生产已落库。

普通证据包含：

- evidence_id、account_id、query_type/evidence_type、query_hash、脱敏 summary、status=ok。
- Metrics 额外包含 trace_id、provider、实际生效的 query_params、资源标识、业务窗口和 collected_at。
- 普通 Metrics 的 payload 仍为空；它没有经过 AI 定稿、Prompt 大小裁剪和模型输入 hash 校验。
- Logs 的 summary 不保存 keyword 原文，只保存 has_keyword、keyword_len 和完整 SHA-256 keyword_hash。
- 历史 0019 证据保持 summary-only，0060 不会为旧行伪造 payload 或窗口。

查询响应中的 series/entries/spans/topology 是本次 API DTO，证据表中的 summary 是审计摘要，AI payload 是第三种“定稿模型输入”，三者不能混为一谈。

## 7. AI 指标定稿快照

AI 指标采集使用 `SnapshotRequired=true`：

```text
QueryMetrics 成功
  -> 初始 EvidenceRef(status=collecting, collector=ai.metric_evidence.v1, payload=NULL)
  -> AI 计算统计并定稿
  -> 可选每序列降采样与总字节裁剪
  -> SaveSnapshot 校验并保存 payload/payload_hash/status/schema/truncation
  -> 同一份定稿结果才进入模型输入
```

当前对话路径每序列最多 288 点，策略为 `stride_first_last`，首末点保留；first/peak/peak_at/last/breach 片段与越限时长等统计在降采样前基于完整序列计算，裁剪后仍保留。`original_count` 是裁剪前总点数，`retained_count` 是定稿后总点数，存在于 payload/summary；数据库独立列只有 original_count、truncated 等，没有 retained_count 列。

单条 payload 默认受 32KiB 配置上限约束：超限时先清空 points 保留统计，再超限时清空 metrics 保留状态与查询元数据，并标记 truncated。若连元数据也超限，则返回快照错误。

`payload_hash` 是定稿 payload 经规范化 JSON/PostgreSQL JSONB 后的 SHA-256，不是云端原始响应 hash。SaveSnapshot 会验证调用方 hash，并按数据库规范化结果重算，保证模型输入与落库内容可比对。

失败策略为 fail-closed：

- 快照存储未装配、编码失败或保存失败时，OK 证据转为 `status=error` 和稳定 reason，清空 metrics、retained_count、evidence_id，不进入支持性 references。
- 已进入 collector 的 skipped/permission_denied/unsupported/error 会尽力保存失败快照供追踪，但失败保存后清除 ID；非 OK 证据不能作为模型支持性引用。
- 普通 HTTP 查询不使用这套 Prompt 定稿流程，不能由普通 evidence_id 推断存在 payload。

## 8. RDS for PostgreSQL 快照

`SnapshotStore` 同时实现 PostgresEvidenceStore，保存 `evidence_type=postgres_activity`、`schema_version=ai.postgres_activity.v1` 的定稿 payload，包括 sessions、wait_events、Top SQL 脱敏摘要、查询窗口、采集时间、状态、错误和截断计数。

该路径只允许已解析实例和只读历史接口，不接受自由连接地址或 SQL；数据范围、诊断解释和证据缺口由 AI 模块契约定义。快照保存同样先校验规范化 hash，OK 证据持久化失败会 fail-closed。0061 是 AI 工具权限种子迁移，不是新的证据表。

## 9. 前端页面

页面为 `web/src/views/observability/index.vue`：

- 加载前 100 个 enabled Integration 账号，不显式传 provider，由后端按账号解析。
- 指标选择账号首 region，要求 namespace 和 dimensions；只展示 `series[0].points`，并将资源 ID/时间范围写入 AI 页面上下文。
- Logs 固定 limit=100；Traces 固定 limit=50；Topology 使用独立 application_id 和运行时调用拓扑模型。
- 四类结果只显示 evidence_id 文本，没有证据详情请求、payload 展示或快照抽屉。
- 当前 Vitest 只覆盖指标参数和 AI 上下文，不覆盖日志、链路、拓扑、错误、真实 CES 枚举或多序列。

`e2e-observability.ps1` 使用 `huawei_cloud + none`，只验证四类 fake 查询和 evidence_id。`e2e-ai-evidence.ps1` 也使用 fake 指标，外加本地 mock 模型和 Docker PostgreSQL，验证模型输入与快照规范化 hash 一致；两者都不是实际华为 CES 验收。

## 10. 审计、迁移与验证覆盖

审计统一为 `resource_type=observability_query`、`resource_id=evidence_id`：

- `metrics_query`、`logs_search`、`traces_query`、`topology_get`。
- 内部 `resources_list`、`alerts_list` 也复用该资源类型。
- 审计在证据写入后 best-effort 执行，不包含凭据、完整日志或原始云端响应。

迁移：

- `0019_init_observability`：obs_evidence_ref 基表和 `app:observability:read`。
- `0060_obs_evidence_snapshot`：定稿 payload/hash、查询参数、窗口、采集状态及截断元数据。
- `0061_postgres_history_tool`：RDS 历史会话工具权限。
- `0062_ai_rule_health_tool_permission`：批量规则体检工具权限，无证据表结构变化。

已有测试覆盖查询归一化、capability、fake Provider、华为 mock CES/凭据/错误映射、证据 collecting、定稿快照与 fail-closed、PostgreSQL 证据仓储，以及单个指标前端 Vitest。PostgreSQL 仓储测试在数据库不可用或 short 模式会跳过；华为集成式单测使用 mock CES，不是真实云调用。

本轮仅静态复核，未执行 Go 测试、前端 Vitest 或任何 E2E，不形成运行通过结论。
