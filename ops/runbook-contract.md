# Runbook 处置预案工程契约（Execution Phase 2）

本文档定义 Runbook 模板化处置的当前接口和实现边界。Runbook 负责模板、推荐和向 Execution 提供可执行快照，不直接执行真实动作。Execution 状态机、确认和执行代理分别见 `execution-contract.md`、`execution-agent-contract.md`。

## 1. 当前实现状态

| 能力 | 状态 | 边界 |
| --- | --- | --- |
| 模板与结构化步骤 | 已落地 | 创建、分页、详情、全量更新、软停用；至少一个步骤 |
| 告警推荐 | 已落地 | 启用模板按匹配分、风险、更新时间排序；推荐 API 不限制告警状态 |
| Runbook 生成多步任务 | 已落地 | 仅非 Agent 的模拟执行路径展开模板，保存 snapshot |
| 风险与人工确认 | 已落地 | low 为 pending_execute，其余为 pending_confirm；用户只能提高风险 |
| Dry-run | 已落地 | 任务 dry_run 与步骤 dry_run_supported 共同决定有效 dry-run |
| 前端模板管理 | 部分落地 | 列表、详情、启停；无新建、完整编辑、删除 UI |
| Command Spec / medium_selector | 未落地 | 当前 Runbook 步骤模型没有这些字段，Runbook→Agent 展开也未实现 |

非目标仍包括图形化编排器、多级审批、复杂 DAG、自动无人工确认执行，以及由 Runbook 直接调用云 API/K8s/SSH。

## 2. 数据模型与迁移

migration：`0012_init_runbook.up.sql`。

| 表/字段 | 说明 |
| --- | --- |
| `runbook_template` | 模板、enabled、operation_type、risk_level、匹配条件、parameter_schema、rollback_plan |
| `runbook_step` | step_order、action_type、risk_level、dry_run、parameter/default schema、rollback_plan、timeout_seconds |
| `exec_task.runbook_template_id` | 创建任务使用的模板业务 ID |
| `exec_task.runbook_snapshot` | 创建时的模板与步骤快照，后续模板更新不回写 |
| `exec_task.dry_run` | 任务是否按 dry-run 模拟 |
| `exec_step.runbook_step_id` | 来源步骤业务 ID |
| `exec_step.parameters/risk_level/dry_run/rollback_plan/timeout_seconds` | 展开后的步骤字段 |

`0012` 同时种子 HighCPU + pod + prod 的三步 `Restart service and verify` 模板及 Runbook 权限。

模板字段：

- 模板：name、description、enabled、operation_type、risk_level、match_alert_name、match_resource_type、match_environment、parameter_schema、rollback_plan。
- 步骤：step_order、name、action_type、risk_level、dry_run_supported、default_dry_run、parameter_schema、default_parameters、rollback_plan、timeout_seconds。
- 当前没有 `command_spec_id`、`medium_selector` 字段。

## 3. 统一响应与权限

成功响应沿用 `code/message/trace_id/data`，成功码为字符串 `OK`。列表为 PageData，默认 page=1、page_size=20，最大 100。

| 路由 | 权限 | 当前语义 |
| --- | --- | --- |
| `GET /api/runbooks/templates` | `app:runbooks:read` | 分页、keyword、enabled 筛选 |
| `GET /api/runbooks/templates/:template_id` | `app:runbooks:read` | 模板与有序步骤 |
| `POST /api/runbooks/templates` | `app:runbooks:create` | 同事务创建模板和至少一个步骤 |
| `PUT /api/runbooks/templates/:template_id` | `app:runbooks:update` | 全量更新模板并替换全部步骤 |
| `DELETE /api/runbooks/templates/:template_id` | `app:runbooks:delete` | 软停用 enabled=false，不是物理删除 |
| `GET /api/runbooks/recommendations?alert_id=` | `app:runbooks:read` | 推荐启用模板，不直接创建任务 |

未登录返回 401，权限不足返回 403。路由使用静态 RBAC；告警 processing 约束由 Execution 创建任务时另行校验。

## 4. 模板管理语义

### 4.1 创建与更新

创建和更新均要求：

- name、operation_type、risk_level 合法，且至少一个步骤。
- 每个步骤 name、action_type、risk_level 合法，step_order ≥ 1。
- timeout_seconds 未提供或小于等于 0 时默认 300。
- 模板与步骤使用事务写入；更新采用全量步骤替换。

前端启停流程会先读取完整详情，再回传模板和步骤，避免只传 enabled 导致空步骤被拒绝。其他调用方更新时也必须提供完整步骤集合。

### 4.2 删除与历史任务

DELETE 只把 enabled 设置为 false：

- 不再参与推荐和新的执行创建。
- 已存在的执行任务、runbook_snapshot、时间线和审计保留。
- 不物理删除模板和步骤。

`GetForExecution` 会拒绝 disabled 模板和无步骤模板。

## 5. 推荐契约

`GET /api/runbooks/recommendations?alert_id=<alert_id>` 返回 `{ "items": [] }`，每项包含：

| 字段 | 说明 |
| --- | --- |
| `template_id` | 模板业务 ID |
| `name` / `description` | 模板展示信息 |
| `operation_type` / `risk_level` | 操作类型与风险 |
| `matched_reason` | 命中的匹配维度或 wildcard |
| `steps_count` | 当前模板步骤数 |
| `dry_run_supported` | 至少一个步骤支持 dry-run |
| `parameter_schema` | 任务参数 schema |

匹配与排序：

- 环境精确匹配：1000；资源类型精确匹配：100；告警名精确匹配：10；名称包含式模糊匹配：1。
- 空 match 字段为通配；非空字段不匹配则排除。
- 依次按匹配分降序、模板风险升序、更新时间降序。

**状态边界**：推荐 API 只要求告警存在，不限制 new/acknowledged/processing/recovered 等状态。前端只在 processing 显示创建入口，Execution 后端在创建任务时再次强制 processing，不能只依赖前端按钮。

推荐会尝试写 `resource_type=alert`、`action=runbook_recommend` 审计，payload 包含 alert_id 和推荐数量；审计失败不阻断推荐。

## 6. Runbook 创建执行任务

`POST /api/executions/tasks` 在既有字段上接受：

```json
{
  "source_type": "alert",
  "source_id": "alert-001",
  "runbook_template_id": "tpl-001",
  "dry_run": true,
  "parameters": {
    "service_name": "payment-service",
    "replicas": 3
  }
}
```

处理顺序：

1. source_type=alert 时告警必须为 processing，并补充环境、目标资源等缺省字段。
2. 模板必须 enabled 且至少一步；operation_type 未提供时使用模板 operation_type。
3. 校验模板 parameter_schema。
4. 按 step_order 逐步合并 default_parameters 与用户 parameters，用户值覆盖默认值，再校验步骤 parameter_schema。
5. 生成 exec_step，保留 runbook_step_id、action_type、risk_level、dry_run、rollback_plan、timeout_seconds。
6. 保存创建时的 runbook_snapshot，后续模板更新不影响历史任务。
7. 写 execution_created 时间线和 create_from_runbook 审计；确认/执行后回写 execution_started/finished。

### 6.1 风险

任务风险取以下最大值：

- 操作默认风险。
- 模板风险。
- 各步骤风险。
- prod 环境下 script/command/custom 至少 medium。
- 用户 override。

用户 override 只能提高风险，降低返回 INVALID_ARGUMENT。low → pending_execute；medium/high/critical → pending_confirm。low 仍需调用 Execution 执行入口，不是创建后自动执行。

### 6.2 Dry-run

有效步骤 dry-run 为：

```text
task.dry_run && step.dry_run_supported
```

支持 dry-run 的步骤由模拟器输出 dry_run 标记；不支持的步骤保留 `skipped_real_execution`。`default_dry_run` 会保存在模板和 snapshot 中，但当前不会脱离任务 dry_run 单独触发模拟，不能把它解释为默认自动执行 dry-run。

### 6.3 Command Spec / Agent（规划，当前未落地）

早期设想步骤引用：

```json
{
  "action_type": "command",
  "command_spec_id": "cmd_linux_disk_usage",
  "medium_selector": {
    "medium_type": "jumpbox",
    "environment": "prod",
    "capabilities": ["linux.command.readonly"]
  },
  "parameters": { "mount_point": "/" }
}
```

当前 Runbook 模板、步骤 DTO 和 Execution 的 RunbookLoader 均没有 command_spec_id/medium_selector。`execution_mode=agent` 会进入独立 createAgentTask，不走 Runbook 模板展开。因而当前不能宣称“Runbook 步骤已绑定 Command Spec 并自动选择介体”。若后续实现，必须补齐模板 schema、Command Spec 校验、Execution 确认页、风险、审计和专项 E2E。

## 7. 前端实现

- `/runbooks`：分页列表、关键字搜索、模板详情和启停；没有新建、完整编辑或删除 UI。
- 告警详情：processing 时显示推荐处置入口，选择模板后填写可选任务名、dry-run 和 JSON object 参数。
- dry-run 默认开启，只有推荐结果至少一步支持时显示开关；参数 JSON 必须是 object，数组/标量/非法 JSON 不提交。
- 创建任务后进入 Execution 确认/执行页面，Runbook 页面不直接执行。
- 当前没有独立 Runbook 前端 Vitest，不能把告警页其他测试当作 Runbook 交互覆盖。

## 8. 审计动作

| resource_type | action | 语义 |
| --- | --- | --- |
| `runbook` | `runbook_create` / `runbook_update` / `runbook_delete` | 模板创建、更新、软停用 |
| `runbook` | `runbook_enable` / `runbook_disable` | enabled 状态变化时在 update 之外追加 |
| `alert` | `runbook_recommend` | 推荐数量与 alert_id |
| Execution Task | `create_from_runbook` | 从模板创建执行任务 |

Runbook 审计均为 best-effort，失败不回滚模板变更、推荐或任务创建。审计不记录 secret、Token 或未脱敏参数。

## 9. 错误与边界

| code | 典型场景 |
| --- | --- |
| `INVALID_ARGUMENT` | 空步骤、非法枚举/schema、disabled 模板、非 processing 告警、降低风险 |
| `UNAUTHENTICATED` | 未登录或 token 无效 |
| `PERMISSION_DENIED` | 缺少 Runbook/Execution 权限 |
| `NOT_FOUND` | 模板或告警不存在 |
| `ALREADY_EXISTS` | 业务唯一键冲突 |
| `UNAVAILABLE` | Runbook loader、AlertReader 或 Execution 依赖未装配 |

## 10. 验证覆盖

### 单元与 HTTP 测试

- 模板创建、全量更新、软停用、空步骤和禁用模板拒绝。
- 匹配、通配、模糊名称、推荐结果、风险和 snapshot 源数据。
- Create/推荐 Handler、权限拒绝、无效参数。
- Execution 单测另覆盖多步生成、风险、dry-run、snapshot 持久化和时间线。

### E2E 的实际范围

`scripts/e2e-runbook.ps1` 覆盖：

1. 创建或复用 processing 告警；
2. 推荐并选择模板；
3. 创建 prod medium 多步 dry-run 任务，状态 pending_confirm；
4. 检查多步与 runbook metadata；
5. CONFIRM 后 pending_execute，模拟执行 success；
6. 首步 dry_run 标记；
7. execution_created/started/finished 时间线。

脚本不覆盖模板 CRUD E2E、非 processing 拒绝、low 风险、步骤失败、完整前端交互、Command Spec 或 Runbook→Agent。测试和脚本存在不代表本轮已执行通过，实际验收需单独记录日期、环境和结果。
