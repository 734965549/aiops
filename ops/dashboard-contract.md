# Dashboard 首页驾驶舱契约

本文档定义 Dashboard 只读聚合接口、降级语义和前端行为。Dashboard 不拥有 Alert、Execution、Asset 或 Runbook 业务数据，也不提供写操作。

## 1. 当前实现状态

| 能力 | 状态 | 边界 |
| --- | --- | --- |
| 跨模块摘要 | 已落地 | 告警、执行、资产、Runbook 和活跃告警提示 |
| 单项容错 | 已落地 | 单项失败降级为 0/空，其他项继续 |
| 降级可观测性 | 未提供 | 响应没有 degraded/error/reason，无法区分真实 0 与降级 0 |
| 首页展示与跳转 | 已落地 | 指标卡、Runbook 推荐、最近执行、版本与 readiness |
| 数据范围过滤 | 未提供 | 只有 `app:dashboard:read` 静态 RBAC |
| 专用 E2E/前端测试 | 未提供 | 目前只有单元/HTTP 测试和 Identity E2E 单次调用 |

## 2. 接口

`GET /api/dashboard/summary`

- 鉴权：Bearer + `app:dashboard:read`。
- migration：`0013_dashboard_permission`。
- 未登录返回 401，权限不足返回 403。
- 响应采用统一 `code/message/trace_id/data` 封装。

## 3. 响应模型

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "alerts": {
      "active_total": 12,
      "p0": 1,
      "p1": 3
    },
    "executions": {
      "pending_confirm": 2,
      "recent": []
    },
    "assets": {
      "applications": 5,
      "resources": 20
    },
    "runbooks": {
      "total": 4,
      "enabled": 3
    },
    "processing_alerts": []
  }
}
```

### 3.1 `alerts`

| 字段 | 语义 |
| --- | --- |
| `active_total` | `active_only=true` 的告警数，只排除 closed |
| `p0` | 同 active 范围内 severity=p0 |
| `p1` | 同 active 范围内 severity=p1 |

recovered 和 silenced 不是 closed，因此仍可能属于 active 统计。

### 3.2 `executions`

| 字段 | 语义 |
| --- | --- |
| `pending_confirm` | 当前 pending_confirm 任务数 |
| `recent` | 最近最多 10 条任务，按 created_at DESC，含当前状态和结果，不限成功任务 |

### 3.3 `assets`

| 字段 | 语义 |
| --- | --- |
| `applications` | asset_application 注册总量 |
| `resources` | asset_resource 注册总量 |

当前没有数据范围过滤，不能把这些总数解释为当前用户在所有业务模块中可见的同等范围。

### 3.4 `runbooks`

| 字段 | 语义 |
| --- | --- |
| `total` | 模板总数 |
| `enabled` | enabled=true 的模板数 |

推荐模板不直接包含在 summary 中，前端另调 Runbook 推荐接口。

### 3.5 `processing_alerts`

最多 5 条提示，按 last_seen_at DESC，包含：

- new
- acknowledged
- processing

因此该字段并不只包含 status=processing，前端“处理中告警”是工作流展示名称，不能按单一状态过滤理解。

## 4. 容错与降级

SummaryService 对各统计块分别读取：

- 单项失败时，该项数值为 0或列表为空。
- 其他统计项继续返回。
- `executions.recent` 和 `processing_alerts` 始终可返回空数组。

响应没有 degraded、error 或 reason 字段，真实零值与依赖失败降级不可区分。客户端不得把 0 解释为依赖健康，也不得把空推荐解释为没有任何 Runbook。

前端额外行为：

- summary 整体请求失败：摘要置空，指标显示 `—`，最近执行和推荐为空；当前无专门错误 Alert。
- 单条推荐失败：忽略该告警，保留其他推荐组。
- `/version` 或 `/readyz` 失败：健康信息显示未探测，不直接影响 summary 已返回内容。

## 5. 前端聚合与跳转

首页并行请求 summary、version 和 readiness：

1. 指标卡：活跃告警、P0、P1、待确认执行、注册资源、平台状态；可跳转 /alerts、/executions、/assets。
2. Runbook 推荐：取 `processing_alerts` 前 3 条，每条并发请求推荐接口，每组最多展示 3 个模板；最近 Runbook 执行最多 5 条。
3. 最近执行结果：跳转 `/executions?task_id=<id>`。
4. ReadinessPanel：展示版本、环境、运行时长和各检查项 ok/down/degraded。
5. 刷新按钮重新加载 summary 和健康信息。

`/version` 与 `/readyz` 是独立系统接口，其 liveness/readiness 语义见 `ops/health-contract.md`，不属于 summary DTO。

## 6. 权限与数据范围

Dashboard 使用 `AuthorizeStatic`，仅要求 `app:dashboard:read`。viewer 角色具有该只读权限。

当前没有对象级数据范围过滤。即使 Alert、Asset 或 AI 的某些专用调用点有数据范围校验，也不能推断 Dashboard 聚合计数使用同一过滤规则。

## 7. 验证覆盖

现有测试：

- `summary_service_test.go`：正常聚合，以及 Alert 块失败时其他块继续。
- Handler：未登录、权限不足、成功响应。
- `e2e-identity-access.ps1`：viewer 能调用一次 summary，但不逐项断言字段。

当前没有：

- 专用 Dashboard E2E。
- processing_alerts 状态与排序、全部统计块失败的端到端验收。
- 前端 Vitest、降级、推荐数量和页面跳转测试。

后续验收应分别记录 API 字段、单项降级、推荐组装、Readiness 和 UI 结果，不能用一次 200 响应代替完整 Dashboard 验收。
