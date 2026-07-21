# AI 模块对外 API 契约

## 1. 范围

本文档描述 AI 模块当前对外暴露的 provider 管理接口与工具调用接口，供前端联调、联调环境配置和后续运维接入使用。

## 2. 统一响应格式

与平台其它接口一致，使用 `httpx.OK` / `httpx.Fail` 封装；成功时 `code` 为字符串 `"OK"`（**不是**数字 `0`）。完整字段说明与 `/healthz`、`/readyz` 示例见 `health-contract.md`。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | string | 成功为 `"OK"`；失败为业务错误码 |
| `message` | string | 成功为 `"ok"` |
| `trace_id` | string | 请求追踪 ID |
| `data` | object | 业务数据 |

## 3. 权限模型

所有 AI 接口均挂载在 `/api/ai` 路由组下，统一要求已登录用户访问，并在进入业务逻辑前执行统一授权校验：

- RBAC：检查当前用户是否具备对应角色权限；
- 数据权限：检查目标对象是否落在允许的数据范围内；
- 工具权限：检查当前工具是否允许调用、是否需要人工确认。

如果授权失败，HTTP 返回 **403**，`code` 为 `PERMISSION_DENIED`（与 HTTP 401 `UNAUTHENTICATED` 区分）。

## 4. Provider 管理接口

### 4.1 获取 provider 列表

- Method: `GET`
- Path: `/api/ai/providers`
- 鉴权: 是

#### 响应示例

列表接口**不返回** `api_key` 字段；通过 `has_api_key` 表示是否已配置密钥。

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": [
    {
      "id": "demo-http-a",
      "name": "Demo HTTP Provider A",
      "type": "a",
      "base_url": "http://127.0.0.1:9000",
      "has_api_key": true,
      "timeout_ms": 30000,
      "headers": {
        "X-Client-Name": "aiops"
      },
      "enabled": true,
      "description": "通用 HTTP API Key 工具提供方样例"
    }
  ]
}
```

### 4.2 新增或更新 provider

- Method: `POST`
- Path: `/api/ai/providers`
- 鉴权: 是

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | provider 唯一标识 |
| `name` | string | 是 | provider 显示名称 |
| `type` | string | 是 | provider 类型，`a` / `b` / `c` |
| `base_url` | string | 是 | provider 基础地址 |
| `api_key` | string | 新增必填；更新可省略 | 调用凭据；更新时省略则保留原密钥 |
| `timeout_ms` | number | 否 | 请求超时时间，默认 30000 |
| `headers` | object | 否 | 额外请求头 |
| `enabled` | boolean | 否 | 是否启用 |
| `description` | string | 否 | 备注说明 |

#### 请求示例

```json
{
  "id": "demo-http-a",
  "name": "Demo HTTP Provider A",
  "type": "a",
  "base_url": "http://127.0.0.1:9000",
  "api_key": "demo-api-key",
  "timeout_ms": 30000,
  "headers": {
    "X-Client-Name": "aiops"
  },
  "enabled": true,
  "description": "通用 HTTP API Key 工具提供方样例"
}
```

### 4.3 删除 provider

- Method: `DELETE`
- Path: `/api/ai/providers/:id`
- 鉴权: 是

## 5. 工具调用接口

### 5.1 调用工具

- Method: `POST`
- Path: `/api/ai/tools/invoke`
- 鉴权: 是

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `provider_id` | string | 是 | 使用哪个 provider |
| `tool_code` | string | 是 | 工具编码 |
| `resource` | string | 否 | 资源域 |
| `action` | string | 否 | 动作 |
| `owner_id` | string | 否 | 数据归属人 |
| `dept` | string | 否 | 部门 |
| `team` | string | 否 | 团队 |
| `region` | string | 否 | 区域 |
| `tags` | array<string> | 否 | 标签 |
| `confirmed` | boolean | 否 | 是否已人工确认 |
| `payload` | object | 否 | 工具调用负载 |

#### 请求示例

```json
{
  "provider_id": "demo-http-a",
  "tool_code": "alarm.analyze",
  "resource": "alarm",
  "action": "analyze",
  "dept": "platform",
  "confirmed": true,
  "payload": {
    "alarm_id": "a-123",
    "severity": "critical"
  }
}
```

#### 响应说明

HTTP 200 且 `code` 为 `"OK"` 时，`data` 内仍可能 `allowed = false`（业务层拒绝，如未勾选 `confirmed`）；前端须检查 `data.allowed`。

- `allowed = true`：授权成功并完成调用；
- `allowed = false`：`reason` 说明拒绝原因；
- `provider`：实际使用的 provider；
- `data`：provider 原始返回数据。

## 6. P1+ 观测与执行工具边界

后续接入云厂商只读观测和执行介体后，AI 工具网关需要明确区分只读观测、任务提议和真实执行。

| tool_code | 建议模式 | 说明 |
| --- | --- | --- |
| `cloud.resources.list` | read_only | 查询资源 |
| `cloud.metrics.query` | read_only | 查询指标 |
| `cloud.logs.search` | read_only | 查询日志 |
| `cloud.traces.query` | read_only | 查询链路 |
| `cloud.topology.get` | read_only | 查询拓扑 |
| `execution.media.list` | read_only | 查询可用执行介体和健康状态 |
| `execution.tasks.propose` | require_confirm | 根据 AI 建议创建待确认 Execution Task |
| `execution.tasks.dispatch` | deny | 不允许 AI 直接分发或执行任务 |
| `execution.command.run` | deny | 不允许 AI 直接运行自由命令 |

约束：

- AI 可以调用只读观测工具获取上下文。
- AI 可以调用 `execution.tasks.propose` 生成待确认任务，但任务必须进入 Execution 状态机。
- AI 不得直接调用执行代理、SSH、K8s exec、云厂商写 API、数据库写操作。
- AI 生成的命令必须匹配 Execution 模块中的 Command Spec，并由运维人员确认后才能执行。
- 执行介体、代理、租约和日志回传契约见 `ops/execution-agent-contract.md`。

## 7. Provider 类型说明

| 类型 | 说明 |
| --- | --- |
| `a` | 通用 HTTP API Key 接入 |
| `b` | 类 OpenAI / Claude 兼容接入 |
| `c` | 内部服务接入 |

## 8. 前端页面交互建议

### 8.1 Provider 管理页

- 左侧为 provider 列表，支持按启用状态筛选；
- 右侧为表单编辑区，可新增、编辑、删除 provider；
- 点击“测试连接”可校验 `base_url`、`api_key`、`headers` 是否可用；
- 提交后立即刷新列表。

### 8.2 工具调用页

- 先选择 provider；
- 再填写工具编码与调用参数；
- 若工具要求人工确认，则前端需先勾选“已确认”再允许提交；
- 返回结果区展示 `allowed`、`reason`、`provider` 和 `data`。

## 8. 错误语义

| code | HTTP | 说明 |
| --- | --- | --- |
| `OK` | 200 | 成功 |
| `INVALID_ARGUMENT` | 400 | provider / invoke 请求参数非法 |
| `UNAUTHENTICATED` | 401 | 未登录或 token 无效 |
| `PERMISSION_DENIED` | 403 | RBAC / 数据范围 / 工具权限校验未通过 |
| `UNAVAILABLE` | 503 | provider registry 或 gateway 未配置 |
| `INTERNAL` | 500 | provider 调用失败或内部错误 |

前端判断成功应使用 `code === "OK"`，勿使用数字 `0`。
