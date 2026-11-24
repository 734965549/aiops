# internal/execution 模块说明

Execution 负责执行任务与步骤状态机、风险、人工确认/拒绝、同步模拟执行、Runbook 展开、告警时间线，以及 Execution Agent 的服务端控制面。稳定契约见 `ops/execution-contract.md`、`ops/execution-agent-contract.md`。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 任务闭环 | alert/manual/ai_conversation/inspection 来源，创建、列表、详情、确认、拒绝、模拟执行 | alert 来源必须 processing；多级审批和任意取消未实现 |
| 状态机 | pending_confirm → pending_execute → running → success/failed；confirm=false 可转 cancelled | cancelled 是已实现的拒绝结果，不再只是预留；前端没有拒绝按钮 |
| Runbook | 非 Agent 路径展开多步骤、保存 snapshot、dry-run、风险聚合 | Agent 路径不会展开 Runbook，Command Spec 模板关联见 Runbook 契约 |
| 模拟执行 | 顺序执行步骤，失败停止，写任务/步骤结果和 Alert 时间线 | 不调用真实 SSH/K8s/云 API；adapter 枚举保留但没有独立实现 |
| Medium | 创建、分页列表、详情、能力/命令白名单/风险上限 | 没有 update/disable/delete HTTP 管理接口和专门前端 |
| Command Spec | 查询迁移种子的受控命令规格，schema/argv/能力/风险校验 | 没有创建/更新/禁用管理接口；不支持自由 shell |
| Agent 控制面 | 注册、心跳、租约、日志、结果、token hash、服务端脱敏 | 仓库没有独立 Agent 可执行程序；mTLS、自动介体选择、周期健康 worker 未实现 |
| Agent 日志查询 | `GET /tasks/:task_id/logs?step_id=` | 前端未封装和展示日志流；E2E 未验证读取与脱敏结果 |
| Agent 告警时间线 | 当前未闭环 | Agent 创建/lease/result 不写 Alert execution_created/started/finished，需单独补实现 |
| 数据范围 | 所有用户路由使用静态 RBAC | 没有任务对象级数据范围过滤；Agent 使用独立 token，不继承用户身份 |

## 2. 分层与运行形态

```text
interfaces/http
  -> Handler          任务、Medium、Command Spec、日志查询
  -> AgentHandler     Agent 注册/心跳/租约/日志/结果

application
  -> TaskService      创建、确认/拒绝、模拟执行、Runbook、Alert 时间线
  -> MediumService    介体创建与查询
  -> CommandSpecService 受控命令规格查询
  -> AgentService     Agent 注册、token hash、心跳
  -> DispatchService  租约、argv、日志、结果与审计

infrastructure
  -> PostgreSQL       exec_task / exec_step / exec_medium / exec_agent
                      exec_command_spec / exec_lease / exec_log_stream

外部执行端
  -> 仓库只提供服务端协议和 PowerShell fake Agent E2E
  -> 没有 cmd/execution-agent
```

## 3. 用户侧路由与权限

### 3.1 任务

| 路由 | 权限 |
| --- | --- |
| `GET /api/executions/tasks` | `app:executions:read` |
| `GET /api/executions/tasks/:task_id` | `app:executions:read` |
| `GET /api/executions/tasks/:task_id/logs?step_id=` | `app:executions:read` |
| `POST /api/executions/tasks` | `app:executions:create` |
| `POST /api/executions/tasks/:task_id/confirm` | `app:executions:confirm` |
| `POST /api/executions/tasks/:task_id/execute` | `app:executions:execute` |

### 3.2 Medium 与 Command Spec

| 路由 | 权限 |
| --- | --- |
| `GET /api/executions/media` | `app:executions:media:read` |
| `GET /api/executions/media/:medium_id` | `app:executions:media:read` |
| `POST /api/executions/media` | `app:executions:media:create` |
| `GET /api/executions/command-specs` | `app:executions:command_specs:read` |
| `GET /api/executions/command-specs/:command_spec_id` | `app:executions:command_specs:read` |

Medium 创建权限不是 `app:executions:agent.manage`。migration 0022 虽种子了 media update/delete、agents manage 和 Command Spec 写权限，但当前没有对应 HTTP 路由，不能把权限种子当成管理功能已完成。

## 4. 核心任务状态机

### 4.1 来源与操作

- 来源：alert、manual、ai_conversation、inspection，均已有代码使用路径，不再只是预留。
- 操作：restart、scale、script、runbook、command、custom。
- `diagnose` 不是当前任务 operation_type；Command Spec 可使用 action_type=diagnose，但 Agent 任务落为 operation_type=command。
- 执行模式：simulated 默认、agent 已有控制面；adapter 只是枚举保留，没有独立适配器执行链路。

### 4.2 风险与确认

风险为 low/medium/high/critical。普通任务按操作和环境计算，prod script/runbook 至少 medium；Runbook 和 Agent 另叠加模板/步骤或 Medium/Spec 风险。用户 override 只能提高。

- low：pending_execute。
- medium/high/critical：pending_confirm。
- confirm=true 且 confirm_text 必须为大小写敏感的 CONFIRM：pending_confirm → pending_execute。
- confirm=false：pending_confirm → cancelled，并写 reject 审计。
- 没有 pending_execute/running 的通用取消接口。

### 4.3 模拟执行

`POST .../execute` 仅接受 pending_execute，Agent 任务调用此接口返回 FAILED_PRECONDITION。模拟执行按步骤串行：

1. pending_execute → running，写 execution_started。
2. 每步 running → success/failed；失败停止，后续步骤保持 pending。
3. 全部成功任务 success，否则 failed。
4. 写 execution_finished；dry-run 输出按 Runbook 规则生成。

任务与步骤创建使用事务，步骤父任务、task_id、step_order 和零步骤场景有完整性校验。

## 5. Agent 服务端控制面

### 5.1 注册与鉴权

`POST /api/executions/agents/register` 不走用户 Bearer，而是：

- `X-Register-Token`，或 Authorization Bearer 中的静态注册令牌。
- 注册令牌来自 `execution.agent_register_token`，dev/test 可有默认值，prod 必须配置强令牌。
- 当前是可复用静态配置，不是短期一次性令牌，也不会使用后失效。
- 注册成功生成随机 `agt_` Agent Token，只存 SHA-256 hash，明文只在注册响应返回。

后续 heartbeat/lease/log/result 使用 Agent Token Bearer。当前没有 mTLS 或 public_key 签名验证，public_key 只是存储字段。

### 5.2 创建 Agent 任务

`POST /api/executions/tasks` 使用顶层字段：

```json
{
  "name": "检查磁盘",
  "source_type": "manual",
  "operation_type": "command",
  "execution_mode": "agent",
  "medium_id": "med-prod-jumpbox-01",
  "command_spec_id": "cmd_linux_disk_usage",
  "arguments": { "mount_point": "/" },
  "environment": "prod",
  "risk_level": "high"
}
```

也兼容从 parameters 中读取 medium_id、command_spec_id、arguments。当前 medium_id 和 command_spec_id 都是必填，不存在自动介体选择。校验：

- Medium enabled，类型、allowed_command_ids、capabilities 和 max_risk_level 满足 Spec。
- Command Spec enabled，arguments 满足 schema/pattern。
- 参数与渲染结果禁止 shell 元字符，最终只生成 argv，不交给 shell 拼接执行。
- 生成单个 Agent Step，保存 command_template、arguments、output_redaction 和 timeout。

Agent 路径不走 Runbook 展开，也不写 Alert execution_created；这是当前与早期闭环目标的差距。

### 5.3 心跳与状态

- 注册后状态 registered，heartbeat 可上报 online/draining/offline/unhealthy 等合法状态。
- 只有 online 且未 disabled 的 Agent 可 lease。
- 心跳更新 running_tasks/free_slots/version/last_heartbeat，并把 Medium health_status 置 online。
- 没有周期心跳超时监控自动转 offline，也没有管理员禁用 Agent 的 HTTP 接口。
- FreeSlots 字段会保存和递减，但当前 lease 准入主要看 online 状态，不是完整调度器。

### 5.4 租约

`POST /api/executions/agents/:agent_id/lease`：

- 只领取同 Medium、pending_execute、pending_dispatch 的 Agent 任务。
- pending_confirm 返回空结果，不能提前领取。
- 创建默认 300 秒租约，任务转 running/leased，步骤转 running。
- 返回 lease_id、task_id、step_id 和受控 command.argv/timeout/working_dir/env。
- 当前响应没有独立 redaction 字段；服务端仍在日志写入时执行步骤和 Spec 双重脱敏。
- 没有租约续租 API 或周期过期重派 worker。

### 5.5 日志与结果

- 日志：校验 Agent/任务/步骤/租约归属和 active 状态，服务端再次脱敏，保存 stdout/stderr、sequence、truncated、redacted。
- 查询：用户侧 `GET /tasks/:task_id/logs?step_id=<step_id>`，step_id 必填。
- 结果：校验 allowed_exit_codes，更新步骤与任务 success/failed、exit_code/result_summary，并释放租约。
- 已终态任务重复结果请求返回当前详情，不再次执行。
- dispatch/agent_log/agent_result 写审计，日志正文不进入审计 payload。

当前 Agent 结果不会回写 Alert execution_started/finished，也没有专门前端日志流。

## 6. 前端实现

`/executions` 当前支持：

- 任务分页、状态/来源筛选、详情抽屉、步骤与输出。
- pending_confirm 的 CONFIRM 确认弹窗。
- pending_execute 的同步执行按钮和成功/失败展示。
- Runbook 名称/dry-run 标记、`/executions?task_id=` 深链和 AI 页面上下文。

当前未提供：拒绝按钮、手工 Agent 任务创建表单、Medium/Agent/Command Spec 管理页、自动匹配原因、租约/Agent 信息专门视图和实时日志流。`web/src/api/execution.ts` 也未封装日志、Medium、Command Spec 或 Agent 字段。没有独立 Execution 前端 Vitest。

## 7. 数据与审计

主要迁移：

| 版本 | 作用 |
| --- | --- |
| `0011` | exec_task / exec_step 与基础权限 |
| `0012` | Runbook snapshot、多步参数、风险、dry-run、rollback、timeout 扩展 |
| `0022` | exec_medium / exec_agent / exec_command_spec / exec_lease / exec_log_stream 和 Agent 字段、权限、三个只读命令规格 |

审计：

| resource_type | 当前 action |
| --- | --- |
| `execution` | `create` / `confirm` / `reject` / `execute` / `create_from_runbook` |
| `alert` | `execution_create`（普通 alert 任务创建时） |
| `execution_medium` | `create`；update/disable 常量存在但无管理路由 |
| `execution_agent` | `register` / `heartbeat` |
| `execution` | `dispatch` / `agent_log` / `agent_result` |

Command Spec 管理、Medium 更新/禁用/删除和 Agent 禁用审计属于未提供管理功能，不因枚举或权限种子存在而视为完成。

## 8. 验证覆盖

### 单元、集成与 HTTP 测试

- 任务状态机、风险、重复确认/执行、非 processing 拒绝、无步骤失败和步骤完整性。
- Runbook 多步生成、snapshot、dry-run、Alert 时间线和持久化。
- 注册 token 拒绝、Agent Token 注册/心跳、Command argv/shell 元字符/输出脱敏。
- 当前没有 DispatchService 租约/日志/结果的完整应用层测试组，主要依赖 fake Agent E2E。

### E2E 的实际范围

- `e2e-execution.ps1`：processing alert、restart pending_confirm、CONFIRM、模拟 success、三类 Alert 时间线。
- `e2e-runbook.ps1`：Runbook 多步 dry-run 模拟闭环。
- `e2e-execution-agent.ps1`：创建 Medium、静态 token 注册 fake Agent、心跳、manual high Agent 任务、未确认空 lease、确认后 lease、追加一条 stdout、success result、非法参数拒绝。
- `e2e-execution-agent-permission.ps1`：实际只验证错误注册令牌返回 PERMISSION_DENIED；没有创建低权限用户验证 Medium/Execution 403。

Agent E2E 不覆盖真实 Agent 进程或真实命令执行、Medium/Command Spec CRUD、自动选择、日志读取/脱敏断言、审计断言、结果幂等、禁用介体/Agent、Alert 时间线或 Runbook→Agent。上述项目不得从脚本 PASS 推断完成。

本轮仅静态复核，未重新执行测试或脚本。
