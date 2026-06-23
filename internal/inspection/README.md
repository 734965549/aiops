# Inspection 限界上下文

巡检策略、运行、发现与建议；编排 Observability 证据链与规则/AI 分析。

## 目录

```text
internal/inspection/
  domain/           策略、运行状态机、发现、建议
  application/      PolicyService、RunService、EvidenceAnalyzer
  infrastructure/
    persistence/    GORM 仓储
    observability/  QueryAdapter（包装 Observability QueryService）
    audit/          审计 recorder
  interfaces/http/  /api/inspections/*
```

## 主流程

```text
创建 Policy（含 scope.account_id + checks）
  -> POST /policies/:id/runs 手动触发
  -> RunService 按 checks 调用 Observability 查询（生成 evidence_id）
  -> EvidenceAnalyzer 分析证据 -> Finding + Recommendation
  -> 运行状态 success | partial | failed
```

## 调用关系（粤语补充）

```text
/api/inspections/*
  -> interfaces/http Handler
  -> PolicyService / RunService / RecommendationService
  -> domain.Policy / Run / Finding / Recommendation
  -> infrastructure.persistence
  -> infrastructure.observability.QueryAdapter
  -> observability.application.QueryService
  -> Provider Port + EvidenceRef
```

Inspection 唔直接接云厂商 SDK，亦唔直接读 Integration 凭据。佢只经 `ObservabilityQueryPort` 收集证据，再由 `EvidenceAnalyzer` 生成 Finding 同 Recommendation。Recommendation 如要变成真实处置，只可以经 `ExecutionCreatorPort` 创建 Execution Task；任务后续是否执行，由 Execution 权限、风险、确认文本同审计决定。

同其他上下文关系：

| 调用方/被调方 | 调用方向 | 说明 |
| --- | --- | --- |
| 前端 `inspection.ts` | -> Inspection | 管策略、触发运行、查运行/发现 |
| Inspection | -> Observability | 查询指标/日志/链路，生成证据链 |
| Inspection | -> AI（后续） | 对脱敏证据做分析，唔传凭据 |
| Inspection | -> Execution | 只创建待确认任务，唔直接 dispatch |
| Dashboard / Audit | <- Inspection | 展示巡检摘要同审计追溯 |

## 权限

- `app:inspections:read` — 查看策略/运行/发现
- `app:inspections:write` — 创建策略、触发巡检

迁移：`0020_init_inspection.up.sql`

契约：`ops/cloud-observability-contract.md` §6
