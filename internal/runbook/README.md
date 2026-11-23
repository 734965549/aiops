# internal/runbook 模块说明

Runbook 负责处置预案模板、结构化步骤、告警推荐，以及向 Execution 提供可执行模板快照。Runbook 不直接执行真实动作；任务风险、确认、状态机、模拟/Agent 执行和审计由 Execution 负责。对外契约见 `ops/runbook-contract.md`。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 模板管理 | 模板与步骤创建、列表、详情、全量更新、软停用 | 更新会全量替换步骤；历史任务使用创建时保存的 snapshot，不随模板变化 |
| 告警推荐 | 按告警名、资源类型、环境匹配，按匹配分、风险和更新时间排序 | 推荐 API 不限制告警状态；创建执行任务才强制 processing |
| 多步任务生成 | 校验模板启用和步骤，生成 exec_step、保存 runbook_snapshot、合并默认/用户参数 | Execution 的模拟路径负责生成；真实 Agent 路径不走这套 Runbook 展开逻辑 |
| 风险与确认 | 模板/步骤/操作/环境风险取最大值，用户只能提高 | low 为 pending_execute，其余为 pending_confirm；仍需通过 Execution 执行入口 |
| Dry-run | 支持的任务按 dry_run 模拟，步骤输出保留 dry_run 标记 | 有效值为任务 `dry_run && step.dry_run_supported`；模板 `default_dry_run` 只作元数据，不单独触发 |
| 前端 | Runbook 列表/详情/启停，告警页推荐、参数 JSON、dry-run 和创建任务 | 前端没有模板新建、完整编辑或删除入口；后端 CRUD 不能等同为完整 UI |
| Command Spec | 当前步骤只有 action_type、参数 schema、默认参数、回滚和 timeout | `command_spec_id`、`medium_selector` 属于规划，当前模板与 Runbook→Agent 展开未实现 |

## 2. 分层与调用关系

```text
interfaces/http
  -> TemplateService
       List / GetDetail / Create / Update / Delete / Recommend / GetForExecution
  -> domain
       模板匹配评分、风险规则、步骤与 snapshot 结构
  -> infrastructure/persistence
       runbook_template / runbook_step（创建与更新使用事务）

Execution
  -> RunbookLoader.GetForExecution
  -> 校验模板启用且至少一步
  -> buildStepsFromRunbook + buildRunbookSnapshot
  -> 任务风险与确认状态机
  -> 模拟执行 / 时间线 / 审计

Alert
  -> AlertReader.GetForExecution
  -> 提供告警名、环境、资源类型、labels、annotations
```

## 3. 路由与权限

| 路由 | 权限 | 当前语义 |
| --- | --- | --- |
| `GET /api/runbooks/templates` | `app:runbooks:read` | 分页、keyword、enabled 筛选 |
| `GET /api/runbooks/templates/:template_id` | `app:runbooks:read` | 模板与有序步骤 |
| `POST /api/runbooks/templates` | `app:runbooks:create` | 同事务创建模板和至少一个步骤 |
| `PUT /api/runbooks/templates/:template_id` | `app:runbooks:update` | 全量更新模板并替换全部步骤 |
| `DELETE /api/runbooks/templates/:template_id` | `app:runbooks:delete` | **软停用 enabled=false**，不是物理删除 |
| `GET /api/runbooks/recommendations?alert_id=` | `app:runbooks:read` | 推荐启用模板，不直接创建任务 |

所有路由都使用静态 RBAC。推荐和模板路由没有额外对象级数据范围过滤；Execution 的创建权限与告警处理状态另行校验。

## 4. 推荐与模板规则

### 4.1 推荐评分

- 环境精确匹配：1000 分。
- 资源类型精确匹配：100 分。
- 告警名精确匹配：10 分；包含式模糊匹配：1 分。
- 空 match 字段为通配；非空字段不匹配则排除。
- 排序依次为匹配分降序、模板风险升序、更新时间降序。

推荐结果包含 template_id、operation_type、risk_level、matched_reason、steps_count、dry_run_supported 和 parameter_schema。`dry_run_supported` 表示至少一个步骤支持 dry-run。

推荐 API 只要求告警存在，不检查 processing。告警详情页只在 processing 显示创建入口，Execution 后端也再次拒绝非 processing 任务，不能只依赖前端隐藏按钮。

### 4.2 创建、更新和删除

- 模板至少包含一个步骤；operation_type 和每个 action_type、risk_level 必须合法。
- 步骤 step_order 从 1 开始，timeout 默认 300 秒；模板/步骤参数 schema 分别校验。
- 更新采用全量步骤替换，前端启停时会先读详情并回传完整模板和步骤。
- DELETE 只设置 enabled=false，使模板不再参与推荐和执行创建；历史任务和 snapshot 保留。
- enabled 变化除 runbook_update 外，另写 runbook_enable 或 runbook_disable 审计。

## 5. Runbook 到 Execution

`POST /api/executions/tasks` 接受 `runbook_template_id`、`dry_run` 和 `parameters`：

1. source_type=alert 时，告警必须为 processing。
2. 模板必须 enabled 且至少一步；operation_type 缺省时使用模板类型。
3. 校验模板 parameter_schema，逐步骤合并 default_parameters 与用户 parameters，用户值覆盖默认值，再校验步骤 schema。
4. 按 step_order 生成 exec_step，保留 runbook_step_id、action_type、risk、dry_run、rollback_plan、timeout。
5. 保存创建时的模板与步骤 snapshot，后续模板更新不改变历史任务。
6. 任务风险取操作默认、模板、步骤、prod command/script/custom 至少 medium 和用户 override 的最大值；override 只能提高。
7. 写 execution_created 时间线和 create_from_runbook 审计，确认/执行后回写 started/finished。

当前只有非 Agent 的模拟执行路径会按本模块展开 Runbook；选择 Agent 模式会进入独立 `createAgentTask`，不会按本模块展开模板。Command Spec/medium_selector 的模板字段和 Runbook→Agent 组合仍是规划能力。

## 6. 前端实现

- `/runbooks`：分页列表、关键字搜索、详情抽屉和启停开关；无新建、完整编辑、删除 UI。
- 告警详情：processing 时显示“推荐处置预案”，选择模板后填写可选任务名、dry-run 和 JSON 参数。
- dry-run 默认开启，只有推荐结果至少一步支持时显示开关；参数必须是 JSON object。
- 创建成功后进入 Execution 待确认/待执行流程，不在 Runbook 页面直接运行。
- 当前没有独立 Runbook 前端 Vitest；告警页测试主要验证详情布局与 AI 入口，不能替代 Runbook 交互覆盖。

## 7. 数据与审计

migration `0012_init_runbook` 创建：

- `runbook_template`：模板、匹配条件、风险、参数 schema、回滚与 enabled。
- `runbook_step`：有序步骤、动作、风险、dry-run、参数、回滚和 timeout。
- 扩展 `exec_task.runbook_template_id/runbook_snapshot/dry_run` 和 `exec_step.runbook_step_id/parameters/risk_level/dry_run/rollback_plan/timeout_seconds`。
- 种子 HighCPU + pod + prod 的三步 Restart service and verify 模板。

审计动作：

| resource_type | action |
| --- | --- |
| `runbook` | `runbook_create` / `runbook_update` / `runbook_delete` / `runbook_enable` / `runbook_disable` |
| `alert` | `runbook_recommend` |
| Execution Task | `create_from_runbook` |

Runbook 审计均为 best-effort，失败不回滚模板变更或推荐结果。

## 8. 验证覆盖

### 单元与 HTTP 测试

- 模板创建/更新/软停用、空步骤拒绝、禁用模板执行拒绝。
- 匹配、通配、模糊名称、推荐结果、风险排序与 prod command 风险。
- snapshot 源数据、推荐 AlertReader 错误、Create/推荐 Handler 与参数错误。
- Execution 单测另覆盖多步生成、风险、dry-run、时间线和 snapshot 持久化。

### E2E 的实际范围

`e2e-runbook.ps1` 覆盖：构造 processing 告警、推荐、创建 prod medium 多步任务、pending_confirm、CONFIRM、模拟执行成功、首步 dry_run 标记，以及 execution_created/started/finished 时间线。

它不覆盖模板 CRUD E2E、非 processing 拒绝、low 风险直接 pending_execute、步骤失败、完整前端交互、Command Spec 或 Runbook→Agent。上述项目需分别通过单元/HTTP 测试、专项接口或后续 Agent 验收记录。

本轮仅静态复核，未重新执行测试或脚本。
