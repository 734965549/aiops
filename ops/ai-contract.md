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
      "timeout_ms": 30000,
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
      "timeout_ms": 30000,
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

### 4.4 持久化与密钥约定

- provider 配置落库到 `ai_provider`（迁移 `0048`），数据库是事实源；`ai.providers` YAML 仅在新库/空表时作为种子写入，之后以页面配置为准。
- 页面入口位于「系统管理 / AI 配置」的 Provider 管理页签（`web/src/views/system/ai-config`）；AI 运维助手页只负责对话，不再承载 AI 配置。
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

## 7. Provider 类型说明

| 类型 | 说明 |
| --- | --- |
| `a` | 通用 HTTP API Key 接入：平台按 `POST <base_url>/v1/tools/invoke` 转发工具调用 |
| `b` | 平台直连模型：`base_url` 为含版本段的根地址（OpenAI 填 `https://api.openai.com/v1`，方舟填 `https://ark.cn-beijing.volces.com/api/v3`），平台追加 `/chat/completions`；`model` 必填，`response_format={"type":"json_object"}`，解析 `choices[0].message.content` 并做分析 JSON 校验 |
| `c` | 内部服务接入：兼容旧 `ai-orchestrator` 参考实现，按 `POST <base_url>/api/tool/invoke` 调用 |

类型 `b` 是推荐路径：平台内置固定 system prompt（服务端构建，不接受调用方 payload 覆盖），告警快照和指标证据由 AnalyzeService 组装为 user 消息。模型不可达、429、5xx、空响应或输出无法解析时，平台返回保守降级结果并标记 `degraded=true`，审计记为 `partial`。类型 `c` 仅为可选参考实现，不再是必需组件。

AI 运维助手（§10）只使用类型 `b` 做对话补全：没有启用中的类型 `b` 时对话返回 `status=degraded`、`degraded_reason=model_unavailable`，仍返回已获取的只读上下文，不返回任何执行建议。

## 8. 前端页面交互建议

### 8.1 Provider 管理页（系统管理 / AI 配置）

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
5. 会话与消息落库，工具调用、对话与反馈全部写审计。

约束：

- AI 不能直接执行、分发或调度任何任务；`execution.tasks.dispatch` / `execution.command.run` 保持不可达。
- 执行类入口只返回「生成执行计划」动作，前端调用既有 `POST /api/executions/tasks`（`source_type=ai_conversation`），任务进入 Execution `pending_confirm`，需人工输入 `CONFIRM` 后才会执行。
- 凭据、`api_key`、AK/SK、Token 不进入 prompt、日志、审计明文或前端持久化状态。
- 工具调用继承当前用户身份，服务端二次校验 RBAC 与工具权限；`allowed=false` 是业务拒绝，前端必须展示 `reason`。

### 10.2 权限码

| 权限码 | 说明 |
| --- | --- |
| `app:ai:chat` | 创建会话、查询会话、发送消息 |
| `app:ai:chat:feedback` | 提交回答有效/无效反馈 |

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
| `degraded` | 模型未配置、调用失败或输出非法；`answer.degraded=true` 且 `answer.degraded_reason` 为 `model_unavailable` / `model_failed` / `model_output_invalid`，保留已获取的只读上下文 |
| `insufficient_context` | 模型显式声明上下文不足（`answer.insufficient_context=true`） |

#### 10.6.2 `answer` 字段

`summary` / `evidence` / `impact` / `recommendations` / `risk_level` / `require_confirm` / `actions` / `rollback` / `uncertainties` / `insufficient_context`。

- `risk_level`：`low` / `medium` / `high` / `info`；模型返回 `critical` 时按 `high` 呈现。
- `actions` 由服务端按权限生成，**不采信模型输出**：
  - `execution.propose`：服务端校验 `execution.tasks.propose` 工具权限，`allowed=false` 时带 `reason`；
  - `view.alert` / `view.metrics`：只读跳转入口，`target` 为前端路由。
- `require_confirm`：模型声明或 `risk_level` 为 medium/high 时为 `true`。该字段只表达“建议需要人工确认”，不替代 Execution 状态机的确认。
- `uncertainties` 必须保留，前端需展示；降级回答必须给出不确定性说明。

#### 10.6.3 工具调用与审计

- 上下文中的 `alert_id` 会触发 `alarm.analyze` 权限校验并读取告警快照；可推导指标查询条件时触发 `metrics.query` 权限校验并采集只读指标证据。
- 每次工具校验/调用都写入审计：`resource_type=ai`、`resource_id=session_id`、`action=tool_invoke`，payload 含 `tool_code/allowed/mode/status/reason/result`，不含 prompt 与凭据。
- 回答写入审计：`action=chat_message`，含 `status/risk_level/require_confirm/tool_codes/tool_denied/reference_count/provider_id/tokens/degraded_reason`。
- 工具权限校验只做 RBAC 与工具权限，不做对象级数据范围判断：
  - 指标证据的数据范围由观测取数端口按账号权限二次校验；
  - 告警快照读取当前与 `POST /api/ai/analyze-alert` 一致，仅校验告警读取/工具权限，**尚未按用户数据范围过滤**（P1 待补，见 §10.11）。
  - 不在工具校验处强制数据范围，是为了避免无对象上下文的对话被误判为“缺少对象上下文”。
- 对话历史最多取最近 12 条送入模型；单条历史内容截断，不读取全量会话。

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
  "time_range": "30m"
}
```

- 字段均为可选业务标识；服务端只做长度裁剪与去空白，不信任前端做权限判断。
- 前端在告警详情页写入告警/应用/资源上下文，其它页面写入页面级上下文；发送消息时随请求携带，会话创建时也会保存一份。
- 上下文只存业务 ID 与标题，禁止写入凭据、Token、告警正文或自由文本。

### 10.9 前端状态映射（页面原型 §23.2）

| UI 状态 | 判定 |
| --- | --- |
| AI 思考中 / 查询工具中 / 生成回答中 | 请求进行中的阶段提示（同步接口） |
| AI 回答失败 | `status=degraded`，展示 `degraded_reason` 与可重试入口 |
| AI 无足够上下文 | `status=insufficient_context` 或 `answer.insufficient_context=true` |
| AI 建议需人工确认 | `answer.require_confirm=true` |
| AI 建议不可执行 | `actions` 中 `execution.propose.allowed=false`，展示 `reason` |

### 10.10 存储与迁移

- 迁移 `0049_ai_session`：`ai_session`（`session_id` 业务唯一列、`user_id`、`title`、`context_json`、`status`）与 `ai_session_message`（`message_id` 业务唯一列、`session_id`、`role`、`content`、`tool_calls_json`、`references_json`、`feedback`）。
- `assistant` 消息的 `content` 保存结构化回答 JSON；读取时由服务端解析为 `answer` 字段。
- 消息落库与会话 `updated_at` / 标题 / 上下文更新在同一事务内完成。
- 会话归属用户：跨用户访问返回 403；不暴露自增 `id`。

### 10.11 已知限制与 P1 待办

- **告警上下文未做数据范围过滤（P1，建议优先）**：`AlertReader.GetForAnalysis` 当前按 `alert_id` 直读，ChatService 的工具校验也只做 RBAC + 工具权限（与 `POST /api/ai/analyze-alert` 行为一致，非本次回归）。拥有 `app:ai:chat` 的用户可以把任意 `alert_id` 作为上下文读取告警快照。当前 `app:ai:chat` 仅绑定 `admin`，影响有限；P1 需要：
  1. 为告警读取补用户数据范围校验（按告警归属/部门/团队/区域等对象上下文）；
  2. 评估在 ChatService 工具校验中恢复数据范围判断（带对象上下文，避免误判）；
  3. 同步 `POST /api/ai/analyze-alert` 的同类缺口，保持一致。
- 同步接口：对话为一次性返回，流式与取消语义属 P1+。
- 工具范围：当前仅告警快照 + 指标证据（`alarm.analyze` / `metrics.query`）；日志、资产、变更类只读工具未接入。
- 资源级上下文：告警详情最完整；资产/执行/观测页当前仅页面级上下文。
- 会话能力：仅本人可见，分享未实现；归档后端已支持但 UI 未提供。
- 消息幂等：未引入 `client_message_id`，客户端超时重试可能产生重复用户消息。
