# Runbook 处置预案工程契约（Execution Phase 2）

本文档定义 Runbook 模板化处置闭环工程契约，在 `ops/execution-contract.md` 第一阶段基础上扩展。

## 1. 范围

- `runbook_template` / `runbook_step` 预案模板与步骤模板。
- 告警详情页 Runbook 推荐与从预案创建执行任务。
- 执行任务按模板生成多步骤，保存 `runbook_snapshot`。
- 任务/步骤级 dry-run、风险、参数、回滚字段。
- 同步模拟执行（无真实 K8s/云厂商/SSH 适配器）。
- 告警时间线与审计扩展。

## 2. 非目标

- 图形化编排器、多级审批、真实执行代理、自动无人工确认执行、复杂 DAG。

## 3. 数据表

| 表名 | 说明 |
| --- | --- |
| `runbook_template` | 预案模板 |
| `runbook_step` | 预案步骤模板 |
| `exec_task` | 扩展 `runbook_template_id`、`runbook_snapshot`、`dry_run` |
| `exec_step` | 扩展 `runbook_step_id`、`parameters`、`risk_level`、`dry_run`、`rollback_plan`、`timeout_seconds` |

迁移：`0012_init_runbook.up.sql`。

## 4. HTTP API

### 4.1 Runbook 管理

| Method | Path | 权限 |
| --- | --- | --- |
| GET | `/api/runbooks/templates` | `app:runbooks:read` |
| GET | `/api/runbooks/templates/:template_id` | `app:runbooks:read` |
| POST | `/api/runbooks/templates` | `app:runbooks:create` |
| PUT | `/api/runbooks/templates/:template_id` | `app:runbooks:update` |
| DELETE | `/api/runbooks/templates/:template_id` | `app:runbooks:delete` |

### 4.2 推荐

- `GET /api/runbooks/recommendations?alert_id=xxx`
- 权限：`app:runbooks:read`

### 4.3 Execution 创建扩展

`POST /api/executions/tasks` 新增字段：

```json
{
  "runbook_template_id": "tpl-001",
  "dry_run": true,
  "parameters": {}
}
```

`source_type=alert` 时告警必须为 `processing`。提供 `runbook_template_id` 时按 `runbook_step.step_order` 生成多个 `exec_step`，并保存 `runbook_snapshot`。

## 5. 风险规则

任务风险 = max(操作默认风险, 模板风险, 各步骤风险, prod 下 script/command/custom 至少 medium, 用户 override)。

用户仅允许提高风险，不允许降低。`low` → `pending_execute`；其余 → `pending_confirm`。

## 6. 执行规则

步骤串行：`pending → running → success|failed`。任一步失败则任务 `failed`，后续步骤保持 `pending`。

任务 `dry_run=true` 时，支持 dry-run 的步骤按 dry-run 模拟；不支持的步骤 output 标记 `skipped_real_execution`。

### 6.1 P1+ 真实执行代理扩展

当 Execution 引入执行介体后，Runbook 步骤不直接保存自由命令，而是引用平台预置 Command Spec：

```json
{
  "action_type": "command",
  "command_spec_id": "cmd_linux_disk_usage",
  "medium_selector": {
    "medium_type": "jumpbox",
    "environment": "prod",
    "capabilities": ["linux.command.readonly"]
  },
  "parameters": {
    "mount_point": "/"
  }
}
```

约束：

- Runbook 模板只能引用已启用 Command Spec。
- `parameters` 必须通过 Command Spec 的 schema 校验。
- prod 环境的 `command` / `script` / `custom` 步骤至少为 medium 风险。
- 执行介体可由模板建议，但最终必须在 Execution 确认页展示并由运维人员确认。
- 真实执行细节见 `ops/execution-agent-contract.md`。

## 7. 审计动作

`runbook_create` / `runbook_update` / `runbook_delete` / `runbook_recommend` / `create_from_runbook`（execution）/ `execution_create`（alert）。

## 8. 验收标准

1. `processing` 告警可看到推荐 Runbook。
2. 非 `processing` 告警不能从 Alert 创建 Runbook 任务。
3. 选择 Runbook 后生成多条 `exec_step`，保存 snapshot。
4. 中高风险需确认，low 可直接执行。
5. 执行成功/失败与时间线回写符合契约。
6. `go test ./...` 与 `npm run build` 通过。
7. E2E 脚本 `scripts/e2e-runbook.ps1` 覆盖推荐 → 创建 → 确认 → 执行 → 时间线全链路。
