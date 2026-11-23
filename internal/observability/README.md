# Observability 限界上下文

统一指标、日志、链路、拓扑、资源发现与告警规则只读查询。第一阶段通过 **Provider Port + fake adapter** 跑通 DDD 边界、API、权限与审计，不依赖真实华为云 SDK。

## 目录

```text
internal/observability/
  domain/           查询模型与结果对象
  application/      QueryService + Provider Port 定义
  infrastructure/
    provider/       ProviderRegistry + 厂商 Adapter（huawei/signoz/prometheus）
    integration/    Integration 账号解析适配
    persistence/    obs_evidence_ref
    audit/
  interfaces/http/  /api/observability/*
```

## Application 层 Provider Port

| Port | 职责 | Agent 工具映射 |
| --- | --- | --- |
| `MetricQueryPort` | 指标时序查询 | `cloud.metrics.query` |
| `LogSearchPort` | 日志搜索（脱敏摘要） | `cloud.logs.search` |
| `TraceQueryPort` | 链路 Span 查询 | `cloud.traces.query` |
| `TopologyQueryPort` | 服务拓扑快照（需 `topology` 能力） | `cloud.topology.get` |
| `AssetDiscoveryPort` | 云资源只读发现（返回 `resources, hasMore, err`；`hasMore=true` 表示因达到查询上限而截断，Asset Sync 通用路径据此跳过该类型 stale） | `cloud.resources.list` |
| `AlertRuleQueryPort` | 告警规则只读列表（含 `unit`、`resource_count`、前 3 个资源维度样本） | `alert.rules.list` / `alert.rules.health` |

`ProviderEntry` 仅标识 provider 类型；`ProviderRegistry` 按 `provider` 路由，`QueryService` 按需断言上述小 Port（Prometheus 等 partial provider 无需实现全部方法）。

## 依赖方向

```text
interfaces/http -> application -> domain <- infrastructure
                      |
                      +-> IntegrationAccountPort (adapter -> integration repos)
```

## 调用关系（粤语补充）

```text
/api/observability/*
  -> interfaces/http Handler
  -> application.QueryService
  -> IntegrationAccountPort 解析账号摘要
  -> capability 校验
  -> ProviderRegistry 拎对应 ProviderEntry
  -> MetricQueryPort / LogSearchPort / TraceQueryPort / TopologyQueryPort
  -> obs_evidence_ref 保存证据引用；AI CES 路径在定稿后补存 payload 快照
  -> audit recorder 写 observability_query
```

Observability 只做统一查询同证据引用，唔持有凭据明文，亦唔将华为云、SigNoz、Prometheus 嘅差异带入 domain。真实 provider adapter 可以逐个能力补齐；只要 application Port 唔变，Inspection、AI 工具同前端都唔需要跟住厂商 SDK 改。

migration `0060` 起，`obs_evidence_ref` 同时支持 summary-only 历史证据与 `ai.metric_evidence.v1` 定稿快照。AI CES 查询先写 `collecting`，裁剪/降采样完成后保存 `payload`、`payload_hash`、`query_params`、业务窗口、采集时间、资源标识、状态及截断计数，再把同一份结果交给模型；写快照失败时 AI 侧降级，不将未持久化指标作为事实。普通 Observability / Inspection 查询仍保持 summary-only。证据详情 API、权限校验入口与前端抽屉属于后续 P0-2，本切片不新增对外读取接口。

AI 规则体检复用本模块能力：`ListAlertRules` 返回 `unit/resource_count/resources`，`alert.rules.health` 按规则关联资源批量取历史曲线并计算峰值/P95/破限比例；`metrics.query` 超过 7 天的窗口最长 30 天，但 `period` 不得小于 3600（小时级），仍受 1440 点上限约束（`ops/ai-contract.md` §10.12.1，migration `0062`）。

跨上下文关系：

| 调用方 | 被调能力 | 说明 |
| --- | --- | --- |
| 前端 `observability.ts` | 查询指标/日志/链路/拓扑 | 用 `app:observability:read` 控制 |
| Inspection | `ObservabilityQueryPort` | 收集巡检证据，结果用 `evidence_id` 串联 |
| AI 工具网关 | `metrics.query` / `alert.rules.list` / `alert.rules.health` 等只读工具 | 继承用户身份与审计；规则体检按账号数据范围二次校验 |
| Integration | 账号摘要来源 | 只提供账号、provider、capability、credential ref |

## API 与权限

契约：`ops/cloud-observability-contract.md` §5。

| 权限码 | 说明 |
| --- | --- |
| `app:observability:read` | 指标/日志/链路/拓扑查询 |

## 迁移

- `0019_init_observability.up.sql`：`obs_evidence_ref` + `app:observability:read` 种子
- `0060_obs_evidence_snapshot.up.sql`：CES 定稿快照、hash、查询参数、窗口、采集状态与截断元数据
- `0061_postgres_history_tool.up.sql`：RDS for PostgreSQL 历史会话只读工具权限；`rds_history_client.go` 封装历史会话、等待事件和 Top SQL 三接口，AI 侧复用 Integration AK/SK 与账号数据范围，快照 `evidence_type=postgres_activity`。
- `0062_ai_rule_health_tool_permission.up.sql`：批量规则体检只读工具 `alert.rules.health` 权限种子并绑定 admin；规则查询补齐单位与资源范围元数据，指标长窗口允许 30 天小时级聚合。

## 测试

```powershell
go test ./internal/observability/...
```
