# internal/dashboard 模块说明

Dashboard 是只读首页聚合上下文，负责把 Alert、Execution、Asset 和 Runbook 的现有读模型汇总为首页摘要。它不拥有这些业务数据，不创建或修改任务，也不替代各模块的权限与状态机。对外契约见 `ops/dashboard-contract.md`。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 告警摘要 | 未关闭告警总数及 P0/P1 计数 | active_only 只排除 closed，recovered/silenced 也可能计入 |
| 执行摘要 | pending_confirm 数、最近 10 条任务 | 最近任务按 created_at DESC；不代表只有成功任务 |
| 资产摘要 | 应用数、资源数 | 是注册表总量，不做数据范围过滤 |
| Runbook 摘要 | 模板总数和 enabled 数 | 推荐不是 summary 直接返回，而是前端再调推荐 API |
| 活跃告警提示 | 最多 5 条 new/acknowledged/processing 告警，last_seen_at DESC | 不是仅 processing；前端只取前 3 条获取推荐 |
| 容错 | 单块统计失败返回 0/空列表，其他块继续 | 响应没有 degraded/error 字段，真实 0 与降级 0 不可区分 |
| 平台状态 | 前端并行读取 `/version` 和 `/readyz` | 不属于 summary DTO；/readyz 语义见健康契约 |
| 数据范围 | `app:dashboard:read` 静态 RBAC | 无对象级数据范围过滤，不能按各业务模块隔离程度推断 |

## 2. 调用关系

```text
GET /api/dashboard/summary
  -> SummaryService.GetSummary
  -> StatsReader（跨模块只读适配）
       CountAlerts / ListAlerts
       CountTasks / ListTasks
       CountApplications / CountResources
       CountRunbookTemplates
  -> 各块独立容错，返回 SummaryDTO
```

前端首页还会：

```text
summary.processing_alerts
  -> 取前 3 条告警
  -> 每条并发调用 GET /api/runbooks/recommendations
  -> 单条失败忽略，保留其他推荐组
  -> 每组最多展示 3 个模板

/version + /readyz
  -> 展示版本、环境、运行时长和各检查项状态
```

## 3. 路由与权限

`GET /api/dashboard/summary`，要求 Bearer + `app:dashboard:read`。migration `0013_dashboard_permission` 种子该权限，viewer 角色也具备 Dashboard 只读能力。

未登录返回 401，权限不足返回 403。`/version` 和 `/readyz` 是独立系统路由，不继承 Dashboard summary 的权限语义。

## 4. 聚合响应

```json
{
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
```

- `executions.recent` 最多 10 条，包含任务当前状态和结果。
- `processing_alerts` 最多 5 条，字段为 id/name/severity/status。
- 推荐模板、Readiness 和版本信息不在该响应中。

## 5. 降级语义

SummaryService 对每一项统计分别调用，单项失败只保留该项默认值，不返回整体错误：

- 数值块为 0。
- recent 和 processing_alerts 为空数组。
- 其他统计块继续返回。

响应没有 degraded/error/reason 字段，因此调用方不能区分“真实为 0”与“依赖失败降级为 0”。若发布或运维验收需要可观测的降级原因，应另加契约字段与测试，不能从现有响应猜测。

前端也有两层降级：

- summary 请求整体失败时清空摘要，指标显示 `—`，最近执行和推荐为空；当前没有专门错误 Alert。
- 单条 Runbook 推荐失败时忽略该告警，其他推荐组继续展示。

## 6. 前端页面

- 指标卡：活跃告警、P0、P1、待确认执行、注册资源、平台状态；支持跳转对应模块。
- 处置建议与 Runbook：enabled/total、推荐组、最近 Runbook 执行（最多 5 条）。
- 最近执行结果：任务、状态、结果、时间，点击 `/executions?task_id=`。
- ReadinessPanel：版本、环境和 `/readyz` 各检查项 ok/down/degraded。
- 刷新会并行重取 summary 和健康信息。

当前没有独立 Dashboard 前端 Vitest，也没有专用 Dashboard E2E；`e2e-identity-access.ps1` 只以 viewer 调用一次 summary，不能覆盖全部字段和降级/UI。

## 7. 验证覆盖

- `summary_service_test.go` 覆盖正常聚合和 Alert 块失败时其他块仍返回。
- Handler 测试覆盖未登录、权限不足和成功响应。
- 当前没有 processing_alerts 状态/推荐数量、全部统计块失败、前端降级和跳转的独立端到端覆盖。
- 本轮仅静态复核，未重新执行测试或脚本。
