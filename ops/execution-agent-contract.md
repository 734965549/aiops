# 执行介体与执行代理契约（服务端控制面）

本文档描述 Execution Medium、Execution Agent、Command Spec、Lease 和 Log Stream 的当前服务端实现。平台提供控制面与 fake Agent E2E，但仓库不提供独立 Agent 可执行程序；不能把协议已落地等同于真实执行产品已完整交付。

## 1. 当前实现状态

| 能力 | 状态 | 当前边界 |
| --- | --- | --- |
| Medium 创建/查询 | 已落地 | 无 update/disable/delete HTTP 管理接口 |
| Command Spec 查询 | 已落地 | 使用 migration 0022 种子的三个只读规格，无管理写接口 |
| Agent 注册/心跳 | 已落地 | 静态注册令牌 + Agent Token；无 mTLS、自动离线 worker 或管理禁用接口 |
| Agent 任务创建 | 已落地 | 必须显式 medium_id 和 command_spec_id，生成单步任务 |
| 租约/日志/结果 | 已落地 | 默认 300 秒租约，服务端日志脱敏；无续租/过期重派 worker |
| 用户侧日志查询 | 已落地 | 必须传 step_id；前端未封装和展示 |
| 独立 Agent 进程 | 未提供 | E2E 通过 PowerShell 直接调用协议，不在目标机器真实执行命令 |
| 自动介体选择 | 未实现 | 没有 medium_selector、自动匹配原因或确认页选择器 |
| Runbook→Agent | 未实现 | Agent 走独立 createAgentTask，不展开 Runbook |
| Agent 告警时间线 | 未闭环 | 创建/lease/result 不写 execution_created/started/finished |
| 专门前端 | 未实现 | 无 Medium/Agent/Spec 管理、租约视图或实时日志流 |

核心原则不变：AI 和前端不能直接 SSH、K8s exec、调用云写 API 或数据库写操作；真实动作必须进入 Execution 状态机。migration 0022 将 `execution.tasks.dispatch` AI 工具权限设为 deny。

## 2. 核心概念与数据

| 概念 | 当前实现 |
| --- | --- |
| Execution Medium | exec_medium，记录类型、环境、网络域、能力、allowed_command_ids、max_risk_level、enabled、health_status |
| Execution Agent | exec_agent，记录 medium、状态、public_key、token_hash、版本、能力、心跳、disabled |
| Command Spec | exec_command_spec，记录受控命令模板、参数 schema、风险、超时、退出码、脱敏规则、所需能力 |
| Execution Lease | exec_lease，绑定 task/step/agent/medium，active/released 和 expires_at |
| Execution Log Stream | exec_log_stream，保存 stdout/stderr、sequence、脱敏后 content、truncated、redacted |

Medium 类型：jumpbox、target_host、kubernetes_pod、cloud_run_command、database_readonly。当前种子 Spec 只面向 jumpbox/target_host 的只读 Linux 诊断，不代表其他类型已有真实 Adapter。

Agent 状态值：registered、online、draining、offline、unhealthy、disabled。只有 online 且 disabled=false 可 lease；状态主要由注册和心跳写入，当前没有自动心跳超时转 offline 的周期 worker。

任务扩展字段：execution_mode、medium_id、agent_id、dispatch_status、lease_id、command_spec_id。步骤扩展 command_spec_id、command_template、arguments、output_redaction、working_dir、requires_tty。`command_policy_id` 不属于当前字段。

## 3. 用户侧管理 API

### 3.1 Medium

| Method/Path | 权限 | 当前语义 |
| --- | --- | --- |
| `GET /api/executions/media` | `app:executions:media:read` | 分页，支持 enabled/environment/medium_type/keyword |
| `GET /api/executions/media/:medium_id` | `app:executions:media:read` | 介体详情 |
| `POST /api/executions/media` | `app:executions:media:create` | 创建介体，不返回登录凭据 |

Medium 创建权限不是 `app:executions:agent.manage`。migration 0022 虽种子 media update/delete 权限，但当前没有对应路由。

创建请求：

```json
{
  "medium_id": "med-prod-jumpbox-01",
  "name": "生产跳板机 01",
  "medium_type": "jumpbox",
  "environment": "prod",
  "region": "cn-north-4",
  "network_zone": "prod-vpc-a",
  "capabilities": ["linux.command.readonly"],
  "allowed_command_ids": ["cmd_linux_disk_usage"],
  "max_risk_level": "high",
  "enabled": true,
  "description": "生产网络域只读诊断介体"
}
```

### 3.2 Command Spec

| Method/Path | 权限 |
| --- | --- |
| `GET /api/executions/command-specs` | `app:executions:command_specs:read` |
| `GET /api/executions/command-specs/:command_spec_id` | `app:executions:command_specs:read` |

当前没有创建/更新/禁用 Command Spec 的 HTTP 路由。种子规格：cmd_linux_disk_usage、cmd_linux_memory_snapshot、cmd_linux_cpu_snapshot。

### 3.3 用户侧日志查询

`GET /api/executions/tasks/:task_id/logs?step_id=<step_id>`，权限 `app:executions:read`，step_id 必填。返回脱敏后的日志条目，不是实时 WebSocket/SSE 流。

## 4. Agent 注册与鉴权

### 4.1 静态注册令牌

`POST /api/executions/agents/register` 是 Public 路由，不使用用户 Bearer。调用方通过以下任一方式提供静态注册令牌：

- `X-Register-Token: <token>`
- `Authorization: Bearer <token>`

令牌来自 `execution.agent_register_token`：

- dev/test 可使用开发默认值。
- prod 必须显式配置，配置校验要求长度、字符多样性和熵。
- 当前令牌可复用，不是短期一次性令牌，使用后不会失效。
- 空值或不匹配返回 PERMISSION_DENIED。

### 4.2 注册请求与 Agent Token

```json
{
  "agent_id": "agent-prod-jumpbox-01",
  "medium_id": "med-prod-jumpbox-01",
  "public_key": "base64-public-key",
  "version": "0.1.0",
  "capabilities": ["linux.command.readonly"]
}
```

agent_id 可省略由服务端生成，Medium 必须存在且 enabled。注册成功返回 agent_id、medium_id、status=registered 和随机 `agt_` Agent Token。数据库只保存 token SHA-256 hash，明文只在注册响应返回一次。

后续 heartbeat/lease/log/result 使用 `Authorization: Bearer <agent_token>`。当前没有 mTLS 或 public_key 签名验证，public_key 仅持久化，不参与鉴权。

### 4.3 心跳

`POST /api/executions/agents/:agent_id/heartbeat`，Agent Token 鉴权：

```json
{
  "status": "online",
  "running_tasks": 0,
  "free_slots": 2,
  "version": "0.1.0",
  "observed_at": 1710000000
}
```

心跳更新状态、running_tasks/free_slots/version/last_heartbeat，并把 Medium health_status 置 online。observed_at 当前不作为服务端心跳时间来源。FreeSlots 会保存并在 lease 时递减，但不是完整并发调度器。

## 5. 创建 Agent 任务

`POST /api/executions/tasks`，用户权限 `app:executions:create`。当前使用顶层 execution_mode、medium_id、command_spec_id、arguments：

```json
{
  "name": "检查 payment 主机磁盘",
  "source_type": "manual",
  "operation_type": "command",
  "target_type": "host",
  "target_id": "host-prod-01",
  "target_name": "host-prod-01",
  "environment": "prod",
  "execution_mode": "agent",
  "medium_id": "med-prod-jumpbox-01",
  "command_spec_id": "cmd_linux_disk_usage",
  "arguments": { "mount_point": "/" },
  "risk_level": "high",
  "rollback_plan": { "description": "只读诊断命令，无需回滚" }
}
```

也兼容从 parameters 中读取 medium_id、command_spec_id、arguments。

创建要求：

- medium_id 和 command_spec_id 均必填，当前没有自动选择。
- Medium enabled，medium_type、allowed_command_ids、capabilities 满足 Spec。
- Spec enabled，arguments 满足 schema/pattern，命令可成功渲染。
- target_host、cloud_run_command、database_readonly 至少 high；prod 至少 medium；不得超过 Medium max_risk_level；override 只能提高。
- low 可直接 pending_execute/pending_dispatch，其余 pending_confirm。
- 生成单个 Agent Step，保存命令模板、参数、脱敏规则和 timeout。

Agent 路径不走 Runbook 展开，也不写 Alert execution_created。即使 source_type=alert，也不能宣称已有完整 Alert 时间线。

## 6. 命令安全模型

允许的来源是数据库中已启用的 Command Spec。Runbook 引用 Command Spec 属于规划，不是当前能力。

安全校验：

- 参数 schema 与字符串 pattern。
- 参数和渲染结果禁止 shell 元字符 `;&|$`、反引号、重定向符。
- 模板必须无未解析占位符。
- 使用受控模板和参数生成 argv，不允许前端/AI 提交自由 shell 字符串。
- 退出码必须命中 allowed_exit_codes，未配置时仅 0 为成功。
- 日志由服务端按步骤和 Spec 双重脱敏，不能只信任 Agent。

这是一套参数化 argv 控制面，不代表仓库内已有真实执行沙箱、命令白名单进程或资源隔离实现。

## 7. 租约、日志与结果

### 7.1 领取任务

`POST /api/executions/agents/:agent_id/lease`，Agent Token 鉴权。

- 仅同 Medium、pending_execute、pending_dispatch 的 Agent 任务可领取。
- pending_confirm 返回空 data 对象，不得提前执行。
- 创建默认 300 秒 active lease，任务转 running/leased，步骤转 running。
- 当前只处理单个 Agent Step，不展开 Runbook 多步。

响应：

```json
{
  "lease_id": "lease_xxx",
  "task_id": "task_xxx",
  "step_id": "step_xxx",
  "command": {
    "argv": ["df", "-h", "/"],
    "timeout_seconds": 10,
    "working_dir": "",
    "env": {}
  }
}
```

当前没有独立 redaction 响应字段，也没有续租 API、过期重派或失败接管 worker。

### 7.2 回传日志

`POST /api/executions/agents/:agent_id/tasks/:task_id/logs`，Agent Token 鉴权：

```json
{
  "lease_id": "lease_xxx",
  "step_id": "step_xxx",
  "stream": "stdout",
  "sequence": 12,
  "content": "Filesystem Size Used Avail Use% Mounted on\n...",
  "truncated": false,
  "observed_at": 1710000001
}
```

服务端校验 lease 属于该 Agent/任务、active 且未过期，步骤属于任务；随后执行步骤与 Spec 双重脱敏并保存。成功响应 `{ "accepted": true }`。

### 7.3 回传结果

`POST /api/executions/agents/:agent_id/tasks/:task_id/result`，Agent Token 鉴权：

```json
{
  "lease_id": "lease_xxx",
  "step_id": "step_xxx",
  "status": "success",
  "exit_code": 0,
  "result_summary": "根分区使用率 61%",
  "started_at": 1710000000,
  "finished_at": 1710000003
}
```

服务端校验退出码，更新步骤和任务 success/failed、exit_code/result_summary，并释放 lease。已终态任务重复请求返回当前详情，不再次执行。

当前 Agent lease/result 不写 Alert execution_started/finished；dispatch/agent_log/agent_result 审计存在，但不能替代 Alert 时间线。

## 8. 调度与介体选择边界

早期规划的选择顺序为：用户确认页 medium_id → Recommendation/Runbook medium_selector → environment/region/network_zone/capabilities 自动匹配。

当前只有显式 medium_id 路径。没有 medium_selector 字段、自动匹配算法、可解释匹配原因或确认页选择器，不能将本节早期规划写成已实现。

## 9. 审计动作

| resource_type | 当前 action |
| --- | --- |
| `execution_medium` | `create` |
| `execution_agent` | `register` / `heartbeat` |
| `execution` | `create` / `confirm` / `reject`（按任务操作） |
| `execution` | `dispatch` / `agent_log` / `agent_result` |

Medium update/disable/delete、Agent disable、Command Spec create/update/disable 没有当前管理路由，相关审计枚举或权限种子不能视为功能完成。

审计记录 task/step/medium/agent/spec、风险、状态、退出码等业务摘要；禁止记录 SSH 私钥、密码、Token、原始 Authorization header 或完整敏感日志。

## 10. 前端边界

当前 `/executions` 只有普通任务列表、详情、CONFIRM 和同步执行。没有：

- 模拟/Agent/Adapter 执行模式选择器。
- Medium/Agent/Command Spec 管理页。
- 自动匹配原因、介体健康与租约专门视图。
- Agent 参数表单、实时 stdout/stderr 日志流和证据引用展示。

`web/src/api/execution.ts` 未封装 execution_mode/medium_id/command_spec_id/arguments、Medium、Command Spec 或任务日志。没有独立 Execution 前端 Vitest。

## 11. 验证覆盖

### 单元与 HTTP 测试

现有测试覆盖：

- 注册令牌为空、缺失、错误时拒绝。
- Agent Token 注册后 hash 鉴权和 online 心跳。
- argv 渲染、shell 元字符拒绝、输出脱敏。
- 核心任务风险、状态机、确认和模拟执行。

当前没有完整的 DispatchService 应用层测试组，租约/日志/结果主要依赖 fake Agent E2E。

### E2E 的实际范围

`e2e-execution-agent.ps1` 覆盖：

1. 创建 jumpbox Medium。
2. 使用静态注册令牌注册 fake Agent，获取 Agent Token。
3. 心跳 online。
4. 创建 manual high Agent 任务。
5. pending_confirm 时 lease 为空，CONFIRM 后成功 lease。
6. 追加一条 stdout，上报 success result。
7. 非法 mount_point 参数被拒绝。

它不调用 Command Spec 查询、不读取日志或断言脱敏、不检查 argv 具体内容/审计、不测试结果幂等、不禁用 Medium/Agent、不运行真实命令，也不覆盖 Alert 时间线或 Runbook→Agent。

`e2e-execution-agent-permission.ps1` 实际只验证错误注册令牌返回 PERMISSION_DENIED；没有创建低权限用户验证 Medium/Execution 403，不能按脚本名推断权限矩阵已覆盖。

真实 Agent、自动选择、管理 UI、日志流、完整脱敏/幂等/禁用/时间线验收均需单独补实现或专项脚本。本轮仅静态复核，未执行任何测试或 E2E。
