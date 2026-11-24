# Execution 执行任务工程契约

本文档定义 Execution 任务、步骤、风险、确认/拒绝、同步模拟执行和告警时间线的当前契约。Runbook 展开见 `runbook-contract.md`，Agent 控制面见 `execution-agent-contract.md`。

## 1. 当前实现状态

| 能力 | 状态 | 边界 |
| --- | --- | --- |
| 任务创建与查询 | 已落地 | 来源 alert/manual/ai_conversation/inspection；alert 创建必须 processing |
| 风险与确认 | 已落地 | low 直达 pending_execute，其余 pending_confirm；CONFIRM 大小写敏感 |
| 拒绝 | 已落地 | confirm=false 将 pending_confirm 转为 cancelled；前端暂无拒绝按钮 |
| 同步模拟执行 | 已落地 | 顺序执行步骤，失败停止；不调用真实 SSH/K8s/云 API |
| Runbook 多步任务 | 已落地 | 非 Agent 路径展开模板并保存 snapshot |
| 任务日志查询 | 已落地 | `GET /tasks/:task_id/logs?step_id=`，主要服务 Agent 日志 |
| Agent 控制面 | 已落地 | 见专项契约；不能通过同步 execute 执行 Agent 任务 |
| adapter 模式 | 未实现 | 枚举保留，没有独立外部适配器执行链路 |

非目标包括多级审批、通用运行中取消、自动回滚编排，以及前端直接执行任何真实动作。

## 2. 统一响应与权限

成功响应为 `code/message/trace_id/data`，成功码为字符串 `OK`。列表使用 PageData，默认 page=1、page_size=20，最大 100。

| 路由 | 权限 |
| --- | --- |
| `GET /api/executions/tasks` | `app:executions:read` |
| `GET /api/executions/tasks/:task_id` | `app:executions:read` |
| `GET /api/executions/tasks/:task_id/logs?step_id=` | `app:executions:read` |
| `POST /api/executions/tasks` | `app:executions:create` |
| `POST /api/executions/tasks/:task_id/confirm` | `app:executions:confirm` |
| `POST /api/executions/tasks/:task_id/execute` | `app:executions:execute` |

用户路由使用静态 RBAC，当前没有任务对象级数据范围过滤。Medium、Command Spec 和 Agent 路由权限见专项契约，不能笼统写成所有 Agent 接口都要求用户 Bearer。

## 3. 领域枚举与状态机

### 3.1 任务状态

| 状态 | 说明 |
| --- | --- |
| `pending_confirm` | 中高风险，等待确认或拒绝 |
| `pending_execute` | 低风险直进或已确认；Agent 任务此时才可被 lease |
| `running` | 正在执行步骤 |
| `success` / `failed` | 终态 |
| `cancelled` | confirm=false 拒绝后的终态 |

```text
create(low)                    -> pending_execute
create(medium/high/critical)   -> pending_confirm
pending_confirm + CONFIRM      -> pending_execute
pending_confirm + confirm=false -> cancelled
pending_execute + execute      -> running
running + all steps success    -> success
running + any step failure     -> failed
```

没有 pending_execute/running 的通用取消接口。

### 3.2 来源类型

| 来源 | 当前用途 |
| --- | --- |
| `alert` | 告警处置，必须 processing，模拟路径回写时间线 |
| `manual` | API/Agent 手工任务 |
| `ai_conversation` | AI 助手生成待确认执行计划 |
| `inspection` | Inspection Recommendation 转执行 |

### 3.3 操作类型与风险

支持 restart、scale、script、runbook、command、custom。`diagnose` 不是当前任务 operation_type；Command Spec 的 action_type=diagnose 会生成 operation_type=command 的 Agent 任务。

风险为 low/medium/high/critical：

- restart/scale/custom 默认 medium。
- script/runbook 非 prod 默认 low，prod 默认 medium。
- Runbook 叠加模板、步骤和 prod command/script/custom 风险。
- Agent 叠加 Medium 类型、max_risk_level、Spec 和环境风险。
- 用户 override 只能提高，降低返回 INVALID_ARGUMENT。

### 3.4 执行模式

- `simulated`：默认，使用同步模拟执行。
- `agent`：确认后由 Agent lease/log/result，不能调用同步 execute。
- `adapter`：枚举保留，没有独立实现，不应作为可用生产模式验收。

## 4. 任务与步骤模型

ExecutionTask 主要字段：

- 身份与来源：id、name、source_type、source_id。
- 操作与目标：operation_type、target_type、target_id、target_name、environment。
- 风险与状态：risk_level、status、execution_mode、dispatch_status。
- 参数与计划：parameters、rollback_plan、dry_run。
- Runbook：runbook_template_id、runbook_snapshot、runbook_name（由 snapshot 派生）。
- Agent：medium_id、agent_id、lease_id、command_spec_id。
- 结果：result_summary、error_message。
- 人员与时间：created_by、confirmed_by、executed_by、created_at、confirmed_at、started_at、finished_at。

ExecutionStep 主要字段：

- id、task_id、step_order、name、action_type、status。
- runbook_step_id 或 command_spec_id。
- parameters/arguments、risk_level、dry_run/dry_run_supported、rollback_plan、timeout_seconds。
- output、error_message、started_at、finished_at。

公开 DTO 不直接返回数据库自增 ID。Runbook 历史任务使用创建时 snapshot，模板后续更新不改变历史任务。

## 5. HTTP API

### 5.1 创建任务

`POST /api/executions/tasks`，权限 `app:executions:create`。

普通模拟任务示例：

```json
{
  "name": "重启 payment-service",
  "source_type": "alert",
  "source_id": "alert-001",
  "operation_type": "restart",
  "environment": "prod",
  "parameters": { "grace_period_s": 30 },
  "rollback_plan": { "description": "如异常持续，回滚到上一版本" },
  "risk_level": "medium"
}
```

可提供 runbook_template_id 和 dry_run，此时 operation_type 可省略，由模板补充。source_type=alert 时 source_id 必填、状态必须 processing，缺省名称、目标和环境可从告警补充。

成功响应：

```json
{
  "task_id": "task-001",
  "status": "pending_confirm",
  "risk_level": "medium",
  "confirm_url": "/executions?task_id=task-001"
}
```

普通 alert 任务创建后写 execution_created；Agent 创建路径当前不写该时间线，见专项契约差距。

### 5.2 列表与详情

- `GET /api/executions/tasks`：page/page_size/status/source_type/source_id/keyword。
- `GET /api/executions/tasks/:task_id`：返回 `{ "task": {}, "steps": [] }`。

### 5.3 确认或拒绝

`POST /api/executions/tasks/:task_id/confirm`，权限 `app:executions:confirm`。

确认：

```json
{ "confirm": true, "confirm_text": "CONFIRM" }
```

仅 pending_confirm → pending_execute；Agent 任务同时进入 pending_dispatch。

拒绝：

```json
{ "confirm": false }
```

仅 pending_confirm → cancelled，不要求 CONFIRM，并写 reject 审计。重复确认、重复拒绝或非法状态返回错误。

### 5.4 同步模拟执行

`POST /api/executions/tasks/:task_id/execute`，权限 `app:executions:execute`。

- 仅 pending_execute 可执行。
- execution_mode=agent 返回 FAILED_PRECONDITION，必须由 Agent 领取。
- 普通任务 pending_execute → running，串行执行步骤。
- 任一步失败则任务 failed，后续步骤保持 pending；全部成功任务 success。
- 无步骤、读取步骤失败或步骤持久化失败不能把任务标记为成功。
- alert 来源写 execution_started/finished。

### 5.5 查询任务日志

`GET /api/executions/tasks/:task_id/logs?step_id=<step_id>`，权限 `app:executions:read`。

step_id 必填，返回指定任务和步骤的 Agent 日志条目，包含 stream、sequence、content、truncated、redacted、observed_at。当前前端未封装该接口，也没有实时日志 UI。

## 6. Runbook 与 Alert 关系

Runbook 非 Agent 路径：

1. 校验 processing、模板 enabled 和步骤。
2. 校验模板/步骤 schema，合并默认与用户参数。
3. 生成多步骤、保存 snapshot、聚合风险。
4. 写 execution_created 和 create_from_runbook 审计。
5. 确认/执行后写 execution_started/finished。

普通 alert 模拟任务时间线：

| 场景 | 事件 | 关键 payload |
| --- | --- | --- |
| 创建 | `execution_created` | execution_id、operation_type、risk_level、dry_run、Runbook 信息 |
| 开始 | `execution_started` | execution_id、operation_type |
| 完成 | `execution_finished` | execution_id、status、result_summary/error |

时间线写入为业务旁路，失败记 Warn，不回滚已创建或已执行任务。审计 `resource_type=execution`、`resource_id=task_id`；普通 alert 创建还写 `resource_type=alert`、`action=execution_create`。

## 7. 数据与完整性

| 迁移 | 作用 |
| --- | --- |
| `0011` | exec_task / exec_step 与基础权限 |
| `0012` | Runbook snapshot、多步参数、风险、dry-run、rollback、timeout |
| `0022` | Agent/Medium/Command Spec/Lease/Log Stream 及任务扩展 |

应用层完整性：

- CreateWithSteps 在同一事务创建任务和步骤。
- 创建前至少一步，step.task_id 必须等于 task.task_id。
- StepRepository.Create 拒绝父任务不存在的孤儿步骤。
- `(task_id, step_order)` 唯一。
- Execute 对无步骤任务落 failed，不允许零步骤成功。

## 8. 前端实现

`/executions` 支持列表筛选、详情、步骤输出、CONFIRM、同步执行、Runbook/dry-run 展示和 task_id 深链。

当前未提供拒绝按钮、Agent 任务创建表单、Medium/Agent/Command Spec 管理页、租约/自动匹配视图或实时日志流。`web/src/api/execution.ts` 未封装日志、Medium、Command Spec 和 Agent 创建字段。没有独立 Execution 前端 Vitest。

## 9. 验证覆盖

### 单元与 HTTP 测试

现有覆盖状态机、风险、重复确认/执行、非 processing、无步骤、Runbook 多步、snapshot、时间线、持久化和 Handler 权限。Command argv、schema 和输出脱敏另有测试。

### E2E 的实际范围

- `e2e-execution.ps1`：processing alert、restart pending_confirm、CONFIRM、模拟 success、execution_created/started/finished。
- `e2e-runbook.ps1`：Runbook 多步 dry-run 模拟闭环。
- Agent 脚本只覆盖专项契约列出的 fake 控制面路径。

核心脚本不覆盖 confirm=false 拒绝、low 风险、步骤失败、任务日志查询、adapter、完整前端或真实执行。测试和脚本存在不代表本轮已执行通过。
