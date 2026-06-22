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

## 权限

- `app:inspections:read` — 查看策略/运行/发现
- `app:inspections:write` — 创建策略、触发巡检

迁移：`0020_init_inspection.up.sql`

契约：`ops/cloud-observability-contract.md` §6
