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
| `AssetDiscoveryPort` | 云资源只读发现 | `cloud.resources.list` |
| `AlertRuleQueryPort` | 告警规则列表 | `cloud.alerts.list` |

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
  -> obs_evidence_ref 保存证据引用
  -> audit recorder 写 observability_query
```

Observability 只做统一查询同证据引用，唔持有凭据明文，亦唔将华为云、SigNoz、Prometheus 嘅差异带入 domain。真实 provider adapter 可以逐个能力补齐；只要 application Port 唔变，Inspection、AI 工具同前端都唔需要跟住厂商 SDK 改。

跨上下文关系：

| 调用方 | 被调能力 | 说明 |
| --- | --- | --- |
| 前端 `observability.ts` | 查询指标/日志/链路/拓扑 | 用 `app:observability:read` 控制 |
| Inspection | `ObservabilityQueryPort` | 收集巡检证据，结果用 `evidence_id` 串联 |
| AI 工具网关 | `cloud.metrics.query` 等只读工具 | 后续注册工具时要继承用户身份同审计 |
| Integration | 账号摘要来源 | 只提供账号、provider、capability、credential ref |

## API 与权限

契约：`ops/cloud-observability-contract.md` §5。

| 权限码 | 说明 |
| --- | --- |
| `app:observability:read` | 指标/日志/链路/拓扑查询 |

## 迁移

- `0019_init_observability.up.sql`：`obs_evidence_ref` + `app:observability:read` 种子

## 测试

```powershell
go test ./internal/observability/...
```
