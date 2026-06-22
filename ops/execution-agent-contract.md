# 执行介体与执行代理契约草案

> 本文是 P1+ 演进契约草案，用于描述 AI 分析产生建议后，如何在运维人员确认后，通过指定介体安全执行命令、脚本或诊断动作。当前 P0 执行链路仍以 `execution-contract.md` 的同步模拟执行为准。

## 1. 背景与目标

平台后续需要支持以下场景：

```text
AI/Agent 分析问题
  -> 生成诊断建议或处置计划
  -> 运维人员审查、选择执行介体、确认风险
  -> Execution 创建待确认任务
  -> 执行代理在指定介体或目标机器上执行
  -> 实时回传日志、结果、证据
  -> 审计与时间线闭环
```

这里的“介体”可以是：

- 一台专门的运维跳板虚拟机。
- 部署在某个网络域内的执行代理。
- 问题点目标机器自身。
- Kubernetes 集群内的诊断 Pod。
- 云厂商受控执行通道。

核心原则：

- AI 不直接执行命令。
- 前端不直接 SSH、K8s exec、云 API 写操作或数据库写操作。
- 所有真实执行必须进入 Execution 模块。
- 中高风险任务必须人工确认，确认文本沿用 `CONFIRM`。
- 执行介体必须注册、授权、健康检查和审计。

## 2. 核心概念

| 概念 | 说明 |
| --- | --- |
| Execution Medium | 执行介体，表示命令或脚本实际落地的位置，例如 jumpbox、target_host、k8s_pod、cloud_run_command |
| Execution Agent | 执行代理，部署在介体或网络域内，主动拉取任务并执行受控动作 |
| Agent Registration | 执行代理注册记录，包含身份、公钥、环境、可执行能力和心跳 |
| Command Spec | 命令规格，描述命令模板、参数 schema、风险、超时、输出脱敏规则 |
| Execution Lease | 执行租约，代理领取任务后的短期锁，避免多代理重复执行 |
| Execution Log Stream | 执行日志流，代理实时回传 stdout/stderr 摘要、状态和证据引用 |

## 3. DDD 上下文归属

该能力属于 Execution 上下文扩展：

```text
internal/execution/
  domain/
    agent.go
    medium.go
    command_spec.go
    lease.go
  application/
    agent_service.go
    dispatch_service.go
    command_policy.go
  infrastructure/
    persistence/
    agentqueue/
  interfaces/http/
    agent_handler.go
    medium_handler.go
  worker/
```

依赖边界：

- `interfaces/http` 只负责参数绑定、鉴权和响应转换。
- `application` 负责任务分发、介体选择、命令策略、租约和审计。
- `domain` 负责状态机、能力匹配、命令安全规则。
- `infrastructure` 负责数据库、队列、代理通信和日志流存储。

## 4. 执行介体类型

| medium_type | 说明 | 典型用途 | 风险基线 |
| --- | --- | --- | --- |
| `jumpbox` | 运维跳板机或专用诊断 VM | 内网 curl、dig、telnet、日志采样、只读诊断脚本 | medium |
| `target_host` | 问题点机器本身 | systemctl status、journalctl、df、top、只读诊断 | high |
| `kubernetes_pod` | K8s 内诊断 Pod 或目标 Pod exec | kubectl describe/logs/exec、网络诊断 | medium/high |
| `cloud_run_command` | 云厂商受控命令通道 | 云主机批量只读诊断或受控修复 | high |
| `database_readonly` | 数据库只读会话 | explain、慢查询采样、连接数检查 | high |

第一阶段建议只启用 `jumpbox` 和 `target_host` 的只读诊断命令，写操作、批量操作、数据库变更和云资源变更继续保持禁用。

## 5. 执行代理状态机

```text
registered -> online -> draining -> offline
registered -> disabled
online -> unhealthy -> offline
```

| 状态 | 说明 |
| --- | --- |
| `registered` | 已注册但未上线 |
| `online` | 心跳正常，可领取任务 |
| `draining` | 不再领取新任务，等待已有任务结束 |
| `offline` | 心跳超时 |
| `unhealthy` | 健康检查失败 |
| `disabled` | 管理员禁用 |

## 6. 执行任务扩展字段

`exec_task` 建议扩展：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `execution_mode` | string | `simulated` / `agent` / `adapter` |
| `medium_id` | string | 执行介体业务 ID |
| `agent_id` | string | 实际领取任务的代理 ID |
| `dispatch_status` | string | `pending_dispatch` / `leased` / `dispatched` / `dispatch_failed` |
| `lease_id` | string | 当前执行租约 ID |
| `command_policy_id` | string | 命令策略 ID |

`exec_step` 建议扩展：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `command_spec_id` | string | 命令规格 ID |
| `command_template` | string | 命令模板，不能直接保存未校验自由文本 |
| `arguments` | object | 参数对象，按 schema 校验 |
| `timeout_seconds` | number | 超时 |
| `working_dir` | string | 可选工作目录 |
| `output_redaction` | object | 输出脱敏规则 |
| `requires_tty` | boolean | 默认 false |

## 7. 命令安全模型

### 7.1 命令来源

允许：

- 平台预置 Command Spec。
- Runbook 模板引用 Command Spec。
- AI 生成参数化执行计划，但必须匹配已有 Command Spec。

禁止：

- AI 直接提交自由命令字符串并执行。
- 前端直接传任意 shell 命令。
- 将用户输入拼接成 shell 字符串。

### 7.2 Command Spec 示例

```json
{
  "command_spec_id": "cmd_linux_disk_usage",
  "name": "检查磁盘使用率",
  "action_type": "diagnose",
  "medium_types": ["jumpbox", "target_host"],
  "risk_level": "low",
  "command_template": "df -h {mount_point}",
  "argument_schema": {
    "type": "object",
    "properties": {
      "mount_point": {
        "type": "string",
        "pattern": "^/[A-Za-z0-9_./-]*$"
      }
    },
    "required": ["mount_point"]
  },
  "timeout_seconds": 10,
  "allowed_exit_codes": [0],
  "output_redaction": {
    "patterns": ["(?i)password=.*", "(?i)token=.*"]
  }
}
```

### 7.3 风险规则

| 场景 | 最低风险 |
| --- | --- |
| `target_host` 介体 | high |
| prod 环境执行 `script` / `command` / `custom` | medium |
| 写文件、重启服务、变更配置 | high |
| 删除、扩缩容、权限变更、数据库写操作 | critical |
| 只读诊断命令 | low/medium，按介体和环境提升 |

用户只能提高风险，不能降低风险。

## 8. API 草案

### 8.1 注册执行介体

- Method: `POST`
- Path: `/api/executions/media`
- Auth: Bearer + `app:executions:agent.manage`

请求体：

```json
{
  "medium_id": "med-prod-jumpbox-01",
  "name": "生产跳板机 01",
  "medium_type": "jumpbox",
  "environment": "prod",
  "region": "cn-north-4",
  "network_zone": "prod-vpc-a",
  "capabilities": ["linux.command.readonly", "network.diagnose"],
  "enabled": true,
  "description": "生产网络域只读诊断介体"
}
```

响应不返回任何介体登录凭据。

### 8.2 注册执行代理

- Method: `POST`
- Path: `/api/executions/agents/register`
- Auth: bootstrap token 或一次性注册令牌

请求体：

```json
{
  "agent_id": "agent-prod-jumpbox-01",
  "medium_id": "med-prod-jumpbox-01",
  "public_key": "base64-public-key",
  "version": "0.1.0",
  "capabilities": ["linux.command.readonly", "network.diagnose"]
}
```

注册令牌必须短期有效，使用后失效。

### 8.3 代理心跳

- Method: `POST`
- Path: `/api/executions/agents/:agent_id/heartbeat`
- Auth: agent mTLS 或 agent token

请求体：

```json
{
  "status": "online",
  "running_tasks": 1,
  "free_slots": 3,
  "version": "0.1.0",
  "observed_at": 1710000000
}
```

### 8.4 创建带介体的执行任务

扩展 `POST /api/executions/tasks`：

```json
{
  "name": "检查 payment 主机磁盘",
  "source_type": "inspection",
  "source_id": "rec_xxx",
  "operation_type": "command",
  "target_type": "host",
  "target_id": "host-prod-01",
  "target_name": "host-prod-01",
  "environment": "prod",
  "execution_mode": "agent",
  "medium_id": "med-prod-jumpbox-01",
  "parameters": {
    "command_spec_id": "cmd_linux_disk_usage",
    "arguments": {
      "mount_point": "/"
    }
  },
  "risk_level": "high",
  "rollback_plan": {
    "description": "只读诊断命令，无需回滚"
  }
}
```

创建要求：

- `execution_mode=agent` 时必须指定 `medium_id` 或满足自动选择策略。
- `command_spec_id` 必须存在且启用。
- `arguments` 必须通过 schema 校验。
- `medium_id` 的能力必须覆盖 Command Spec 需要的能力。
- 中高风险进入 `pending_confirm`。

### 8.5 代理领取任务

- Method: `POST`
- Path: `/api/executions/agents/:agent_id/lease`
- Auth: agent mTLS 或 agent token

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
  },
  "redaction": {
    "patterns": ["(?i)password=.*", "(?i)token=.*"]
  }
}
```

代理侧禁止自行解释平台未下发的命令。

### 8.6 回传日志

- Method: `POST`
- Path: `/api/executions/agents/:agent_id/tasks/:task_id/logs`
- Auth: agent mTLS 或 agent token

请求体：

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

服务端必须再次执行脱敏，不能完全信任代理侧脱敏。

### 8.7 回传结果

- Method: `POST`
- Path: `/api/executions/agents/:agent_id/tasks/:task_id/result`
- Auth: agent mTLS 或 agent token

请求体：

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

## 9. 调度与介体选择

介体选择顺序：

1. 用户在执行确认页显式选择的 `medium_id`。
2. Recommendation 或 Runbook 指定的 `medium_selector`。
3. 平台根据目标资源的 environment、region、network_zone、capabilities 自动匹配。

自动匹配必须可解释，执行确认页展示匹配原因。

## 10. 审计动作

| resource_type | action |
| --- | --- |
| `execution_medium` | `create` / `update` / `disable` / `delete` |
| `execution_agent` | `register` / `heartbeat` / `disable` |
| `execution_command_spec` | `create` / `update` / `disable` |
| `execution` | `dispatch` / `lease` / `agent_log` / `agent_result` |

审计字段建议：

- `task_id`
- `step_id`
- `medium_id`
- `agent_id`
- `command_spec_id`
- `risk_level`
- `actor_user_id`
- `approved_by`
- `result`
- `exit_code`
- `duration_ms`

禁止写入：

- SSH 私钥、密码、Token。
- 完整敏感命令输出。
- 原始 Authorization header。

## 11. 页面要求

执行确认页必须增加：

- 执行模式：模拟 / 执行代理 / 外部适配器。
- 执行介体：名称、类型、环境、网络域、健康状态。
- 自动匹配原因。
- 命令规格：Command Spec 名称、参数、风险、超时。
- 只读/写操作标识。
- 输出脱敏说明。
- 失败接管方式。

任务详情页必须增加：

- 代理领取记录。
- 实时日志流。
- stdout/stderr 分流。
- exit_code。
- 介体和代理信息。
- 每个步骤的证据引用。

## 12. 验收规划

| 脚本 | 范围 |
| --- | --- |
| `scripts/e2e-execution-agent.ps1` | 介体 CRUD、命令规格、任务确认、fake agent 领取、日志和结果回传 |
| `scripts/e2e-execution-agent-permission.ps1` | 无权限管理介体、无权限执行、风险确认失败 |

必须覆盖：

- AI 生成建议不能直接执行。
- 未确认任务不能被代理领取。
- 禁用介体不能执行。
- 参数 schema 不通过时任务创建失败。
- 日志和结果脱敏。
- 代理重复回传结果幂等。
