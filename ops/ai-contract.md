# AI 模块对外 API 契约

## 1. 范围

本文档描述 AI 模块当前对外暴露的 provider 管理接口、工具调用接口、告警分析接口，以及 AI 运维助手（会话 + 对话）接口，供前端联调、联调环境配置和后续运维接入使用。

AI 助手只做分析、解释、建议与执行计划，不直接执行任何动作；真实执行必须进入 Execution 状态机并人工确认。

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

`POST /api/ai/analyze-alert` 的内部工具调用使用固定权限码 `app:ai.analysis:analyze`（通过内部 `ToolRequest.RequiredPermission` 传递，与 HTTP 入口一致），不再按 `resource/action` 拼接权限码。若工具网关返回 `allowed=false`，分析仍返回本地兜底摘要，但审计记为 `result=partial` 并在 `tool_error` 记录拒绝原因（`authorization_denied: ...`），避免静默降级。

## 4. Provider 管理接口

### 4.1 获取 provider 列表

- Method: `GET`
- Path: `/api/ai/providers`
- 鉴权: 是

#### 响应示例

列表接口**不返回** `api_key` 字段；通过 `has_api_key` 表示是否已配置密钥。`model` 仅对类型 `b` 有意义，类型 `a` / `c` 返回空字符串。

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
      "model": "",
      "has_api_key": true,
      "timeout_ms": 300000,
      "headers": {
        "X-Client-Name": "aiops"
      },
      "enabled": true,
      "description": "通用 HTTP API Key 工具提供方样例"
    },
    {
      "id": "ark",
      "name": "火山方舟",
      "type": "b",
      "base_url": "https://ark.cn-beijing.volces.com/api/v3",
      "model": "ep-20241128xxxxx",
      "has_api_key": true,
      "timeout_ms": 300000,
      "headers": {},
      "enabled": true,
      "description": "平台直连模型"
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
| `base_url` | string | 是 | provider 基础地址；类型 `b` 填含版本段的模型 API 根地址（如 `https://ark.cn-beijing.volces.com/api/v3`、`https://api.openai.com/v1`），平台只追加 `/chat/completions` |
| `model` | string | 类型 `b` 必填 | 模型名或推理接入点 ID（如 `ep-20241128xxxxx`） |
| `api_key` | string | 新增必填；更新可省略 | 调用凭据；更新时省略则保留原密钥 |
| `timeout_ms` | number | 否 | 非流式请求总时长上限，默认 `300000`；流式对话使用首块 60s + 空闲 180s，总时长受 `ai.turn_budget_s` 约束 |
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
  "timeout_ms": 300000,
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

### 4.4 持久化与密钥约定

- provider 配置落库到 `ai_provider`（迁移 `0048`），数据库是事实源；`ai.providers` YAML 仅在新库/空表时作为种子写入，之后以页面配置为准。
- 页面入口位于「系统设置 → AI 配置」的 Provider 管理页签（`web/src/views/system/ai-config`）；AI 运维助手页只负责对话，不再承载 AI 配置。
- 启动时从数据库解密并加载到运行时注册表；新增/更新/删除经 `POST` / `DELETE` 同步写库并立即生效，无需重启。
- `api_key` 使用平台凭据加密机制（AES-GCM，复用 Integration credential vault）保存为密文；接口只返回 `has_api_key`，从不回显明文。
- 更新时省略 `api_key` 保留原密钥；新增时必填。日志与审计只记录 `has_api_key` / `provider_id` / `type` / `base_url` / `model` 等字段。
- 单条 provider 启动加载失败（解密失败或配置非法）只跳过该条并记录 warn，不阻断 API 启动；坏配置可在页面修正后保存生效。
- 告警分析动态选择 provider：优先启用中的类型 `b`，其次类型 `c`，最后任意启用项；因此页面启停/换模型无需重启。

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

> 状态（2026-11）：cloud.* 系列与 `execution.tasks.propose` 工具化保持推迟（§10.12.2 / §10.12.3）。对话链路当前可用的只读工具为 `alarm.analyze` / `metrics.query`（§10.6.3）/ `alert.rules.list`（规则体检 MVP）/ `accounts.list` / `resource.analyze` / `resources.list`（上下文发现，§10.12.1）。

## 7. Provider 类型说明

| 类型 | 说明 |
| --- | --- |
| `a` | 通用 HTTP API Key 接入：平台按 `POST <base_url>/v1/tools/invoke` 转发工具调用 |
| `b` | 平台直连模型：`base_url` 为含版本段的根地址（OpenAI 填 `https://api.openai.com/v1`，方舟填 `https://ark.cn-beijing.volces.com/api/v3`），平台追加 `/chat/completions`；`model` 必填，`response_format={"type":"json_object"}`，解析 `choices[0].message.content` 并做分析 JSON 校验 |
| `c` | 内部服务接入：兼容旧 `ai-orchestrator` 参考实现，按 `POST <base_url>/api/tool/invoke` 调用 |

类型 `b` 是推荐路径：平台内置固定 system prompt（服务端构建，不接受调用方 payload 覆盖），告警快照和指标证据由 AnalyzeService 组装为 user 消息。模型不可达、429、5xx、空响应或输出无法解析时，平台返回保守降级结果并标记 `degraded=true`，审计记为 `partial`。类型 `c` 仅为可选参考实现，不再是必需组件。

AI 运维助手（§10）只使用类型 `b` 做对话补全：没有启用中的类型 `b` 时对话返回 `status=degraded`、`degraded_reason=model_unavailable`，仍返回已获取的只读上下文，不返回任何执行建议。

## 8. 前端页面交互建议

### 8.1 Provider 管理页（系统设置 → AI 配置）

- 左侧为 provider 列表，支持按启用状态筛选；类型 `b` 的 `model` 直接展示在列表中；
- 右侧为表单编辑区，可新增、编辑、删除 provider；类型 `b` 必须填写模型 / Endpoint；
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

## 9. 告警分析 evidence 语义

`POST /api/ai/analyze-alert` 在 `include_metrics=true` 时，AI 模块会根据告警快照（账号、区域、命名空间、指标、维度、结构化阈值）调用只读指标查询，并把压缩后的证据注入 provider payload 的 `evidence` 字段。collector 未装配时该能力关闭，不影响既有分析链路。

### 9.1 payload.evidence

```json
{
  "status": "ok",
  "reason": "",
  "metrics": [
    {
      "namespace": "SYS.ECS",
      "metric": "cpu_util",
      "unit": "%",
      "threshold": 90,
      "operator": ">",
      "first": 62.1,
      "peak": 97.4,
      "last": 95.2,
      "breach_points": 4,
      "points": [[1762830000, 62.1], [1762830300, 97.4]],
      "labels": {"instance_id": "i-001"},
      "evidence_id": "ev-..."
    }
  ]
}
```

`status` 取值：

| status | 含义 |
| --- | --- |
| `ok` | 取数成功；可能包含 metrics，也可能为空序列 |
| `skipped` | 证据不可取，`reason` 为 `no_account` / `no_region` / `no_namespace` / `no_metric` / `no_dimensions` / `collector_disabled` |
| `permission_denied` | 用户无该账号数据权限，未发起云查询 |
| `unsupported` | 账号认证方式（如 agency）暂不支持指标查询 |
| `error` | 账号解析或指标查询失败，已降级；`reason` 不包含凭据或底层原始错误 |

`points` 为 `[Unix 秒, 值]`；证据总大小超过 32KB 时先丢弃 `points` 仅保留统计摘要并置 `reason=truncated`，仍超限时只保留 `status/reason`。

### 9.2 边界

- 只读：仅调用指标查询，不触发执行、不写业务数据；证据引用和查询审计由 Observability `QueryService` 落库。
- 能力：绑定账号需声明 `metrics` 能力；未声明或 provider 不支持时降级为 `error`，不中断分析。
- 项目：优先使用告警 `labels.project_id` 指定 project；缺失时回落账号 project_id。多项目账号应确保告警来自 CES 拉取（已写入 `project_id`），避免静默查到空序列。
- 窗口：按告警首次触发前 30 分钟到最近触发后 10 分钟推导；最长 6.5 天，超长告警优先保留最近数据，避免超过查询接口 7 天上限。
- 身份：取数继承当前分析用户身份与账号数据范围；权限不足返回 `permission_denied`，不中断分析。
- 凭据：AK/SK 只在 provider 适配层使用，不进入 payload、Prompt、日志或审计明文。
- 事件类告警没有指标维度时自然返回 `skipped`，分析结果仍可返回本地摘要。

### 9.3 对 provider 的要求

provider 应同时消费 `payload.alert` 与 `payload.evidence`，在无证据或证据降级时明确说明不确定性。`references` 建议引用证据 `evidence_id`（如 `observability evidence ev-...`），便于平台回放与审计。

### 9.4 type b 平台直连与 type c 参考协议

类型 `b` 是默认分析路径：平台直接调用模型 API，无需额外编排服务。prompt 由服务端固定构建；`POST /api/ai/analyze-alert` 返回 `summary` / `risk_level` / `recommendations` / `references`，模型异常时降级为保守结果。

类型 `c` 的内部 AI 编排服务仅作为兼容参考实现：平台按现有 `POST /api/ai/tools/invoke` 调用，`tool_code=alarm.analyze`，`payload` 包含 `alert` 与 `evidence`。参考实现位于 `cmd/ai-orchestrator`，监听 `POST /api/tool/invoke`（兼容 `/v1/tools/invoke`），使用 `X-API-Key` 鉴权（平台发送 `Bearer <api_key>`，服务同时兼容裸 key）；模型侧通过 `AIOPS_AI_ORCH_MODEL_BASE_URL`、`AIOPS_AI_ORCH_MODEL_API_KEY`、`AIOPS_AI_ORCH_MODEL_NAME` 配置。平台 provider 注册示例：

```json
{
  "id": "ai-orchestrator",
  "name": "AI Orchestrator",
  "type": "c",
  "base_url": "http://ai-orchestrator.internal:8090",
  "api_key": "<secret>",
  "timeout_ms": 60000,
  "enabled": true
}
```

部署注意：

- `analyze` 总时延 = evidence 预算（默认 5s）+ provider 时延；provider `timeout_ms` 建议不超过 20s，或相应调大平台 `server.write_timeout_s`（默认 30s），避免响应被截断。
- 编排服务应部署在内网，`ModelBaseURL` 由运维通过环境变量配置并限制出站范围；平台侧另有 `ai.outbound_allowed_hosts` / `ValidateOutboundURL` 防护，本参考服务未内置 SSRF 校验。

服务直接返回分析对象（平台 toolgateway 再包装为 `data`）：

```json
{
  "summary": "...",
  "risk_level": "medium",
  "recommendations": ["建议操作...", "建议创建 xx 处置任务..."],
  "references": ["observability evidence ev-..."],
  "uncertainties": ["证据仅覆盖 ... 分钟"]
}
```

服务只做分析与建议，不得返回可直接执行的命令；处置必须由人经 Runbook / Execution 链路确认。服务不持久化告警数据与模型密钥，日志仅记录耗时、token 计数和降级状态。

## 10. AI 运维助手会话与对话

### 10.1 范围与安全边界

AI 运维助手通过自然语言查询状态、分析告警、生成排查步骤与执行计划。服务端固定编排（核心业务流程图 §16）：

1. 接收用户消息 + 页面上下文；
2. 按上下文执行只读工具权限校验并取数（告警快照、指标证据）；
3. 调用平台直连模型（类型 `b`）生成结构化回答；
4. 由服务端补齐可执行动作、确认语义与证据引用；
5. 会话与消息落库，工具调用、对话、反馈与归档/删除全部写审计。

约束：

- AI 不能直接执行、分发或调度任何任务；`execution.tasks.dispatch` / `execution.command.run` 保持不可达。
- 执行类入口只返回「生成执行计划」动作，前端调用既有 `POST /api/executions/tasks`（`source_type=ai_conversation`），任务进入 Execution `pending_confirm`，需人工输入 `CONFIRM` 后才会执行。
- 凭据、`api_key`、AK/SK、Token 不进入 prompt、日志、审计明文或前端持久化状态。
- 工具调用继承当前用户身份，服务端二次校验 RBAC 与工具权限；`allowed=false` 是业务拒绝，前端必须展示 `reason`。
- AI 模块所有 JSON 请求体必须为有效 UTF-8；服务端在解码前校验原始字节，含非法字节（如 GBK 控制台编码污染）直接返回 `INVALID_ARGUMENT`，防止 `encoding/json` 静默替换为 U+FFFD 后损坏入库。

### 10.2 权限码

| 权限码 | 说明 |
| --- | --- |
| `app:ai:chat` | 创建会话、查询会话、发送消息（含流式，§10.17）、归档/恢复/删除会话（§10.13）、创建/查看/撤销会话只读分享（§10.18） |
| `app:ai:chat:feedback` | 提交回答有效/无效反馈 |
| `app:ai:knowledge:manage` | 知识条目管理：新建/编辑/审核/删除/反馈聚合（§10.15，migration `0052`） |
| `app:ai:memory:manage` | 记忆条目管理：检索/手动停用启用（§10.19，migration `0054`） |

迁移 `0049` 同时把以上权限绑定到内置 `admin` 角色。工具权限沿用既有种子：`alarm.analyze`、`metrics.query`（只读），`execution.tasks.propose`（`require_confirm`，仅用于生成执行计划入口的权限判断）。

### 10.3 创建会话

- Method: `POST`
- Path: `/api/ai/chat/sessions`
- 鉴权: 是（`app:ai:chat`）

请求体：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `title` | string | 否 | 会话标题，缺省为「新会话」，首条用户消息会补充标题 |
| `context` | object | 否 | 页面上下文，见 §10.8 |

响应 `data`：

```json
{
  "session_id": "9b0f...",
  "title": "新会话",
  "status": "active",
  "context": {
    "page": "alerts",
    "page_title": "告警详情",
    "alert_id": "alt-1",
    "application_id": "app-1",
    "resource_id": "i-001",
    "resource_type": "ecs",
    "time_range": "30m"
  },
  "created_at": "2026-01-02T03:04:05Z",
  "updated_at": "2026-01-02T03:04:05Z"
}
```

### 10.4 会话列表

- Method: `GET`
- Path: `/api/ai/chat/sessions?limit=20`
- 鉴权: 是（`app:ai:chat`）

`limit` 取值 1–100，默认 20；按 `updated_at` 倒序返回当前用户的会话数组。

### 10.5 会话详情

- Method: `GET`
- Path: `/api/ai/chat/sessions/:session_id`
- 鉴权: 是（`app:ai:chat`）

返回 `{ session, messages[] }`，消息按时间升序；`assistant` 消息带结构化 `answer`，并含 `tool_calls` 与 `references`。会话不属于当前用户时返回 403 `PERMISSION_DENIED`。

### 10.6 发送消息（同步返回回答）

- Method: `POST`
- Path: `/api/ai/chat/sessions/:session_id/messages`
- 鉴权: 是（`app:ai:chat`）

请求体：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `content` | string | 是 | 用户问题，最长 4000 字符 |
| `context` | object | 否 | 覆盖会话上下文（浮层从任意页面唤起时携带） |
| `client_message_id` | string | 否 | 幂等键，最长 64 字符；同一会话内重复提交返回首次结果，见 §10.6.5 |

响应 `data`（节选）：

```json
{
  "session_id": "9b0f...",
  "message_id": "m-assistant-1",
  "status": "ok",
  "answer": {
    "summary": "当前告警可能由实例 CPU 持续升高导致。",
    "evidence": ["告警 HighCPU（级别 p1，状态 firing）", "observability evidence ev-..."],
    "impact": ["影响 payment-service 支付链路"],
    "recommendations": ["建议生成执行计划，由人工在执行确认页确认"],
    "risk_level": "medium",
    "require_confirm": true,
    "actions": [
      { "code": "execution.propose", "label": "生成执行计划", "allowed": true, "target": "/executions" },
      { "code": "view.alert", "label": "查看告警详情", "allowed": true, "target": "/alerts?alert_id=alt-1" },
      { "code": "view.metrics", "label": "查看指标", "allowed": true, "target": "/observability" }
    ],
    "rollback": "缩容回原实例数",
    "uncertainties": ["缺少最近变更记录"],
    "insufficient_context": false
  },
  "tool_calls": [
    { "tool_code": "alarm.analyze", "allowed": true, "mode": "read_only", "status": "ok" },
    { "tool_code": "metrics.query", "allowed": false, "status": "denied", "reason": "tool permission denied" },
    { "tool_code": "execution.tasks.propose", "allowed": true, "mode": "require_confirm", "status": "ok" }
  ],
  "references": [
    { "type": "alert", "id": "alt-1", "label": "HighCPU", "url": "/alerts?alert_id=alt-1" },
    { "type": "observability_evidence", "id": "ev-...", "label": "指标证据 ev-...", "url": "/observability" }
  ],
  "context": { "page": "alerts", "alert_id": "alt-1", "time_range": "30m" },
  "created_at": "2026-01-02T03:04:10Z"
}
```

#### 10.6.1 `status` 语义

| status | 说明 |
| --- | --- |
| `ok` | 模型正常返回并通过结构校验 |
| `degraded` | 模型未配置、调用失败或输出非法；`answer.degraded=true` 且 `answer.degraded_reason` 为 `model_unavailable` / `model_failed` / `model_output_invalid` / `agent_loop_exhausted` / `turn_budget_exceeded` / `stream_interrupted` / `provider_rate_limited`，保留已获取的只读上下文 |
| `insufficient_context` | 模型显式声明上下文不足（`answer.insufficient_context=true`） |
| `incomplete` | 流式生成中断但已保留部分输出/已获取观测；`answer.incomplete=true`、`answer.incomplete_reason` 说明中断原因（如 `stream_idle_timeout`）。不按普通失败处理 |

#### 10.6.2 `answer` 字段

`summary` / `evidence` / `impact` / `recommendations` / `risk_level` / `require_confirm` / `actions` / `rollback` / `uncertainties` / `insufficient_context` / `incomplete` / `incomplete_reason`。

- `risk_level`：`low` / `medium` / `high` / `info`；模型返回 `critical` 时按 `high` 呈现。
- `actions` 由服务端按权限生成，**不采信模型输出**：
  - `execution.propose`：服务端校验 `execution.tasks.propose` 工具权限，`allowed=false` 时带 `reason`；
  - `view.alert` / `view.metrics`：只读跳转入口，`target` 为前端路由；
  - `memory_case` 类型引用只在 §10.14 记忆命中时出现，无 `url`，`id` 为 `case_id`（阶段 1）。
- `require_confirm`：模型声明或 `risk_level` 为 medium/high 时为 `true`。该字段只表达“建议需要人工确认”，不替代 Execution 状态机的确认。
- `uncertainties` 必须保留，前端需展示；降级回答必须给出不确定性说明。

#### 10.6.3 工具调用与审计

- 每条消息在 gather 阶段先执行 `accounts.list`（未配置时跳过），把当前用户可见的集成账号清单注入观测，供模型选择 `account_id`。
- 上下文中的 `alert_id` 会触发告警快照读取：先做 `alarm.analyze` 的 RBAC 与工具权限校验，再由 `AlertReader.GetForAnalysis` 按当前用户的数据范围校验对象（映射见 §10.6.4）；可推导指标查询条件时触发 `metrics.query` 权限校验并采集只读指标证据。
- 无 `alert_id` 但会话带 `resource_id` 时触发 `resource.analyze`：读取资源并推导 `account_id/region/namespace/dimensions`；资源数据范围由 Asset 应用层按账号归属团队/区域收口（拒绝记为 `status=denied`、`reason=data_scope_denied`）。
- 模型可在 Agent Loop 中申请 `metrics.query`（自由参数）、`alert.rules.list`（规则体检）、`accounts.list`、`resource.analyze` 与 `resources.list`（资源发现）；执行类工具仍在白名单之外。
- 每次工具校验/调用都写入审计：`resource_type=ai`、`resource_id=session_id`、`action=tool_invoke`，payload 含 `tool_code/allowed/mode/status/reason/evidence_id/result`，不含 prompt 与凭据。
- 回答写入审计：`action=chat_message`，含 `status/risk_level/require_confirm/tool_codes/tool_denied/reference_count/provider_id/tokens/degraded_reason`。
- 数据范围校验口径：
  - 告警快照的对象级数据范围在 `AlertReader` 收口（`POST /api/ai/analyze-alert` 与对话共用同一入口），拒绝时返回 `PERMISSION_DENIED`，不返回任何告警字段；
  - 对话路径把拒绝降级为一次观测：`alarm.analyze` 的 `allowed=true`、`status=denied`、`reason=data_scope_denied`（工具权限本身已通过，拒绝来自对象数据范围），不产生告警 `references`，也不返回 `view.alert` 跳转入口，模型仍可基于其它上下文（或降级结果）作答；
  - 工具校验固定 `SkipDataScope=true` 且排在对象读取之前：告警/资源的数据范围由各自读取端口判断，而 `metrics.query`、`alert.rules.list`、`accounts.list`、`resources.list`、`execution.tasks.propose` 没有对象上下文（账号级范围由观测取数、规则/资源查询、账号发现端口二次校验），在此强制数据范围会把正常对话误判为“缺少对象上下文”，也会让无权用户靠拒绝原因探测对象是否存在。
  - 分析路径在读取通过后会把同一份对象上下文随 `alarm.analyze` 工具请求下发，由网关复核（纵深防御）。
- 对话历史最多取最近 12 条送入模型；单条历史内容截断，不读取全量会话。

#### 10.6.4 数据范围映射（告警对象）
平台告警没有独立的部门/团队/区域/tag 字段，对象上下文按以下固定映射生成后交给统一授权服务：

| 告警字段 | 授权入参 | 说明 |
| --- | --- | --- |
| `owner_user_id` | `object_owner` | 匹配 `custom` 范围的 `owner_ids` |
| `labels.department` / `dept` / `department_id` | `object_dept` | 平台告警无独立部门字段 |
| `labels.team` / `owner_team` / `team_id` | `object_team` | |
| `labels.region` / `region_id` | `object_region` | CES 拉取源会写入 `region` |
| `labels.env` / `environment` / `business_line` / `tags`，以及归一化后的 `environment`、`business_line` 字段 | `object_tags` | 支持逗号/分号/竖线分隔；去重后最多 16 个；仅这些键参与 tag 匹配 |

判定语义：

- 用户无数据范围或具备 `all` 范围 → 不按对象过滤；
- 用户具备 `department` / `team` / `region` / `tag` / `custom` 范围时，对象对应维度缺失或不匹配 → 403 `PERMISSION_DENIED`（fail-closed，不做“没标注就放行”）；
- 数据范围守卫未装配（读取端口未注入授权适配器）→ 503 `UNAVAILABLE`，不放行；
- `owner_user_id` 只参与对象级校验，不进入 provider payload，也不写入日志正文；
- 拒绝审计：分析路径 `action=analyze_alert`、`result=denied`、`scope_denied=true`、`reason`；对话路径为 `action=tool_invoke` 的同名原因。日志只记录 `user_id` / `alert_id` / `reason`。

#### 10.6.5 消息幂等（`client_message_id`）

客户端可选的幂等键，用于「请求超时后重试」不产生重复消息与重复模型调用。

- 落库：`ai_session_message.client_message_id`（migration `0050`），仅 user 消息写入；`(session_id, client_message_id)` 上有一个 `WHERE client_message_id <> ''` 的部分唯一索引，并发重复提交由数据库兜底。
- 首次提交：正常落用户消息、校验工具、调用模型、落回答。
- 重复提交（已有对应回答）：**原样返回首次结果**——相同的 `message_id`、`answer`、`tool_calls`、`references`、`status`；不新增消息、不再调用模型、不再采集指标证据。审计仍写一条 `action=chat_message`，并带 `replayed=true` 与 `client_message_id`。
- 重复提交（用户消息已落库但回答缺失，如服务端中断）：返回 **409 `FAILED_PRECONDITION`**，message 提示上一次请求仍在处理或已中断；客户端应刷新会话查看结果，不要静默重发。
- 并发重复提交：插入撞唯一索引后回落到上面的分支（有回答则重放，无回答则 409）。
- 未提供 `client_message_id`：行为与之前一致（每次都是新消息），不承诺幂等。
- 边界：本能力只保证「同一会话内同一幂等键最多一条用户消息 + 至多返回一次相同结果」，不是端到端 exactly-once；流式模型调用在未产生任何增量前可内部退避重试（见 §10.17.7），已产生增量后不重试。

#### 10.6.6 Prompt 时间基准与可读引用

- user prompt 顶层固定带 `now`（服务端当前 Unix 秒，Agent Loop 每轮重新取值）；system prompt 要求 `metrics.query` 的 `from/to` 必须以 `now` 为基准，禁止从规则 ID、告警 ID 或其他标识符推断时间。
- 指标观测 `summary` / `data` 带 `metric_label`（常见 CES 指标中文名映射；未映射时为空，模型必须保留原始指标代码并注明“未映射”，禁止臆造含义）与 `point_count`（裁剪前统计的数据点数）。`point_count=0` 时 summary 明确写“无数据点”，模型应先调整时间窗口重查或说明该窗口无数据。`data.metrics` 携带该次查询裁剪后的序列（`first`/`peak`/`last`/`breach_points`/`points`，超 32KB 先丢 `points` 仅保留统计摘要），`data.from`/`to`/`period` 标明查询窗口；模型应基于这些数值收敛作答，不得换参数重查同一窗口。
- 规则观测每条规则带 `metric_label`；回答引用规则时使用可读形式「规则名（指标中文名 操作符 阈值，采样窗口 Xs，级别）」，`rule_id` / `evidence_id` 只作为括号内的证据引用，不得把机器 ID 当作唯一指代。
- 推荐操作必须逐条结构化：目标规则（名称+ID）、调整字段、现值→建议值、理由与风险；涉及多条规则逐条列出，禁止只写“部分规则需调整”式笼统结论。

#### 10.6.7 可选模型列表与切换

- Method: `GET`
- Path: `/api/ai/chat/models`
- 鉴权：是（`app:ai:chat`，普通用户可调）

响应 `data`：`[{ "provider_id": "...", "name": "...", "model": "..." }]`，仅**启用中的类型 b**，按 `provider_id` 升序；不含 `base_url` / `api_key` / `headers` / `timeout_ms` 等运维字段。

切换语义：

- 创建会话时 `context.provider_id` 可选，写入会话上下文；发送消息时 `body.provider_id` 可选。
- 优先级：请求 `provider_id` > 会话 `context.provider_id` > 默认（第一个启用的类型 b）。
- 所选模型不存在/被禁用时**不失败**：回退默认模型，`answer.uncertainties` 首条写入「所选模型 X 不可用，已回退 Y」。
- 审计：`chat_message` 同时记录 `provider_id`（实际使用）、`requested_provider_id`（请求/会话选择）与 `provider_fallback`（是否回退）。
- 前端在会话中可切换模型，选择从下一条消息生效；生成回答期间选择器临时禁用。后端按每次请求的 `provider_id` 选择本轮模型。
- 换模型不改变 RBAC、工具权限与审计口径；工具调用继续继承当前用户身份。

### 10.7 提交回答反馈

- Method: `POST`
- Path: `/api/ai/chat/sessions/:session_id/messages/:message_id/feedback`
- 鉴权: 是（`app:ai:chat:feedback`）

请求体：`{ "feedback": "valid" | "invalid" }`。仅支持 `assistant` 消息，重复提交覆盖为最后一次结果；审计 `action=chat_feedback`。

### 10.8 页面上下文

```json
{
  "page": "alerts",
  "page_title": "告警详情",
  "alert_id": "alt-1",
  "application_id": "app-1",
  "resource_id": "i-001",
  "resource_type": "ecs",
  "execution_id": "task-1",
  "time_range": "30m",
  "provider_id": "ark-primary"
}
```

| 字段 | 长度上限 | 说明 |
| --- | --- | --- |
| `page` / `page_title` | 64 / 128 | 页面标识与标题，仅用于展示 |
| `alert_id` | 64 | 告警详情 / 告警来源的执行任务 |
| `application_id` | 64 | 应用上下文 |
| `resource_id` | 128 | 资源上下文（资产页选中资源、执行任务目标资源、观测查询维度中的实例 ID） |
| `resource_type` | 64 | 资源类型（如 `ecs` / `pod`），缺失时不传 |
| `execution_id` | 64 | 执行任务详情 |
| `time_range` | 32 | 时间范围标签（如 `30m` / `2h`），仅用于提示 |
| `provider_id` | 64 | 当前选择的对话模型（§10.6.7）；会话中可切换，从下一条消息生效 |

- 字段均为可选业务标识；服务端只做长度裁剪与去空白，不信任前端做权限判断。
- 前端按页面写入资源级上下文，其余页面只写页面级上下文；发送消息时随请求携带，会话创建时也会保存一份。写入方：

| 页面 | 写入的上下文 |
| --- | --- |
| 告警中心（打开详情） | `alert_id` + `application_id` + `resource_id` + `resource_type` + `time_range=30m` |
| 资产管理（选中应用 / 高亮资源） | `application_id`；高亮资源时补 `resource_id` + `resource_type`（云资源优先 `cloud_resource_type`） |
| 执行中心（打开任务详情） | `execution_id`；来源为告警时补 `alert_id`，目标为资源时补 `resource_id` + `resource_type` |
| 观测查询（指标查询成功后） | `resource_id`（维度里的 `instance_id` / `cloud_resource_id` 等）+ `resource_type`（按命名空间）+ `time_range`（按时间选择器换算） |
| 其它页面 / AI 助手独立页 | 仅 `page` + `page_title`（进入 AI 助手页时保留来源页面资源，便于连续追问） |

- 上下文只存业务 ID 与标题，禁止写入凭据、Token、告警正文、任务参数、标签明文或自由文本。
- 退出详情或切换页面时清空资源级字段；`execution_id` 与 `resource_id` 一样被视为资源级上下文（有值时不会被路由切换覆盖）。

### 10.9 前端状态映射（页面原型 §23.2）

| UI 状态 | 判定 |
| --- | --- |
| AI 思考中 / 查询工具中 / 生成回答中 | 流式接口按 `stage` / `tool` 事件即时切换；同步接口按请求进行中提示 |
| 用户取消 / 连接中断 | 流提前结束且无 `answer` / `error`；服务端不落回答，记 `chat_canceled` 审计（§10.17.4） |
| AI 回答失败 | `status=degraded` 且 `degraded_reason` 为 `model_unavailable` / `model_failed` / `model_output_invalid` / `provider_rate_limited`，展示原因与可重试入口 |
| AI 未得出结论 | `status=degraded` 且 `degraded_reason` 为 `agent_loop_exhausted` / `turn_budget_exceeded`，橙色提示「取数预算内未得出结论」，按普通回答卡渲染已获取证据并保留重新提问入口 |
| AI 回答生成中断 | `status=incomplete` 或 `answer.incomplete=true`，橙色提示 + 重新提问入口；按普通回答卡渲染已保留内容，不展示为失败 |
| AI 无足够上下文 | `status=insufficient_context` 或 `answer.insufficient_context=true` |
| AI 建议需人工确认 | `answer.require_confirm=true` |
| AI 建议不可执行 | `actions` 中 `execution.propose.allowed=false`，展示 `reason` |

### 10.10 存储与迁移

- 迁移 `0049_ai_session`：`ai_session`（`session_id` 业务唯一列、`user_id`、`title`、`context_json`、`status`）与 `ai_session_message`（`message_id` 业务唯一列、`session_id`、`role`、`content`、`tool_calls_json`、`references_json`、`feedback`）。
- 迁移 `0050_ai_session_message_idempotency`：`ai_session_message.client_message_id`（+ `(session_id, client_message_id)` 部分唯一索引），支撑 §10.6.5 消息幂等；幂等补齐，不改写已发布的 `0049`。
- 迁移 `0053_ai_session_share`：`ai_session_share`（`share_id` 业务唯一、`session_id`、`owner_user_id`、`token_hash` 唯一、`status`、`expires_at`），支撑 §10.18 会话只读分享；只存 token 哈希，无外键，时间戳由 Go 维护。
- `assistant` 消息的 `content` 保存结构化回答 JSON；读取时由服务端解析为 `answer` 字段（含 `degraded` / `degraded_reason`，保证重放与非重放一致的 `status`）。
- 消息落库与会话 `updated_at` / 标题 / 上下文更新在同一事务内完成。
- 会话归属用户：跨用户访问返回 403；不暴露自增 `id`。
- 归档只更新 `status` 与 `updated_at`（保留历史消息）；删除在同一事务内先删消息再删会话，避免孤儿消息。会话表结构不变，因此归档/删除不新增迁移。

### 10.11 已知限制与 P1 待办

- **告警上下文数据范围过滤已完成**：`AlertReader.GetForAnalysis` 按 §10.6.4 校验对象数据范围，`POST /api/ai/analyze-alert` 与 AI 助手共用同一入口；拒绝时分析返回 403 并写 `result=denied` 审计，对话降级为 `reason=data_scope_denied` 观测。后续新增告警读取入口必须复用该入口，不要绕过。
- **会话归档/删除已完成**：见 §10.13，前端入口在会话列表（归档 / 恢复 / 删除，均二次确认）。
- **消息幂等已完成**：见 §10.6.5（`client_message_id` + 部分唯一索引），前端每次提问生成幂等键、重试复用同一值。
- **资源级上下文已完成**：告警、资产、执行、观测四类页面按 §10.8 写入资源级字段；`execution_id` 为 `0050` 之后新增的上下文维度（仅上下文 JSON，无表结构变更）。
- **相似案例记忆（阶段 1）已完成**：见 §10.14（migration `0051_ai_memory_case`）。
- **知识沉淀（阶段 2）已完成**：见 §10.15（migration `0052_ai_knowledge_item`，人工录入 + 反馈聚合生成 draft + 审核激活 + scope 注入 + 低效自动停用），管理页签在「系统设置 → AI 配置 → 知识沉淀」。
- **受限 Agent Loop（阶段 3）已完成**：见 §10.16（≤3 轮只读工具循环、token 预算、每轮 RBAC 复核、`loop_rounds` 审计）；循环仍只做只读取数，执行类工具不可达。
- **流式与取消已完成**：见 §10.17（SSE 事件级推送、取消不落回答并写 `chat_canceled` 审计）；同步接口 §10.6 保留不变。
- **会话分享已完成**：见 §10.18（对内只读链接 + 1–720 小时有效期 + owner 撤销；不提供匿名/对外访问）。
- **记忆手动管理已完成**：见 §10.19（migration `0054`，管理员检索与手动停用/启用，写 `memory_review` 审计）；回答注入范围不因管理接口扩大。
- **工具范围扩展已落地**：当前只读工具为 `alarm.analyze` / `metrics.query`（自由参数）/ `alert.rules.list` / `accounts.list` / `resource.analyze` / `resources.list`（§10.12.1）；日志、变更类与 `cloud.*` 只读工具仍推迟接入。

### 10.12 演进方向与推迟项（2026-11 决策，规则体检与上下文发现已落地）

2026-11 的「规则体检」场景先落地了 `alert.rules.list` 与自由参数 `metrics.query`；「上下文不足」场景随后补齐 `accounts.list` / `resource.analyze` / `resources.list`（migration `0056`）。日志/变更工具、`cloud.*` 系列与 `execution.tasks.propose` 工具化仍保持**推迟**。工具清单、参数 schema、权限、降级语义与审计口径见 §10.12.1。

#### 10.12.1 规则体检与上下文发现只读工具

- **已落地**：`alert.rules.list`、自由参数 `metrics.query`、上下文发现三件套 `accounts.list` / `resource.analyze` / `resources.list`。执行类工具仍不可达。
- **上下文发现解决的问题**：无告警上下文时模型拿不到 `account_id/region/namespace/instance_id`；`accounts.list` 提供账号清单，`resource.analyze` 从资产页/观测页带入资源上下文，`resources.list` 让模型在纯对话场景发现实例。
- **仍推迟什么**：`logs.search`、`changes.list`、`assets.get`（非云资源详情）、`cloud.*` 系列。资产资源只读范围已按集成账号数据范围收口，但变更类对象仍无统一口径。
- **为什么先做规则体检**：告警规则查询 Port 已存在（`AlertRuleQueryPort`），指标查询已有成熟证据链与数据范围校验；用 namespace 过滤即可覆盖数据库类规则的 MVP 场景。

**工具与权限**

| tool_code | 模式 | 权限 | 说明 |
| --- | --- | --- | --- |
| `alert.rules.list` | read_only | AI 工具权限 `alert.rules.list`（migration `0055`，仅 admin） | 按 namespace 查询告警规则及阈值字段 |
| `metrics.query` | read_only | AI 工具权限 `metrics.query`（既有种子） | 自由参数，服务端校验后取指标证据 |
| `alarm.analyze` | read_only | AI 工具权限 `alarm.analyze`（既有种子） | 重新读取当前告警快照，无参数 |
| `accounts.list` | read_only | AI 工具权限 `accounts.list`（migration `0056`，仅 admin） | 列出当前用户可见的启用账号（脱敏摘要，上限 10 条） |
| `resource.analyze` | read_only | AI 工具权限 `resource.analyze`（migration `0056`，仅 admin） | 读取会话选中的资源（业务 ID 或 cloud_resource_id），推导账号/区域/namespace/主维度 |
| `resources.list` | read_only | AI 工具权限 `resources.list`（migration `0056`，仅 admin） | 按账号查询本地云同步资源（source=cloud_sync） |

执行路径：`alarm.analyze` 仍由 provider 执行；`metrics.query` / `alert.rules.list` 由 AI application 经 `MetricEvidencePort` / `AlertRuleListPort` 本地执行；`accounts.list` 经 `AccountListPort`（Integration `AccountService.List` + 账号数据范围），`resource.analyze` / `resources.list` 经 `AssetReader` / `ResourceDiscoveryPort`（Asset 应用层数据范围）本地执行，均不经 `ProviderRegistry` 转发。`toolgateway` 负责工具权限校验与内置工具清单登记（`internal/ai/toolgateway/builtin.go`），工具权限均落在 `iam_ai_tool_permission` + `iam_role_ai_tool_permission`。

**`metrics.query` 参数 schema（`tool_requests[].params`）**

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `account_id` | string | 否 | 缺省依次回退告警标签 `account_id` 与资源上下文；最终仍缺失则 `status=skipped`、`reason=no_account` |
| `region` | string | 否 | 缺省依次回退告警 `region` 与资源 `region`；≤128 字符 |
| `project_id` | string | 否 | 缺省取告警 `project_id`；≤128 字符 |
| `namespace` | string | 否 | 缺省依次回退告警 `ces_namespace`、资源 `labels.namespace` 与云类型映射；匹配 `^[A-Za-z0-9_.:-]{1,128}$` |
| `metric` | string | 否 | 缺省取告警 `metric_name`；资源上下文不推导指标名（模型必须指定）；≤128 字符 |
| `dimensions` | object<string,string> | 否 | 提供时**替换**推导维度；缺省先取告警标签维度，再取资源主维度（如 `rds_cluster_id`）；≤20 对，key≤64、value≤128；最终不能为空 |
| `from` / `to` | int64 Unix 秒 | 否 | 必须同时提供或同时省略；缺省取告警证据窗口；无告警时取最近 1 小时（告警与资源上下文同优先级窗口均由告警优先）；`from < to` |
| `period` | int 秒 | 否 | 默认 300，最小 10 |
| `aggregator` | string | 否 | 缺省沿用告警注解 `ces_filter` |

- 窗口：普通查询最大 **7 天**；`period >= 86400` 的日聚合允许最大 **30 天**；任何情况下点数（`window/period + 1`）不得超过 **1440**。服务端在 AI 层先校验，观测 QueryService 再校验一次（纵深防御）。
- 阈值语义：`threshold` / `operator` 只从告警结构化条件继承；当模型覆盖 `metric` 且与告警 `metric_name` 不同时，证据不再携带告警阈值，避免破限点误判。
- 数据范围：取数前按 `account_id` 解析账号并过 `AccountAccessAuthorizer`；拒绝时工具权限本身已通过，观测记为 `allowed=true`、`status=permission_denied`、`reason=no access to integration account for evidence`，审计 `result=partial`。
- 证据：观测 `summary` 含指标中文名与数据点数（`无数据点` / `N 点`）；`data` 含 `metric`、`metric_label`、`metric_count`、`point_count`、`evidence_ids`、`evidence_id`、`from`/`to`/`period`（查询窗口）与 `metrics`（裁剪后序列，含 `first`/`peak`/`last`/`breach_points`/`points`，超 32KB 丢 `points` 保留统计摘要）；最终回答 `references` 含 `observability_evidence`。

**`alert.rules.list` 参数 schema（`tool_requests[].params`）**

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `account_id` | string | 否 | 缺省依次回退告警标签与资源上下文；最终缺失则 `status=skipped`、`reason=no_account` |
| `region` | string | 否 | 缺省依次回退告警 `region` 与资源 `region`；≤128 字符 |
| `namespace` | string | **是**（可回退告警 `ces_namespace` 或资源 namespace） | 匹配 `^[A-Za-z0-9_.:-]{1,128}$`；数据库类规则示例 `SYS.RDS` |
| `keyword` | string | 否 | ≤100 字符，按规则名/描述过滤（provider 支持时） |
| `limit` | int | 否 | 默认 **50**，最大 **50**；超过上限返回 `status=invalid_argument` |

- 返回规则字段：`rule_id`、`name`、`namespace`、`severity`、`enabled`、`description`（≤300 runes）、`metric`、`metric_label`（常见 CES 指标中文名，未映射为空）、`operator`、`threshold`、`window_seconds`、`labels`（≤20 对）。
- provider 支持矩阵（2026-11）：
  - huawei `ak_sk`：✅ 真实 CES V2 `ListAlarmRules`（单页 100、最多 10 页）；`region` 缺省取账号第一个区域，`project_id` 按 `extra_config.region_projects` / 账号配置解析；`keyword` 由服务端本地过滤（先取上限 500 条再截断到 `limit`）。
  - huawei `auth_type=none`：✅ fake provider（联调/CI）。
  - huawei `agency`：❌ 返回 `status=unsupported` 与稳定 reason，不视为内部错误。
- 数据范围：`region` 不在账号区域列表中返回 `status=error`（invalid_argument），由 AI 层映射为 `status=error` 而非 unsupported。
- 证据：观测 `data` 含 `namespace`、`rule_count`、`evidence_id`，每条规则含 `metric_label`；最终回答 `references` 含 `observability_evidence`。

**`accounts.list` 参数与观测**

- 无参数；gather 阶段每条消息自动执行一次，Agent Loop 也可再次申请（会话中途新增账号时刷新清单）。
- 只返回启用账号，且逐个过 `AccountAccessAuthorizer`（与 `metrics.query` 同口径）；不可见账号不返回。
- 观测 `data={accounts:[{account_id,name,provider,regions}],total}`；`total` 为真实账号数，`accounts` 最多 10 条。
- 脱敏边界：不返回 `auth_type`、`extra_config`、`project_id` 或任何凭据。

**`resource.analyze` 参数与观测**

- 无参数；读取会话上下文 `resource_id`（资产页写业务 ID，观测页写云维度值 `instance_id/cloud_resource_id`）。
- 读取顺序：资产业务 ID → 未命中再按 `cloud_resource_id` 查询云同步资源；命中多条时按数据范围取第一条（其余仅记 warn）。
- 观测 `data` 含 `resource_id` / `name` / `cloud_resource_type` / `cloud_resource_id` / `account_id` / `region` / `namespace` / `dimensions`。
- 数据范围由 Asset 应用层按账号 `owner_team` + region 校验；拒绝记 `status=denied`、`reason=data_scope_denied`；资源不存在记 `reason=resource_not_found`；手工资源无账号归属，无法参与云参数推导。

**`resources.list` 参数 schema（`tool_requests[].params`）**

| 字段 | 类型 | 必填 | 约束 |
| --- | --- | --- | --- |
| `account_id` | string | **是**（可回退资源上下文） | ≤128 字符；最终缺失则 `status=skipped`、`reason=no_account` |
| `resource_type` | string | 否 | 小写云类型（rds/ecs/dcs…），匹配 `^[a-z0-9_-]{1,64}$` |
| `region` | string | 否 | ≤128 字符 |
| `keyword` | string | 否 | ≤100 字符，按名称/cloud_resource_id/类型/区域过滤 |
| `limit` | int | 否 | 默认 20，最大 50；超过返回 `status=invalid_argument` |

- 数据源为本地 `asset_resource`（`source=cloud_sync`），不实时调用云 API；账号尚未同步时返回 `status=ok` 且列表为空。
- 观测 `data={account_id,resource_type,region,resource_count,resources:[...]}`；每条资源含 `resource_id` / `name` / `cloud_resource_type` / `cloud_resource_id` / `region` / `account_id` / `namespace` / `dimension_name` / `dimension_value`，不回传原始 labels。
- 数据范围：账号解析 + `AccountAccessAuthorizer` + Asset `owner_team` 过滤三重收口；拒绝时 `status=permission_denied`、`reason=no access to integration account for evidence`。

**通用语义**

- 参数非法：`allowed=true`、`status=invalid_argument`、`reason` 说明字段约束，不执行取数；模型可在后续轮次修正。
- 上下文缺失：`allowed=true`、`status=skipped`；常见 reason：`no_alert` / `no_account` / `no_region` / `no_namespace` / `no_metric` / `no_dimensions`。
- 资源上下文缺失/不可用：`resource_not_found` / `resource_not_cloud_linked` / `resource_load_failed` / `data_scope_denied`。
- 审计：每次执行写 `tool_invoke`，payload 含 `tool_code/allowed/mode/status/reason/evidence_id/result`；`evidence_id` 仅在成功取数时出现。`chat_message` 仍含 `tool_codes` / `tool_denied` / `loop_rounds`。
- Agent Loop 白名单扩展为 `alarm.analyze` / `metrics.query` / `alert.rules.list` / `accounts.list` / `resource.analyze` / `resources.list`；其余约束（≤3 轮、20000 token、单轮≤3 个申请、轮内/跨轮按 tool_code+params 去重、每轮 RBAC、执行类拒绝）不变（§10.16）。

#### 10.12.2 云厂商只读观测工具（cloud.* 系列）— 推迟

- **推迟了什么**：§6 表格中的 `cloud.resources.list` / `cloud.metrics.query` / `cloud.logs.search` / `cloud.traces.query` / `cloud.topology.get` 保持规划态，不注册进 toolgateway。
- **为什么推迟**：traces/topology 仍为 fake，且 cloud.* 工具需要 Integration 账号维度的数据范围联动（与告警数据范围工作同根）；指标场景已由专用 `metrics.query` 证据链覆盖，重复建第二套并行工具没有增量价值。
- **什么时候推进**：真实云厂商 logs/traces provider 落地之后，且 Integration 账号纳入数据范围模型时，把 cloud.* 定义为 `metrics.query` 的泛化（或别名路由），不另起炉灶。

#### 10.12.3 `execution.tasks.propose` 工具化 — 推迟

- **推迟了什么**：不把 `execution.tasks.propose` 注册为 toolgateway 工具；AI 生成执行计划的唯一出口仍是 answer `actions` 中的 `execution.propose` 入口（前端调 `POST /api/executions/tasks`，`source_type=ai_conversation`，任务落 `pending_confirm`）。
- **为什么推迟**：现有 actions 出口已完整满足安全闭环（人工确认 + 审计 + Execution 状态机），工具化不解决任何现存缺口；反而引入新的边界问题——模型可填字段的白名单、require_confirm 与 §10.16「循环内只读」约束的兼容性（当前裁决：循环内不可达，循环后由服务端触发）。
- **什么时候推进**：仅当出现「模型必须在无人交互场景生成待确认任务」的真实需求（如巡检 Worker 建议 → 待确认任务）时重启定义；届时必须先解决字段白名单、幂等键与 Loop 边界三个问题。

#### 10.12.4 其他未实现项

- **会话分享匿名/对外公开链接**：未实现；若后续需要必须另行评审，不得绕过登录与 `app:ai:chat`。
- **pgvector 向量检索**：记忆/知识检索当前为 SQL 关键词策略；向量增强仅在数据规模或召回质量证明必要时评估（路线图 §7）。

### 10.13 会话归档 / 恢复 / 删除

会话生命周期：`active` ⇄ `archived`，任一状态均可删除。归档后会话只读（可查看历史、不可发消息），用于收口历史而不丢证据。

#### 10.13.1 归档会话

- Method: `POST`
- Path: `/api/ai/chat/sessions/:session_id/archive`
- 鉴权: 是（`app:ai:chat`）

响应 `data`：更新后的会话对象（`status=archived`）。

- 已归档时幂等：返回当前视图，不报错、不推进 `updated_at`。
- 归档只改状态，历史消息与工具调用记录保持原样。
- 审计：`action=chat_session_status`，payload 含 `session_id/status/previous_status/result`。

#### 10.13.2 恢复会话

- Method: `POST`
- Path: `/api/ai/chat/sessions/:session_id/unarchive`
- 鉴权: 是（`app:ai:chat`）

响应 `data`：更新后的会话对象（`status=active`）。未归档时幂等返回当前视图；恢复后可继续发送消息。审计同 §10.13.1。

#### 10.13.3 删除会话

- Method: `DELETE`
- Path: `/api/ai/chat/sessions/:session_id`
- 鉴权: 是（`app:ai:chat`）

响应 `data`：`{ "deleted": true }`。

- 删除会话及其全部消息，不可恢复；重复删除返回 404 `NOT_FOUND`。
- 审计：`action=chat_session_delete`，payload 含 `session_id/status/result`。
- 删除的是当前会话时，前端自动切到列表中的下一个会话；无剩余会话则回到空态。

#### 10.13.4 权限与错误

- 三个接口复用 `app:ai:chat`：会话是用户私有资源，真正的边界是 application 层的归属校验（跨用户 403），不引入新权限码，避免新增迁移与记忆路线图的版本编号冲突。
- 会话不存在或已删除 → 404 `NOT_FOUND`；不属于当前用户 → 403 `PERMISSION_DENIED`；归档会话发消息 → 409 `FAILED_PRECONDITION`（既有语义）。
- 前端在归档会话上发消息时提前拦截并提示先恢复，避免用户输入丢失。

### 10.14 相似案例记忆（阶段 1，RAG-lite）

从「有效反馈」沉淀历史案例，并在后续对话/告警分析中检索注入。设计背景见 `docs/ai-assistant-memory-roadmap.md` §4。

#### 10.14.1 沉淀规则

- 触发：`POST .../feedback` 提交 `valid`。
- 条件：对应 assistant 消息的结构化回答可解析、`degraded=false`、`summary` 非空；降级回答不入记忆。
- 幂等：迁移 `0051` 的部分唯一索引 `(user_id, source_message_id)` 保证同一条消息只沉淀一条案例；重复反馈不重复写入。
- 字段：归属 `user_id`、`session_id`、`source_message_id`、`application_id`/`resource_id`/`resource_type`、`alert_signature`、`keywords`、`summary`（≤500 runes）、`recommendations`（≤3 条）、`risk_level`、`quality_score`（有效初始 +1）。
- 无效反馈：对已有案例 `quality_score -2`；降到 `≤ -2` 时置 `retired`，之后不再被检索（阶段 1 不复活）。案例尚未沉淀时无效反馈是 no-op。
- 失败降级：沉淀/降分失败只记 Warn，不影响反馈接口响应；不写案例原文到日志。

#### 10.14.2 检索与注入

- 触发：`POST .../messages` 在调用模型前检索；`POST /api/ai/analyze-alert` 在组装 payload 时检索。
- 匹配优先级（`match_rank`）：`alert_signature` 精确命中 3 → `application_id` 或 `resource_id` 命中 2 → 关键词全文命中 1；同分按 `quality_score`、`updated_at` 倒序。
- `alert_signature` = `alert_name + metric + 维度哈希`（名称/指标小写，维度按 key 排序后取 sha1 前 8 字节），同类告警可稳定命中。
- 关键词由摘要、建议、告警名、资源类型切词去重生成（PostgreSQL `simple` 分词，`to_tsvector(...) @@ plainto_tsquery(...)`）。
- 上限：单次最多注入 `3` 条；每条摘要 ≤500 runes；建议 ≤3 条。
- 跨用户隔离：检索条件强制 `user_id = 当前用户` 且 `status = active`，A 用户案例不会出现在 B 用户回答中。

#### 10.14.3 回答侧表现

- 对话 prompt 追加 `memory_cases`：`[{case_id, summary, recommendations, risk_level}]`；system prompt 增加约束「记忆仅作参考证据、引用需标注 `case_id`、不得照搬结论」。
- `answer.references` 新增 `{ "type": "memory_case", "id": "<case_id>", "label": "历史案例：<摘要摘要>" }`（无 `url`）。
- 分析接口 provider payload 新增 `similar_cases`（与 `evidence` 平级），`include_metrics` 与否无关。
- 审计：`chat_message` 与 `analyze_alert` 的 payload 新增 `memory_used`（注入条数）与 `memory_status`；
  - `ok`：命中并注入；`empty`：无匹配；`error`：检索失败已降级；`skipped`：记忆能力未装配。
  - 审计不含案例摘要或任何原文。

#### 10.14.4 降级与边界

| 场景 | 行为 |
| --- | --- |
| 记忆表为空 / 无匹配 | 正常回答，`references` 不含 `memory_case`，`memory_status=empty` |
| 检索失败 | 跳过注入，回答主链路不变，`memory_status=error` |
| 记忆能力未装配 | 行为与接入前一致，`memory_status=skipped` |
| 告警指纹读取失败（越权/已删除） | 案例仍可沉淀，只是不带 `alert_signature` |
| 记忆条目 retired | 不参与检索 |

- 存储：`ai_memory_case`（migration `0051`），业务 ID 为 `case_id`，不暴露自增 `id`；时间戳由 Go 维护，无外键。管理员可通过 §10.19 手动检索与停用/启用条目。
- 记忆只影响证据与建议质量，不改变权限判断，也不会产生执行动作。

### 10.15 知识沉淀（阶段 2）

把「反馈有效」的经验沉淀为经人工审核的知识条目，生效后按 scope 注入回答的 system prompt 附加段。设计背景见 `docs/ai-assistant-memory-roadmap.md` §5。

#### 10.15.1 条目模型

| 字段 | 说明 |
| --- | --- |
| `item_id` | 业务 ID，对外使用（references 与审计引用它） |
| `scope` | `global` / `application` / `resource_type` |
| `scope_ref` | 范围值；`global` 必须为空，其它范围必填 |
| `title` / `content` | 标题（≤120 runes）/ 内容（≤2000 runes）；不含凭据与告警正文 |
| `origin` | `manual`（人工录入）/ `feedback_aggregate`（反馈聚合生成） |
| `status` | `draft` / `active` / `disabled`；**只有 active 会注入** |
| `hit_count` | 注入次数 |
| `useful_count` | 注入后获得 `valid` 反馈的次数 |
| `reviewed_by` / `reviewed_at` | 最近审核人与时间 |

存储：`ai_knowledge_item`（migration `0052`），业务 ID 唯一、时间戳由 Go 维护、无外键。

#### 10.15.2 管理接口（鉴权 `app:ai:knowledge:manage`，仅 admin 种子绑定）

| Method | Path | 说明 |
| --- | --- | --- |
| `GET` | `/api/ai/knowledge/items?status=&scope=&limit=&offset=` | 列表，返回 `{ items: [...] }`；`limit` 1–100，默认 20 |
| `POST` | `/api/ai/knowledge/items` | 新建（总是 `draft`，`origin=manual`） |
| `PUT` | `/api/ai/knowledge/items/:item_id` | 编辑内容/范围；**编辑 active 条目会回到 draft 并清空审核人** |
| `POST` | `/api/ai/knowledge/items/:item_id/review` | `{ "action": "activate" \| "disable" }` |
| `DELETE` | `/api/ai/knowledge/items/:item_id` | 删除；重复删除 404 |
| `POST` | `/api/ai/knowledge/aggregate` | 反馈聚合：`{ application_id, min_cases? }` → 生成/更新 draft，不覆盖 active |

- 审计：新建/编辑 `action=knowledge_edit`；审核/停用/删除/自动停用 `action=knowledge_review`（payload 含 `action` 与 `status`）。
- 校验失败返回 400，条目不存在返回 404；`app:ai:knowledge` 权限缺失返回 403。
- 未注入授权适配器（无 RBAC）的环境**不注册**知识管理路由，避免审核动作失去鉴权。

#### 10.15.3 注入规则

- 取数：`status=active` 且 `scope` 命中当前会话上下文（`global` 恒命中；`application` 比对 `context.application_id`；`resource_type` 比对 `context.resource_type`，上下文缺失时回落到告警快照字段）。
- 排序与上限：按 `hit_count` 倒序、`updated_at` 倒序；最多 **5** 条，标题+内容累计 ≤ **3000 runes**（超长条目跳过，不截断内容）。
- 注入形态：追加到 system prompt 末尾的「知识沉淀」段落，**固定安全约束段落保持在前**；同时写入 `answer.references`：`{ "type": "knowledge_item", "id": "<item_id>", "label": "<title>" }`（无 `url`）。
- 命中统计：回答落库后 `hit_count + 1`；用户对含该条目的回答提交 `valid` → `useful_count + 1`。
- 自动停用：`hit_count ≥ 5` 且 `useful_count == 0` 的 active 条目自动转 `disabled`，写 `knowledge_review` 审计（`action=auto_disable`），立即停止注入。
- 审计：`chat_message` payload 新增 `knowledge_used`（条数）与 `knowledge_status`（`ok/empty/error/skipped`），不含条目正文。

#### 10.15.4 降级与边界

| 场景 | 行为 |
| --- | --- |
| 无生效条目 / 范围不匹配 | 不注入，`knowledge_status=empty` |
| 取数失败 | 跳过注入，回答主链路不变，`knowledge_status=error` |
| 未装配知识能力 | 行为与接入前一致，`knowledge_status=skipped` |
| draft / disabled 条目 | 永不注入，也不产生 references |
| 条目内容含敏感信息 | 由录入/审核人负责；服务端只做长度与空白裁剪，注入前不做二次改写 |

- 知识只影响建议质量，不改变权限判断、`actions` 生成或执行链路。
- 反馈聚合（`POST /api/ai/knowledge/aggregate`）：
  - 来源是该用户在某应用下 `status=active` 的记忆案例（§10.14），`min_cases` 默认 3、上限 20；案例不足不生成草稿（`action=insufficient_cases`）。
  - 生成条目 `scope=application`、`scope_ref=application_id`、`origin=feedback_aggregate`、`status=draft`、`item_id=agg-<application_id>`（固定，保证同应用重复聚合同一条目）。
  - 幂等与保护：草稿/停用条目被**更新**（`action=updated`）；已 `active` 的条目**不被机器覆盖**（`action=skipped_active`），需人工先停用再聚合。
  - 聚合只读记忆案例、只写草稿条目，不改变回答链路；管理页签见「系统设置 → AI 配置 → 知识沉淀」。

### 10.16 受限 Agent Loop（阶段 3）

模型在单次回答内可申请追加**只读**工具取数，服务端在预算内多轮执行后收敛。设计背景见 `docs/ai-assistant-memory-roadmap.md` §6。

#### 10.16.1 编排形态

```text
第 0 轮：服务端常规取数（告警快照 + 指标证据，与 §10.3 一致）
  -> 模型输出结构化 answer，或输出 tool_requests 申请追加只读数据
  -> 服务端逐个校验 RBAC + 工具权限并执行，结果作为 observations 追加
  -> 下一轮模型调用（携带全部 observations）
  -> 轮数/token 预算耗尽仍无 answer：返回 degraded(agent_loop_exhausted)
```

#### 10.16.2 硬约束

| 控制点 | 要求 |
| --- | --- |
| 工具白名单 | 仅 `alarm.analyze`、`metrics.query`、`alert.rules.list`、`accounts.list`、`resource.analyze`、`resources.list` 可被申请；其它工具（含 `execution.tasks.propose` / `dispatch` / `command.run`）一律拒绝并记 `allowed=false`、`reason=tool is not allowed in agent loop` |
| 参数透传 | `tool_requests[].params` 按 §10.12.1 校验；非法参数记 `status=invalid_argument` 并反馈给模型，不执行取数 |
| 权限 | 每轮每个工具独立过 RBAC + 工具权限（与首轮同一套校验），`allowed=false` 时把原因反馈给模型而不中断 |
| 预算 | 追加轮数 ≤ **3**；累计 token ≥ **20000** 即终止；单轮最多受理 **3** 个申请（轮内相同 `tool_code+params` 去重，同工具不同参数各自执行）；单次对话总时间预算 `ai.turn_budget_s`（默认 600s），超时立即终止并返回 `turn_budget_exceeded` |
| 去重 | 轮内相同 `tool_code + params` 只执行一次，不同参数各自执行；跨轮按 `tool_code + params` 去重：已成功执行过的组合重复申请时返回 `status=denied`、`reason=duplicate tool request: identical parameters already executed in this turn`，不再取数且不占用有效轮数；连续两轮无实际取数仍按收敛规则终止 |
| 收敛 | 连续两轮「申请了但一个工具都没真正执行」才终止，给模型一次看到拒绝原因后自我纠正的机会 |
| 审计 | 每轮工具调用写 `tool_invoke`（与现状同结构）；`chat_message` 新增 `loop_rounds`（实际追加轮数） |
| 契约 | 接口仍同步返回；`tool_calls` 数组容纳多轮调用记录；流式与取消见 §10.17 |
| 执行边界 | 循环内不存在任何执行类工具调用；`actions` 仍只由服务端按权限生成 |

#### 10.16.3 降级语义

| 场景 | 行为 |
| --- | --- |
| 轮数或 token 预算耗尽 | `status=degraded`、`degraded_reason=agent_loop_exhausted`，保留已获取的 observations 作为证据 |
| 回合总时间预算耗尽 | `status=degraded`、`degraded_reason=turn_budget_exceeded`，停止后续模型/工具调用并保留已获取的 observations |
| 流式客户端断开 | 模型调用随 ctx 取消而中止；不落 assistant 消息，记 `chat_canceled` |
| 连续两轮无实际取数 | 同上（`agent_loop_exhausted`） |
| 模型输出既非 answer 也非 tool_requests | `degraded_reason=model_output_invalid`（与 §10.6.1 一致） |
| 工具被拒绝 | 回答仍正常返回；拒绝原因在 `tool_calls[].reason` 与审计中可见 |
| 未装配知识/记忆 | 循环本身不受影响 |

- 提示词约束：system prompt 明确「只在现有只读观测不足时申请；可申请 `alarm.analyze` / `metrics.query` / `alert.rules.list` / `accounts.list` / `resource.analyze` / `resources.list`；`account_id` 优先来自 `accounts.list` 清单（不可用时回退告警/资源上下文），不得臆造；参数边界见 §10.12.1；最多 3 轮且单轮最多 3 个申请，同轮可并列申请多个不同参数的 metrics.query；现有观测足以作答时必须立即输出 answer；同工具同参数重复申请无意义（服务端跨轮去重拒绝）；不得申请执行类工具」。
- 前端按 §10.9 映射 `status`；多轮调用会自然出现在 `tool_calls`；`loop_rounds` 仅用于审计与排查。
- 循环只影响取数与证据丰富度，不改变权限判断、`actions` 生成或执行链路。

### 10.17 流式对话（SSE）与取消语义

同步接口 §10.6 保留不变；另提供流式接口。回答是结构化 JSON（summary/actions/references），最终答案仍整体下发；模型调用期间额外推送 `delta` 增量（思考链/回答片段）用于进度反馈，阶段、工具观测、最终回答与错误按**事件级**推送。

#### 10.17.1 接口

- Method: `POST`
- Path: `/api/ai/chat/sessions/:session_id/messages/stream`
- 鉴权：是（`app:ai:chat`），会话归属与归档校验与同步接口一致
- 请求体：与 §10.6 完全一致（`content` / `context` / `client_message_id`）
- 响应头：
  - `Content-Type: text/event-stream; charset=utf-8`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
  - `X-Accel-Buffering: no`（禁用反向代理缓冲）
- 事件格式：每条 `data: <json>\n\n`，写入后立即 `Flush()`。

#### 10.17.2 事件类型

| `type` | 字段 | 说明 |
| --- | --- | --- |
| `stage` | `stage`: `gathering` / `thinking` / `answering` | 编排阶段：取数与权限校验 → 模型调用 → 组装回答 |
| `tool` | `tool`: `{tool_code, allowed, status, reason?, summary?}` | 一次工具校验/执行摘要，与 `tool_calls` 同源；不含 prompt、凭据与告警正文 |
| `delta` | `phase`: `thinking` / `answer`；`text`: string | 模型增量：`thinking` 来自 `reasoning_content`，`answer` 来自 `content`。`answer` 增量只用于“已生成 N 字”进度，不改变最终 JSON 解析 |
| `answer` | `result`: 与 §10.6 响应 `data` 同构 | 最终结构化回答；收到即表示本轮结束 |
| `error` | `code` / `message` | 业务错误；此时 HTTP 状态已是 200 |

正常顺序：`gathering` → `tool`* → `thinking` → `delta(thinking)`* → `delta(answer)`* → `tool`*（Agent Loop 追加轮）→ `answering` → `answer`。任一事件都可能因取消或错误提前结束。

- 心跳：服务端每 **15s** 写入一条 `: ping\n\n` SSE 注释（不属于 `data` 事件），防止代理/网关把长时间思考误判为空闲连接；客户端解析器必须忽略 `:` 注释行。
- 模型增量不落库：`delta` 只存在于本次流；最终回答仍以完整 JSON 的 `answer` 事件为准。

#### 10.17.3 状态码与错误

- 请求体非法（缺 `content`、JSON 解析失败）：在写 SSE 头之前返回 400，不产生任何事件。
- 401（未认证）与 403（RBAC/数据权限）发生在中间件或鉴权阶段，仍返回普通 JSON 错误，不进入 SSE。
- 一旦开始写 SSE（HTTP 200），后续业务错误（会话不存在、归档、幂等冲突、内部错误）统一转 `error` 事件，`code` 为业务错误码（如 `FAILED_PRECONDITION` / `NOT_FOUND` / `INTERNAL`）。
- 客户端应把「流结束但既无 `answer` 也无 `error`」视为连接中断。

#### 10.17.4 取消语义

- 触发：客户端断开（HTTP ctx 取消）或事件写入失败。
- 服务端在四个检查点响应取消：`before_gather`（取数前）、`before_model`（模型调用前）、`before_persist`（模型返回后、落库前）、`before_answer`（推送 `answering` 前）。
- 取消后**不写 assistant 消息**；已落库的用户消息保留，不回滚、不删除。
- 取消写审计：`action=chat_canceled`，payload 含 `session_id` / `user_message_id` / `phase` / `result=canceled`。
- 取消不是业务失败：不生成 `degraded` 回答，也不触发模型降级重试。
- 客户端取消后不要用同一 `client_message_id` 立即重试并期待重放：用户消息已存在但无回答，服务端按 §10.6.5 返回 409。

#### 10.17.5 幂等与重放

- 与同步接口共享同一 `client_message_id` 幂等键。
- 命中已有回答：先推 `gathering`，再直接推一条 `answer` 事件（内容与同步接口首次结果一致），不再调用模型、不再落消息；审计 `chat_message` 带 `replayed=true`。
- 命中「用户消息已落库、回答缺失」：推 `error` 事件（`FAILED_PRECONDITION`），与同步接口的 409 语义一致。

#### 10.17.6 前端接入约定

- 使用 `fetch` + `ReadableStream` 读取，不用 `EventSource`（需要 POST 与 Bearer 头）；实现见 `web/src/api/sse.ts` 与 `web/src/api/ai.ts` 的 `streamChatMessage`。
- `stage` 事件驱动阶段提示，`tool` 事件用于即时展示工具校验结果，`delta` 事件只做“已生成 N 字”进度（不渲染半成品 JSON），`answer.result` 走与同步接口相同的渲染与 `actions` 权限判断；`:` 注释心跳必须忽略，未知事件类型优雅忽略。
- 发送中允许「停止生成」：`AbortController.abort()` 中断读取；服务端按 §10.17.4 不落回答。停止后应回读会话，避免本地乐观消息与服务端不一致。
- `status=incomplete` 时展示橙色“生成中断”提示与「重新提问」入口，按普通回答卡渲染已保留的 `summary/uncertainties`，不得展示为红色失败卡。

#### 10.17.7 模型超时、重试与部分保留

- **超时语义**：流式调用不再用单个总超时罩住整个 SSE。首块超时 **60s**、空闲超时 **180s**（仅在连续无任何 `data` 增量时计时），总时长只受 `ai.turn_budget_s`（默认 600s）约束，并与 nginx/server 660s 对齐；provider `timeout_ms` 仅约束非流式请求总时长（默认 300000ms）。
- **重试条件**（仅在尚未向客户端输出任何 `delta` 时）：429（解析 `Retry-After`）/5xx/网络错误/首块超时/空响应；指数退避 1s、2s 加抖动，单次回合最多 **2 次**重试，且必须落在回合预算内。非流式调用采用同一错误分类与固定退避预算；`Retry-After`/配额提示只用于用户文案，不改变其退避间隔。
- **不重试**：3xx/400/401/403 等配置与鉴权错误；客户端断开（`ABORTED`）；已产生任何 `thinking`/`answer` 增量后的中断（避免前端文本重复）。
- **429 配额提示**：退避优先使用 `Retry-After`；缺失时从响应体解析 `resets in 5h` / `quotaResetDelay: 1h` / `try again in 30m` 等文本，只保留时长、不记录响应体。重试耗尽后返回 `degraded_reason=provider_rate_limited`，答案 `uncertainties` 直接给出“预计约 X 后恢复”，前端失败提示同步展示该文案。
- **部分保留**：已输出增量后中断时，返回 `Partial=true` 的部分结果；application 层优先解析部分 JSON，失败则返回带已获取观测的保守回答，`status=incomplete`、`answer.incomplete=true`、`answer.incomplete_reason=stream_idle_timeout|stream_read_error|empty_model_response`。不完整回答仍会落库，重放时保留 `incomplete` 标记。
- 审计：`chat_message.status=incomplete`、`degraded_reason=stream_interrupted`（保守回答场景），`result=partial`。

### 10.18 会话只读分享

会话默认按 `user_id` 隔离（§10.10）。分享提供**对内只读**链接：仅平台内已登录且具备 `app:ai:chat` 的用户可打开；**不提供匿名/对外公开访问**。

#### 10.18.1 设计约束

- token：32 字节随机（base64url），服务端只存 SHA-256；明文只在创建响应返回一次，之后无法再取回。
- 有效期：允许 1–720 小时；缺省 168（7 天）；过期后访问按不可用处理。
- 单会话最多 **10** 条同时有效（未撤销、未过期）的链接，超出返回 409。
- owner 可随时撤销；撤销幂等；会话删除后其分享链接一并失效。
- 分享视图**只读**：不提供发消息、反馈、归档/删除、生成执行计划；共享页会禁用反馈与动作按钮。
- 审计：`chat_share_create` / `chat_share_revoke` / `chat_share_view`，payload 只含 `session_id` / `share_id` / `result`，**不含 token、消息正文或回答内容**。
- 日志：访问日志与内部错误日志把 `/api/ai/chat/shares/<token>` 脱敏为 `/api/ai/chat/shares/<redacted>`（`pkg/transport/http.RedactSensitivePath`），token 不进入任何日志。
- 分享不改变数据范围与权限判断：能打开链接的充要条件是「已登录 + `app:ai:chat`」，不额外做对象级数据范围过滤（会话不是告警类资源）。

#### 10.18.2 创建分享

- Method: `POST`
- Path: `/api/ai/chat/sessions/:session_id/shares`
- 鉴权: 是（`app:ai:chat`，且必须是会话 owner）

请求体（可选）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `expires_in_hours` | int | 否 | 1–720；缺省 168（7 天） |

响应 `data`：

```json
{
  "share_id": "sh-...",
  "session_id": "9b0f...",
  "status": "active",
  "expires_at": "2026-01-09T03:04:05Z",
  "created_at": "2026-01-02T03:04:05Z",
  "token": "<一次性明文 token>",
  "share_url": "/ai-assistant/shared/<token>"
}
```

错误：非 owner → 403 `PERMISSION_DENIED`；`expires_in_hours` 越界 → 400；有效链接已达上限 → 409 `FAILED_PRECONDITION`；未装配分享能力 → 503 `UNAVAILABLE`。

#### 10.18.3 分享列表

- Method: `GET`
- Path: `/api/ai/chat/sessions/:session_id/shares`
- 鉴权: 是（`app:ai:chat`，owner）

响应 `data`：`{ "shares": [ { share_id, session_id, status, expires_at, created_at } ] }`，含已撤销/已过期记录，按创建时间倒序；不返回 token 或哈希。

#### 10.18.4 撤销分享

- Method: `DELETE`
- Path: `/api/ai/chat/sessions/:session_id/shares/:share_id`
- 鉴权: 是（`app:ai:chat`，owner）

响应 `data`：更新后的分享视图（`status=revoked`）。已撤销时幂等返回当前视图；分享不属于该会话或不存在 → 404 `NOT_FOUND`。

#### 10.18.5 访问分享（只读）

- Method: `GET`
- Path: `/api/ai/chat/shares/:token`
- 鉴权: 是（`app:ai:chat`，任意具备该权限的登录用户）

响应 `data`：与 §10.5 相同的 `{ session, messages[] }` 结构；消息中的结构化 `answer` 正常解析。

- 过期、已撤销、token 不存在、会话已删除：**统一返回 404 `NOT_FOUND`**，避免探测链接是否存在过。
- 每次成功访问写 `chat_share_view` 审计（按访问者 `user_id` 记录）。

#### 10.18.6 前端接入

- 会话列表的「分享」打开弹窗：选择 1 / 7 / 30 天，生成后展示完整链接并支持复制；历史分享可撤销。
- 共享页路由 `/ai-assistant/shared/:token`：只读渲染，顶部提示不可提问/反馈/执行；加载失败展示「链接不可用」。
- 前端不持久化 token；链接只在弹窗内展示，关闭后需重新生成。
### 10.19 记忆手动管理（migration 0054）

回答注入仍严格按归属用户过滤（§10.14.2）；本节接口只面向管理员做人工检索与启停，不改变注入范围。

#### 10.19.1 权限与审计

- 权限码：`app:ai:memory:manage`（migration `0054`，仅绑定内置 `admin`）；未注入授权适配器的环境**不注册**路由。
- 审计：手动启停写 `memory_review`，`resource_type=ai`、`resource_id=case_id`，payload 含 `previous_status` / `status` / `result=success`。
- 不返回会话消息、Prompt、告警正文或凭据；列表只含脱敏摘要与建议。

#### 10.19.2 列表与检索

- Method: `GET`
- Path: `/api/ai/memory/cases`
- 鉴权: 是（`app:ai:memory:manage`）
- Query：`status`（`active` / `retired`）、`user_id`、`application_id`、`resource_id`、`keyword`（摘要/关键词/应用/资源模糊匹配）、`limit`（1–100，默认 20）、`offset`（≥0）。
- 响应 `data`：`{ "items": [ { case_id, source_type, session_id, user_id, application_id, resource_id, resource_type, alert_signature, keywords, summary, recommendations, risk_level, quality_score, status, created_at, updated_at } ] }`，按 `updated_at` 倒序。
- 非法 `status`、`limit`、`offset` 返回 400 `INVALID_ARGUMENT`。

#### 10.19.3 手动停用 / 启用

- Method: `POST`
- Path: `/api/ai/memory/cases/:case_id/status`
- 鉴权: 是（`app:ai:memory:manage`）
- Body：`{ "status": "active" | "retired" }`
- 行为：
  - `retired`：后续对话/分析检索不再命中；案例内容、质量分与来源关系不变。
  - `active`：重新参与检索；质量分保持原值，后续 invalid 反馈仍可按 §10.14.1 规则再次转 retired。
  - 目标状态与当前一致时幂等返回当前案例，不写审计。
  - `case_id` 不存在返回 404 `NOT_FOUND`；非法状态返回 400 `INVALID_ARGUMENT`。
- 响应 `data`：更新后的案例视图（字段同 10.19.2 的单项）。

#### 10.19.4 前端接入

- 管理入口：「系统设置 → AI 配置 → 记忆管理」页签（`web/src/views/system/ai-config/components/MemoryManager.vue`）。
- 支持按状态、摘要/关键词、归属用户筛选；详情弹窗展示摘要、建议、告警指纹与质量分；停用需二次确认。
- 无权限时接口返回 403，页面保留结构并提示权限不足。

#### 10.19.5 E2E

- `scripts/e2e-ai-memory.ps1`：自动覆盖「有效反馈沉淀 → 检索引用 → invalid 淘汰」。
- `scripts/e2e-ai-knowledge.ps1`：额外覆盖记忆列表检索与手动 retire 清理。
