# internal/ai 模块说明

AI 上下文承载 Provider 配置、工具权限网关、告警分析、AI 会话与只读 Agent Loop、记忆/知识、分享、多模态和证据诊断。稳定接口以 `ops/ai-contract.md` 为准；本文先固定 Provider 与工具网关的当前实现，其他子能力仍按各自契约和后续复核记录判断完成边界。

AI 不是执行器。平台内置对话工具只做只读取数，执行提议只生成进入 Execution 状态机的任务；工具网关存在不代表可以绕过 Execution 直接派发或运行命令。

## 1. Provider 与工具网关能力边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| Provider 配置 | 列表、同 ID 新增/全量更新、删除；数据库落库并同步当前进程注册表 | 无健康检查、测试连接、默认标记、分页或独立启停路由 |
| Provider 协议 | 类型 `a/b/c` 三种执行器 | 类型 b 是当前分析与助手主路径；类型 c 只是可选内部服务协议 |
| 密钥 | `api_key` 经 Integration Vault AES-GCM 加密，列表只返回 `has_api_key` | 不返回 `api_key` 或 `****` 字段；空白更新保留旧密钥，不能借此清空 |
| 工具授权 | 复用 Identity RBAC、数据范围、AI 工具权限和确认标志 | 静态路由、直接调用和对话内置工具的数据范围入口不同，不能混写 |
| 直接工具调用 | `POST /api/ai/tools/invoke` 授权后向指定 Provider 转发 | 不是平台内置工具的统一执行入口；返回数据由对应 Provider/Executor 决定 |
| 对话内置工具 | 8 个只读工具经 `Gateway.Validate` 校验后由 AI application 本地执行 | 不经 AI Provider 转发；模型补全仍只使用启用中的类型 b |
| 执行类工具 | 部分权限种子和通用转发能力存在 | 无平台内置 Execution 执行器；Agent Loop 不接收执行类工具 |

## 2. 本轮相关路由与权限

| 方法与路径 | 权限 | 当前用途 |
| --- | --- | --- |
| `GET /api/ai/providers` | `app:ai.providers:read` | 从数据库读取全部 Provider，按 provider_id 升序 |
| `POST /api/ai/providers` | `app:ai.providers:write` | 同 ID upsert；不是通用部分更新 |
| `DELETE /api/ai/providers/:id` | `app:ai.providers:delete` | 删除数据库记录并移除当前进程运行时配置 |
| `POST /api/ai/tools/invoke` | 入口 `app:ai.tools:invoke`，随后网关继续校验 | 直接工具调试与外部 Provider 转发 |
| `POST /api/ai/analyze-alert` | `app:ai.analysis:analyze`，内部工具复用同一权限 | 读取告警后调用动态选择的 Provider |
| `GET /api/ai/chat/models` | `app:ai:chat` | 只列出运行时启用中的类型 b 模型摘要 |

Provider 路由使用 `AuthorizeStatic`，只检查 RBAC，不按 Provider 的 owner/team/region 做数据范围隔离。直接 `/tools/invoke` 的路由中间件没有跳过数据范围：绑定了非 all 数据范围的用户会因路由缺少对象上下文而在入口得到 403；进入 Handler 后，网关还会按请求中的对象字段、可选 resource/action 和 tool_code 再校验一次。

AI 工具权限字典和角色绑定属于 Identity：

- 字典读取：`GET /api/identity/ai-tool-permissions`，需要 `app:identity.ai_tool_permissions:read`，后端支持 `permission_mode` 过滤。
- 角色绑定查询/全量替换：`GET|PUT /api/identity/admin/roles/:role_id/ai-tool-permissions`，分别需要上述读权限和 `app:identity.access_control:write`。
- 权限替换写 `identity_role / set_role_ai_tools` 审计，失败不回滚已完成的授权变更。

## 3. Provider 生命周期与配置语义

### 3.1 数据库与运行时

```text
API 启动
  -> ai_provider 为空时才写入 ai.providers YAML 种子
  -> 从数据库读取、解密并加载到进程内 ProviderRegistry
  -> 单条解密/配置失败只跳过该条，不阻断 API 启动

页面/API upsert
  -> 校验配置与出站 URL
  -> 写 ai_provider
  -> 再更新当前进程 ProviderRegistry
```

数据库是列表和持久化事实源，运行时注册表是实际调用事实源。启动时加载失败的记录仍可能在列表里显示 `enabled=true`，该字段不是运行时健康状态。数据库写入与内存更新不是跨资源事务；多实例之间也没有配置变更广播，其他实例不会因本进程写入而自动刷新。

删除是数据库硬删除加当前进程运行时移除；当前仓储对不存在的 ID 也返回成功，响应仍可为 `{ "deleted": true }`。Provider CRUD 当前只写应用日志，没有 `provider_create/update/delete` 操作审计。

### 3.2 upsert 是全量替换

除 `api_key` 外，省略字段不会保留旧值：

- `headers` 省略会写为空对象。
- `enabled` 在 Go 请求结构中是普通 bool，省略即为 false；前端新建表单默认 true，但 API 调用方不能依赖该默认值。
- `model`、`description`、`reasoning_effort` 和 `vision_enabled` 按本次请求覆盖。
- 类型 a/c 的 `model` 不会被服务端强制清空；它只对类型 b 执行器有实际意义。
- 新增必须提供非空 `api_key`；更新省略、空串或全空白都保留原密文。没有单独的密钥清除接口。
- `timeout_ms<=0` 由应用归一化为 300000ms。迁移 `0048` 的数据库默认值仍是 30000ms，GORM 模型和应用默认值是 300000ms；经应用写入通常使用后者。
- `reasoning_effort` 只接受空、`low/high/max`；`vision_enabled` 只影响类型 b 图片输入。YAML 种子当前不下发这两个字段，分别按空和 false 处理。

支持的执行器是 a/b/c，但服务端没有独立的未知类型白名单拒绝；未知类型可能完成 URL 校验并落库，调用时才因缺少 executor 返回 `UNAVAILABLE`。联调和生产配置只能使用已支持类型。

### 3.3 密钥、headers 与出站安全

- 只有 `api_key` 使用专用密文列，响应不含明文或掩码字段。
- `headers` 以 JSONB 保存并在 Provider 管理列表原样返回，不是秘密存储；不要把 Authorization、Token 或其他敏感值放入自定义 headers。
- 只接受 HTTP/HTTPS，禁止重定向；默认拦截私有、链路本地、metadata、组播和未指定地址，loopback 仅在 dev 或显式配置允许时可用。
- `ai.outbound_allowed_hosts` 非空时按精确主机或点号后缀白名单限制；保存和真正 Dial 前都做地址校验。
- 保存时的校验会解析 DNS，但不调用模型或工具端点。因此保存成功、`enabled=true` 和 `has_api_key=true` 都不证明网络、凭据、配额或模型可用。

### 3.4 协议与选择顺序

| 类型 | 当前协议 | 必填与结果边界 |
| --- | --- | --- |
| `a` | `POST <base_url>/v1/tools/invoke`，`Authorization: Bearer <api_key>` | base_url/api_key；响应体上限 1MiB |
| `b` | `POST <base_url>/chat/completions`，OpenAI 兼容 JSON/SSE | base_url/api_key/model；非流式响应上限 4MiB，含有限重试和保守降级 |
| `c` | `POST <base_url>/api/tool/invoke`，默认 `X-API-Key: Bearer <api_key>` | base_url/api_key；响应体上限 1MiB，兼容可选 ai-orchestrator |

直接 `/tools/invoke` 始终使用请求指定的 provider_id，不自动换 Provider。告警分析从运行时启用项按类型 `b → c → 其他`、同类型按 provider_id 升序选择；没有持久化的“默认 Provider”字段。对话模型目录只含启用中的类型 b，默认取 provider_id 最小的项；请求/会话显式选择的模型不可用时，按对话契约回退默认项。

## 4. 工具网关的三条调用链

```text
A. 直接调试
   POST /api/ai/tools/invoke
     -> 入口 RBAC / 数据范围
     -> Gateway.Validate（可选操作权限 + AI 工具权限 + confirmed）
     -> 指定 ProviderRegistry.Invoke
     -> 类型 a/b/c 外部执行器

B. 告警分析
   AnalyzeService 读取告警并收集已授权证据
     -> Gateway.Invoke(动态 Provider, alarm.analyze)
     -> 类型 b 生成分析，或类型 a/c 转发内部分析服务

C. AI 会话 / Agent Loop
   ChatService 对每个内置工具调用 Gateway.Validate
     -> RequiredPermission=app:ai:chat，SkipDataScope=true
     -> 具体 Alert / Observability / Integration / Asset Port 再做对象或账号数据范围校验
     -> 本地生成只读观测
     -> 单独通过类型 b ChatClient 调用模型
```

因此，`alarm.analyze` 在告警分析链中会交给 Provider，在对话链中只是本地读取告警快照；不能把同一 tool_code 理解为永远走同一个外部端点。

### 4.1 授权与确认的实际语义

| permission_mode | 行为 |
| --- | --- |
| `read_only` | 不要求 `confirmed=true` |
| `require_confirm` | 默认要求 confirmed；`permits_unconfirmed_invoke=true` 时允许未确认调用 |
| `deny` | 无论 confirmed 与否都拒绝 |

数据库列 `allow_confirm` 当前直接映射为响应字段 `permits_unconfirmed_invoke`。现有 `execution.runbook`、`inspection.runs.create`、`execution.tasks.propose` 种子同时是 `require_confirm` 且该标志为 true，所以绑定这些定义的 admin 在通用工具网关中可以在未勾选 confirmed 时通过。这与工具调试页“require_confirm 类工具必填”的绝对文案不完全一致，只代表工具网关确认豁免，不会绕过 Execution 自身的 pending_confirm/CONFIRM 状态机。

`allowed=false` 是业务拒绝，通常仍返回 HTTP 200 + `code="OK"`，前端必须继续检查 `allowed/reason/mode`。`allowed=true` 只表示授权通过且 Executor 已返回；类型 b 还可能在 `data.degraded=true` 中给出保守降级结果，不能把它解释为真实模型分析成功。

### 4.2 当前对话内置只读工具

`toolgateway/builtin.go` 登记以下 8 个内置只读工具，均要求对应 AI 工具权限，并允许在受限 Agent Loop 中申请：

| tool_code | 本地执行能力 |
| --- | --- |
| `alarm.analyze` | 读取当前告警快照 |
| `metrics.query` | 查询并定稿指标证据 |
| `postgres.activity.query` | 查询 RDS for PostgreSQL 历史会话证据 |
| `alert.rules.list` | 查询告警规则 |
| `alert.rules.health` | 批量规则历史曲线体检 |
| `accounts.list` | 列出当前用户可见的启用 Integration 账号 |
| `resource.analyze` | 读取当前资源上下文并推导云查询参数 |
| `resources.list` | 查询本地云同步资源清单 |

`logs.search`、`execution.runbook`、`execution.media.list`、`execution.tasks.propose/dispatch` 和 `inspection.*` 等种子不等于平台内置工具已经实现。它们不在对话白名单；部分工具可在通用直接调用入口按权限转发给外部 Provider，但平台不保证 Provider 返回了真实查询结果或待确认 Execution Task。`cloud.*`、`logs.search` 和 `execution.tasks.propose` 的平台内置工具化仍是规划边界。

## 5. 审计边界

- Provider upsert/delete：只有应用日志，日志使用脱敏字段；没有操作审计动作。
- 直接 `/tools/invoke`：Handler 在调用后 best-effort 写 `resource_type=ai`、`resource_id=tool_code`、`action=tool_invoke`，Payload 含 provider_id/tool_code/resource/action/result；当前不写 `allowed/mode/reason`。只要 Gateway 没返回 Go error，即使 `allowed=false`，仍记为 `result=success`，不能据此判断业务已执行。
- 告警分析：写 `analyze_alert`，模型或工具降级记为 partial；不是由 Gateway 自动再写一条完整的 `tool_invoke`。
- 对话内置工具：ChatService 写详细 `tool_invoke`，含 allowed/mode/status/reason/evidence_id/result；对话再写 `chat_message`。这些记录与 Provider 配置日志不是同一审计通道。
- 以上操作审计均按现有 best-effort 口径处理，不与业务或 Provider 调用组成统一事务；Payload 不含 api_key，服务端也没有通用任意文本脱敏保证。

## 6. 前端实现与差距

入口为 `/system/ai-config`：

- `ProviderManager.vue`：列表、新增/编辑、启停和删除；类型 b 的 model、reasoning_effort、vision_enabled 与后端一致。
- `ToolConsole.vue`：选择 Provider，填写 tool_code/resource/action/payload/confirmed，展示原始 JSON，并对 `allowed=false` 弹出 reason。
- Identity 权限管理页：加载 AI 工具字典并对角色做全量绑定替换；403 时保留结构并禁用操作。

当前差距：

- 没有 Provider 健康状态或测试连接按钮，也没有启用状态筛选。
- 页面只显示“已配置/未配置”，不存在掩码值；表单提示中的“只显示掩码”不代表 API 返回 `****`。
- 页面编辑不携带 headers，会把通过 API/YAML 配置的自定义头清空。
- 工具调试页没有 owner_id/dept/team/region/tags 输入，也没有按工具定义动态阻止未确认提交。
- 对话右侧“可用工具”是固定目录，不读取当前角色的实际授权或 `permits_unconfirmed_invoke`。
- ProviderManager、ToolConsole 和 AI 工具角色授权没有专项前端 Vitest；页面错误主要依赖统一请求拦截器。

## 7. 迁移与验证覆盖

| 迁移 | 当前作用 |
| --- | --- |
| `0001/0002` | AI 工具权限表、Provider/直接调用 RBAC、基础工具与 admin 绑定 |
| `0010/0015/0016/0017` | 告警分析权限、工具权限管理、admin 权限修复 |
| `0020/0022` | Inspection/Execution 工具权限种子；不代表平台内置执行器 |
| `0048` | `ai_provider` 基表，无 Provider 记录种子 |
| `0053` | `ai_session_share` 会话只读分享；只存 token SHA-256，无外键 |
| `0055/0056` | 规则查询、账号/资源发现工具权限 |
| `0057` | Provider `reasoning_effort` |
| `0059` | Provider `vision_enabled` 与会话图片字段 |
| `0061/0062` | PostgreSQL 历史会话与批量规则体检工具权限 |

已有覆盖：

- `provider_service_test.go`：空库种子、数据库优先、坏密钥跳过、更新保留 key、新增必须 key、reasoning 校验、脱敏和删除。
- `handler_test.go`：Provider 列表不返回 api_key、CRUD 参数/错误和直接调用主路径。
- `provider_test.go` / `outbound_test.go`：注册表生命周期、禁用、拒绝后不调用 Provider、出站限制、鉴权错误和响应脱敏。
- `chat_completion*.go` 测试：类型 b 协议、重试、降级、流式和多模态的部分行为。
- `provider_repository_test.go` 当前主要是 headers 与表名，不是完整 PostgreSQL CRUD 集成测试。

`e2e-ai-mock-provider.ps1` 是其他 AI E2E 的共享 fixture：创建保留 ID 的类型 b mock Provider，并在结束时删除。它不是完整 Provider CRUD E2E。现有 `e2e-ai-*.ps1` 主要验证 mock/fake 下的对话、证据、规则和 Loop，没有直接 `/tools/invoke`、Provider 全量更新、角色工具权限绑定或真实外部模型的完整专项验收；部分脚本在无 active 告警时还可能 SKIP 后正常退出。

本轮只静态核对，未运行 Go 测试、前端 Vitest 或 AI E2E，不形成运行通过结论。

## 8. AI 会话与 Agent Loop

### 8.1 路由、归属与历史范围

| 方法与路径 | 权限 | 当前语义 |
| --- | --- | --- |
| `GET /api/ai/chat/models` | `app:ai:chat` | 启用中的类型 b 模型摘要，含 vision_enabled |
| `POST /api/ai/chat/sessions` | `app:ai:chat` | 创建当前用户会话，写 chat_session 审计 |
| `GET /api/ai/chat/sessions?limit=20` | `app:ai:chat` | 当前用户会话，1–100 条，updated_at/id 倒序 |
| `GET /api/ai/chat/sessions/:session_id` | `app:ai:chat` | 会话及**最近 200 条**消息，按时间升序 |
| `POST /api/ai/chat/sessions/:session_id/messages` | `app:ai:chat` | 同步执行同一对话编排并返回最终回答 |
| `POST /api/ai/chat/sessions/:session_id/messages/stream` | `app:ai:chat` | SSE 事件流，不是逐字渲染最终 JSON |
| `POST /api/ai/chat/sessions/:session_id/messages/:message_id/feedback` | `app:ai:chat:feedback` | 仅 assistant 消息，valid/invalid 覆盖 |
| `POST /api/ai/chat/sessions/:session_id/archive` | `app:ai:chat` | active → archived，历史只读 |
| `POST /api/ai/chat/sessions/:session_id/unarchive` | `app:ai:chat` | archived → active，可继续提问 |
| `DELETE /api/ai/chat/sessions/:session_id` | `app:ai:chat` | 同事务删除消息和会话，重复删除 404 |

会话归属由 application 的 `loadOwnedSession` 校验，跨用户返回 403，未找到返回 404。会话列表只按 user_id 过滤；静态 RBAC 不替代归属检查。详情不是全量历史分页接口，超过 200 条时只返回最新窗口，不能据响应长度判断数据库总消息数。

归档/恢复在目标状态相同时不推进 updated_at，但仍写 `chat_session_status` 审计；归档后仍可重放已生成回答，新提问才返回 409。删除只保证消息和会话在同一数据库事务，后续审计仍是 best-effort。

### 8.2 一次发送的事务与执行顺序

```text
校验 content/images/client_message_id/provider_id
  -> 校验会话归属
  -> client_message_id 命中则重放或 409（先于归档检查）
  -> 加载最近 12 条模型历史
  -> AppendMessage(user + title/context/updated_at，同一事务)
  -> gather：工具权限、对象/账号数据范围、只读观测
  -> 记忆/知识注入
  -> 类型 b 模型 + 受限 Agent Loop
  -> finalizeAnswer：服务端 actions/证据过滤/执行提议权限校验
  -> AppendMessage(assistant + updated_at，另一事务)
  -> chat_message 审计与最终 answer 事件（best-effort）
```

用户消息和助手消息是两个事务，不是一次原子提交。模型、采集、取消或助手落库失败时，已保存的用户消息不会自动删除；只有走到取消检查点时才按取消语义处理并写 `chat_canceled`。审计不进入上述事务。

`finalizeAnswer` 会额外调用 `execution.tasks.propose` 的工具权限校验，把结果追加到 `tool_calls` 并写 `tool_invoke` 审计，然后生成 `execution.propose` action。**这只是权限检查，不创建 Execution Task**。真实任务仍由前端调用 `POST /api/executions/tasks`，不能因为响应里出现该 tool_code 就认为模型已执行了任务提议工具。

### 8.3 消息幂等与模型选择

幂等键只在同一会话内生效，0050 的部分唯一索引兜底并发：

- 已存在用户消息和后续 assistant：返回首次 assistant 的 message_id、answer、tool_calls、references 和状态，不重新取数/调用模型，审计带 `replayed=true`。
- 只有用户消息没有 assistant：409 `FAILED_PRECONDITION`，无论上次仍在处理还是已经中断，调用方都不能假定正在运行。
- 并发撞唯一索引后回到上述两个分支；未传 key 不提供幂等。
- 服务端不比较请求内容 hash。相同 key 即使 content/context/provider_id 不同，也按首次结果重放，调用方必须为新的提问使用新 key。
- 后端没有会话级串行锁；不同 key 的并发消息不属于该幂等保证。`FindAssistantAnswer` 取该用户消息之后的第一条 assistant，不是独立的外键关联。

模型选择优先级为请求 provider_id > 会话 context.provider_id > provider_id 最小的启用类型 b。请求或会话指定模型不可用时回退默认并在 uncertainties 首条说明；没有可用默认模型时返回 degraded/model_unavailable。

顶层 `provider_id` 只在会话尚未保存模型时写入会话上下文。已有模型后，API 调用方若要持久切换，需同时更新 `context.provider_id`；否则只影响本次请求。当前前端会同时发送顶层和 context 两个字段，因此页面切换可以保存，但不能把该行为推广到只传顶层字段的自定义客户端。

### 8.4 受限 Agent Loop 的实际约束

- 白名单固定为 8 个本地只读工具，执行类工具申请只生成 denied 观测，不调用 Execution。
- 最多 4 个追加轮（含实际取数和无进展轮；纯重复申请不占），单轮最多 4 个不同 `tool_code+params` 申请；同工具不同参数可分别执行。
- token 预算为 Provider 回报的累计 60000，达到后停止继续取数；Provider 不返回 usage 时不能据本地字符数推断等值 token。
- 整回合总预算默认 600 秒，包含首轮 gather、Loop 和强制收敛；超时返回 degraded/turn_budget_exceeded。
- 轮内同参数去重；跨轮只对**已经成功执行**的同参数组合拒绝重复，失败或非法参数可在后续轮修正重试。纯重复申请不占有效轮数。
- 连续两轮没有任何实际取数，或轮数/token 预算耗尽，会先做一次强制收敛：禁止新工具，只基于已有 observations 作答。
- 强制收敛成功可返回 ok 或 insufficient_context，审计 `forced_convergence=true`；只有仍失败/输出非法才 degraded/agent_loop_exhausted。时间预算耗尽不走该强制轮。
- `loop_rounds` 是实际追加取数轮数，不包含强制收敛；所有工具观测进入 tool_calls 和详细 tool_invoke 审计。

**证据覆盖限制**：`ListAlarmRules` 不携带完整资源明细，规则资源优先通过 CES `ListAlarmRuleResources` 补全并用其 count 校正 `resource_count`，并透传 `resource_level` / `product_name`（维度名）；资源分组型规则再展开分组（单次规则列表最多 10 条、每条最多 3 个实例样本）；仍无资源维度时按 namespace + 规则维度名从本地云同步资源发现最多 3 个实例兜底（`scope_source=resource_discovery`，此时 `resource_count` 为空），并按规则名/描述中的引擎线索优先匹配同引擎实例，避免 PG 规则抽到 MySQL 实例。每条规则对最多 3 个实例取数、取越限最严重的样本；只有连实例发现也取不到时才 `no_resource_scope`。单次体检默认预算 60 次指标查询（总超时 75s），按广度优先分配：先保证每条规则至少 1 个实例，有余量再补第 2/3 个；`point_count=0` 的规则会追加近 1 小时/60s 探测（`probe_point_count`，单次最多 6 条），区分“近期无数据”和“窗口无点”。进入 Prompt 的曲线最多 288 点，峰值时刻（`peak_at`）、破限片段与越限时长在降采样前按全量序列计算并保留；长窗口小时级聚合仍不能还原规则原始秒级窗口。排查时应先核对 `scope_source`、`dimension_name`、`checked_resources`/`sampled`、`probe_point_count`、截断标记、聚合 period 与实际执行轮数，再决定是否缩窗复查。

### 8.5 SSE、取消与模型重试

服务端事件为 `stage/tool/delta/answer/error`：

- stage 为 gathering/thinking/answering；tool 是每次工具观测摘要；delta 分 thinking 与 answer。
- thinking delta 可在前端过程面板展示，answer delta 只计数，不渲染半成品 JSON；最终回答仍以 answer 事件的完整结构为准。
- 每 15 秒发送 `: ping` 注释，业务事件与心跳在同一 ResponseWriter 上串行写入；前端忽略注释和未知事件。
- 请求体错误在写 SSE 头之前返回 400；开始 SSE 后的会话不存在、归档、幂等冲突等通过 error 事件返回。
- 客户端取消或事件回调失败在 before_gather/before_model/before_persist/before_answer 检查点停止，不写 assistant，保留 user，并写 chat_canceled。
- assistant 落库后的最终 answer 推送是 best-effort：此时客户端断开不会回滚 assistant，也不会补写取消审计，不能绝对表述为“任何断流都不落回答”。

模型流式首段非空 SSE 行预算 60 秒，之后按连续无字节空闲计时，代码默认 180 秒，可由 `ai.stream_idle_timeout_s` 调整；任意非空行（含注释心跳）都会重置。整回合仍受 600 秒总预算限制。

未向客户端输出任何 delta 前，429、5xx、网络错误、首段/空闲超时、空响应可按预算最多尝试 3 次；其他 3xx/4xx 不重试。已输出 delta 后不重试，读流中断、空闲超时、空内容或输出长度超限会尽量形成 incomplete 并落库。429 的 Retry-After/配额恢复时长只保留时长信息，不写原始响应体。

### 8.6 独立工作台的当前差距

`useChat` 独立工作台使用流式接口，会话列表、创建/详情、归档/恢复/删除、模型切换、实时工具、停止和执行提议均已接线，但当前实现与理想交互仍有差距：

- ChatPanel 在归档提示前先清空输入和附图，不能保证“归档拦截不丢输入”。
- ModelControl 支持 disabled，但 ChatPanel 没有传 sending，生成期间模型选择器并未禁用。
- 刷新按钮只刷新会话列表；已加载会话没有通用的手动重新获取当前详情入口，409 文案所说的“刷新会话”需要额外导航或重新加载页面。
- 重试复用原 client_message_id。已有 degraded/incomplete 回答时只会重放旧结果，不会再次调用模型；乐观消息也没有按 message_id 去重，可能本地重复显示。
- 历史详情加载会清空重试内容，却仍恢复 failed/incomplete 状态，因此历史回答的重新提问按钮可能不可用。
- 无 answer/error 的连接中断最终按 failed 显示，即使回读已找到落库回答；主动 stop 与传输中断需要区分验收。
- 右侧工具目录是固定展示，包含不属于 Loop 白名单的 execution.tasks.propose；多轮工具卡片的 key 只有 tool_code，且右侧只保留每个工具最后一次状态，不是完整 Loop 时间线。
- 同步 `sendChatMessage` 封装为 90 秒 Axios 超时，与后端默认 600 秒 Loop 回合预算不匹配；独立工作台走 SSE，不受该同步封装超时影响。

### 8.7 会话与 Loop 验证覆盖

已有后端测试覆盖 domain 状态/标题/上下文、ChatService 创建/归属/归档/删除、同步与 SSE、顺序和并发幂等、取消、模型选择与回退、Loop 白名单/轮数/去重/无进展/强制收敛、工具参数和多种降级。Handler 测试覆盖 DTO、列表 limit、SSE 头/事件、错误和会话生命周期；toolgateway 测试覆盖模型重试、限流、部分输出和多模态。SessionRepository 当前测试主要是表名与 JSON 序列化，不是 PostgreSQL 事务/唯一索引集成测试。

前端现有 useChat Vitest 主要覆盖跨会话并发和同会话串行，SSE parser 覆盖基本分片/多行 data/取消识别，另有工具原因和 AnswerCard 复制测试。上述归档输入、模型禁用、重试重放、历史重试、409 回读、模型切换和完整 Loop 展示没有专项前端测试。

`e2e-ai-assistant.ps1` 覆盖同步主路径、执行提议、顺序幂等、反馈、归档/恢复、删除和部分审计；degraded 仍可能只做结构断言，业务会话/消息/执行任务/审计保留，只有 probe 会话删除。`e2e-ai-stream.ps1` 覆盖 SSE、429、部分输出、可选慢流、归档、幂等和取消，`-SkipSlow` 会跳慢流，业务数据不清理。`e2e-ai-agent-loop.ps1` 覆盖只读 Loop、执行拒绝、发现和 3 轮耗尽；没有 active 告警会打印 SKIP 后 exit 0，也没有覆盖全部预算、去重和强制收敛分支。三者都是 API/curl 脚本，不是浏览器 UI 验收。

本轮仅静态复核，未运行 Go 测试、前端 Vitest 或这些 E2E，不形成运行通过结论。

## 9. 相似案例记忆与知识沉淀

### 9.1 记忆案例的生命周期

当前只有 AI 助手回答的 `valid` 反馈会沉淀案例；`invalid` 只对已有案例降分。`source_type` 枚举包括 `chat_message`、`alert_analysis` 和 `knowledge_item`，但后两者没有实际写入方：

```text
SubmitFeedback(valid)
  -> 解析 assistant 回答 JSON
  -> degraded=false 且 summary 非空才沉淀（incomplete/降级回答不会入记忆）
  -> 按 AlertReader 读取会话告警生成指纹；失败仅省略 signature
  -> 记录 chat_message、source_message_id、application/resource、keywords、summary/recommendations/risk_level
  -> ai_memory_case 按 (user_id, source_message_id) 部分唯一索引幂等；重复反馈返回 created=false

SubmitFeedback(invalid)
  -> 按 user_id + source_message_id 查已有案例
  -> quality_score -2；score<=-2 转 retired；不存在时 no-op
```

`quality_score=1` 表示第一次 valid；`invalid` 不要求案例仍在 active，`ApplyFeedback` 对 retired 继续降分但不会再改回 active。手动启用只是恢复检索，质量分保持原值。

检索只查当前用户、`status=active` 的案例，最多 3 条。SQL `match_rank` 与领域优先级一致：`alert_signature` 精确命中 3、`application_id` 或 `resource_id` 命中 2、keywords 全文命中 1；同分按 `quality_score`、`updated_at`。signature 的维度只取 `instance_id/cloud_resource_id/pod/node`，keywords 由摘要、建议、告警名、资源类型和用户问题按分隔符切词生成，PostgreSQL 端用 `simple` 分词。回答 `references` 的 memory_case label 是“历史案例：<摘要>”，摘要最多 80 runes，无 URL。

自动 valid/invalid 引起的沉淀或降分不写独立操作审计；其事实反映在 `chat_feedback` 与 `chat_message.memory_used/memory_status`。管理端手动启停才写 `memory_review`；目标状态相同时幂等返回当前案例且不写审计。0051 无外键，案例删除不随会话删除级联。

### 9.2 知识条目、审核与反馈统计

`ai_knowledge_item` 只有 `active` 会注入；`draft` / `disabled` 不进 Prompt，也不产生 references。注入按 scope 匹配 global/application/resource_type，取数先由仓储筛 active 目标，再按 hit_count 倒序、updated_at 倒序选择；单次最多 5 条，标题+内容累计最多 3000 runes，超长条目直接跳过而非截断。附加段由服务端生成，固定包含 item_id、标题和内容，不接受模型或前端覆盖。

命中统计在回答落库后执行；`valid` 反馈给本轮 references 中的条目 useful_count +1，并评估自动停用。`invalid` 不减 useful_count，只执行评估：active、`hit_count>=5` 且 `useful_count=0` 时转 disabled，清空 reviewer 并写 `knowledge_review(auto_disable)`。该评估 best-effort，不阻断反馈接口。

管理接口：

| 方法与路径 | 当前语义 |
| --- | --- |
| `GET /api/ai/knowledge/items` | status/scope 精确过滤；limit 1–100 默认 20，offset 分页 |
| `POST /api/ai/knowledge/items` | manual 来源固定 draft |
| `PUT /api/ai/knowledge/items/:item_id` | 全量替换 scope/scope_ref/title/content；active 回 draft 并清空 reviewer，但保留 hit/useful 计数与 origin |
| `POST /api/ai/knowledge/items/:item_id/review` | activate/disable 前只校验存在；activate/disable 分别写 reviewed_by/reviewed_at |
| `DELETE /api/ai/knowledge/items/:item_id` | 硬删除；重复删除 404；审计 `knowledge_review(action=delete)` |
| `POST /api/ai/knowledge/aggregate` | 反馈聚合，见 §9.3 |

| `POST /api/ai/memory/cases/:case_id/status` | active/retired 间流转；同状态幂等返回当前案例且不写审计 |

自动停用属于机器状态变更，仍归在 `knowledge_review`，审计 actor 为空。没有单独的条目历史或版本接口。

### 9.3 反馈聚合与边界

```text
POST /api/ai/knowledge/aggregate
  -> application_id 必填
  -> 读取当前操作者在该应用下 status=active 的记忆案例
  -> 不足 min_cases 返回 insufficient_cases，不建条目
  -> 足够时按 scope=application + origin=feedback_aggregate 查同一条目
  -> 不存在则 created；draft/disabled 则 updated；active 则 skipped_active 且不覆盖
```

`min_cases<=0` 使用 3。它只是“至少需要多少案例”的阈值，服务端没有将其限制为最大 20 的入参校验；仓储聚合取数上限固定为 20，传入更大阈值通常只会得到 insufficient_cases。生成条目 ID 固定为 `agg-<application_id>`，内容按案例列出摘要和最多 2 条建议，单条摘要最多 300 runes；标题为“应用 <id> 历史有效经验”。聚合只读当前操作者自己的案例，不跨用户聚合。

### 9.4 管理端与验证边界

- 记忆管理路由要求 `app:ai:memory:manage`（0054），知识管理要求 `app:ai:knowledge:manage`（0052），未注入授权适配器时对应路由不注册。管理端是静态 RBAC；列表能按 `user_id` 查询其他用户案例，但没有对象级数据范围过滤。
- `GET /api/ai/memory/cases` 支持 status/user_id/application_id/resource_id/keyword/limit/offset；keyword 对 summary/keywords/application_id/resource_id ILIKE。`session_id` 有值才返回。
- MemoryManager 页面目前只传 status/keyword/user_id，`limit=100`，未使用 application_id/resource_id/offset，也没有分页控件；详情未展示 session_id。KnowledgeManager 未展示审核人/审核时间/创建者，聚合 UI 固定默认 min_cases=3，激活/停用没有二次确认（仅删除有）。两者均无前端 Vitest。
- 后端已有 domain、service、management、aggregate、chat 注入、analyze similar_cases 和 Handler 测试；仓储测试主要是 JSON 序列化口径，不是 PostgreSQL 检索/索引集成测试。
- `e2e-ai-memory.ps1` 不使用管理列表、不校验 quality_score/幂等；首条回答 degraded 会 SKIP 后 exit 0，业务会话/消息/审计保留，案例最终为 retired。它没有直接覆盖 9.9.1 的管理口径、跨用户、error 或 memory_review。
- `e2e-ai-knowledge.ps1` 需要 0052+0054 和 dev/mock，finally 只删除残留条目、retire 残留案例、清理 mock，会话不删。它部分覆盖 draft/激活/注入/停用/valid/聚合/记忆列表/retire，不覆盖 403、编辑回草稿、自动停用、聚合幂等/skipped_active/insufficient_cases 的全部断言或 UI。

本轮仅静态核对，未执行 Go 测试、前端 Vitest 或这两个 E2E，不形成运行通过结论。

## 10. 会话只读分享与图片输入

### 10.1 分享路由与装配

| 方法与路径 | 权限 | 当前语义 |
| --- | --- | --- |
| `POST /api/ai/chat/sessions/:session_id/shares` | `app:ai:chat` | owner 创建只读分享，返回一次性明文 token |
| `GET /api/ai/chat/sessions/:session_id/shares` | `app:ai:chat` | owner 查询会话全部分享记录（含撤销/过期） |
| `DELETE /api/ai/chat/sessions/:session_id/shares/:share_id` | `app:ai:chat` | owner 撤销分享，幂等 |
| `GET /api/ai/chat/shares/:token` | `app:ai:chat` | 任意具备权限的登录用户只读访问共享会话 |

分享是 ChatService 的可选能力：`SetShares` 未注入仓储时四个接口返回 503 `UNAVAILABLE`，其余对话能力不受影响；`cmd/api/main.go` 当前已注入 `ai_session_share` 仓储，因此标准装配下能力可用。创建/列表/撤销先经 `loadOwnedSession` 校验归属，非 owner 返回 403；访问分享不校验会话归属。分享路由挂在 `chatHandler != nil` 分支下，与其它会话路由一致。

### 10.2 分享生命周期与安全语义

```text
CreateShare
  -> 校验归属 -> TTL 归一化（0=默认 168h，合法区间 1–720h）
  -> 统计同会话 active 且未过期的链接数，>=10 返回 409
  -> 生成 32 字节随机 token（base64url，43 字符），落库只存 SHA-256 十六进制
  -> 审计 chat_share_create（payload 含 session_id/share_id/expires_at/result）
  -> 明文 token 与 share_url=/ai-assistant/shared/<token> 只在本次响应返回

ViewSharedSession
  -> 按 token 哈希查记录 -> Usable 校验（撤销/过期不可用）
  -> 加载会话与最近 200 条消息（同 §8.1 详情窗口）
  -> 审计 chat_share_view 按访问者 user_id 记录
```

- 过期、已撤销、token 不存在、会话已删除统一返回 404，不能区分“曾经存在过”；撤销时跨会话的 `share_id` 也按 404 处理，避免探测。
- 删除会话**不会级联删除**分享记录（0053 无外键，仓储也没有删除方法）；链接失效由访问时“会话不存在 → 404”保证，`ai_session_share` 中的残留记录会一直存在。
- 撤销幂等：已撤销再撤销返回当前视图且不更新数据，但**每次调用仍会写一条 `chat_share_revoke` 审计**，与记忆状态幂等不写审计的口径不同。
- 分享列表按 `created_at` 倒序返回全部记录，不返回 token 或哈希；已归档会话的分享仍可访问（归档只限制新提问）。
- 审计 payload 不含 token、消息正文或回答内容；访问日志与请求日志中间件通过 `pkg/transport/http.RedactSensitivePath` 把 `/api/ai/chat/shares/<token>` 脱敏为 `/api/ai/chat/shares/<redacted>`。
- 分享不改变数据范围判断：能打开链接的充要条件是已登录且具备 `app:ai:chat`，不做对象级数据范围过滤。

### 10.3 图片输入（migration 0059）

`normalizeChatImages` 校验：最多 3 张；仅接受 `data:image/(png|jpeg|webp);base64,` 前缀；base64 解码后单张 ≤2MB 且非空。远程 URL、其它 MIME、非法 base64、空 payload 都是 400 `INVALID_ARGUMENT`。文本为空但带图时服务端补默认提问“请分析这张图片。”；文本与图片均为空在写 SSE/落库之前返回 400。

视觉门控发生在**幂等重放与归档校验之后、用户消息落库之前**：本轮实际模型未开启 `vision_enabled` 时返回 400，不落用户消息、不调用模型。本轮模型按 请求 `provider_id` > 会话 `context.provider_id` > 默认模型 解析，请求/会话指定的 provider 不在目录中时按默认模型的视觉开关判断，运行时无模型目录时直接 400。归档会话带图发送先命中归档 409，不会走到视觉校验。

```text
sendMessage
  -> normalizeChatImages（400 不落库）
  -> 幂等重放 / 归档校验
  -> chatVisionSupported(requestedProviderID)（400 不落库）
  -> AppendMessage(user, images 落 images_json)
  -> generateAnswer：首轮 + Loop 各轮 + 强制收敛轮均携带 images
```

- 类型 b 执行器把带图 user 消息构造为 OpenAI 兼容 content 数组（`text` + `image_url`）；无图消息仍是纯字符串，报文不变。registry 未实现视觉通道时 `completeModel` 明确报错，正常流程已被前置校验拦截。
- 送入模型的会话历史（最近 12 条）只保留 Role/Content 文本，不回放历史图片；图片只在当前回合的模型调用中携带。
- `ai_session_message.images_json`（JSONB，可空）只写 user 消息；assistant 消息不写。会话详情与分享访问的消息视图返回 `images`（omitempty，仅 user 消息非空时）。
- `chat_message` 审计 payload 增加 `image_count`；图片 base64 不进入日志、审计 payload 或 Prompt 文本字段；记忆/知识沉淀不读取图片内容。

### 10.4 前端实现与差距

- `ProviderManager.vue` 以「视觉能力」开关维护 `vision_enabled`；`GET /api/ai/chat/models` 返回 `vision_enabled`，聊天页据此启用/禁用图片按钮。
- `ChatImagePicker.vue` 支持点击上传与粘贴（ChatPanel 的 paste 事件转 `addImageFiles`）；≤300KB 原样读取，>300KB 经 canvas 缩放最长边 1280 后按 JPEG 0.85 重编码，压缩后仍超 2MB 拒绝；预览条可删除，非视觉模型下按钮禁用且粘贴给出提示。
- `ChatUserMessage.vue` 用 `ChatMessageImages.vue`（a-image，可点击预览）渲染用户附图缩略图；独立工作台与全局浮层共用同一图片通道。重试复用同一 `client_message_id` 并重新携带附图，服务端按幂等重放，不重复调用模型。
- `ShareDialog.vue` 提供 1/7/30 天有效期选项，生成后弹窗内展示完整链接并复制，历史分享可撤销；token 不持久化。
- **当前差距**：`SharedSession.vue` 的用户气泡只渲染文本，未复用 `ChatMessageImages`，因此分享页不展示附图——API 已返回 `images` 字段，仅前端渲染缺失，验收 9.16.6 的“分享页只读展示图片”当前不成立。ShareDialog/SharedSession/useChat 图片链路也没有专项 Vitest（`imageInput.spec.ts` 只覆盖工具函数本身）。

### 10.5 验证覆盖

- `chat_share_test.go`：token 哈希与默认 TTL、TTL/owner 校验、active 上限、只读 active 链接、过期与已删会话隐藏、owner 与跨会话撤销、未装配 503。
- `chat_images_test.go`：normalize 规则（数量/MIME/远程 URL/非法 base64/空 payload/超限）、视觉通道调用与落库、纯图片默认提问、非视觉模型 400 不落库、无目录 400；`chat_vision_test.go`：同步/流式多模态报文与无图纯文本报文。
- `e2e-ai-assistant.ps1` 的 share 步骤覆盖创建 24h 分享、访问链接、撤销后 404 与 `chat_share_create/revoke/view` 审计；不覆盖分享列表接口、过期链接、owner 边界、active 上限（由单测覆盖）与跨账号语义（需人工）。**图片链路没有任何 AI E2E**，报文与拒绝语义依赖单测，视觉效果需真实模型人工验证。

## 11. 全局浮层与消息排队

对话状态与排队全部在前端 `useChat` 组合式函数内维护，后端没有排队或会话锁概念；同一会话的并发消息仍按 §8.3 幂等语义处理（不同 key 各自成轮，无串行保证）。

### 11.1 挂载与生命周期

```text
BasicLayout（登录后的统一布局）
  -> provide(ChatControllerKey, useChat())  // 全应用唯一控制器实例
  -> AIAssistantOverlay 通过 useSharedChat() inject 同一控制器
  -> 独立工作台 /ai-assistant 同样 inject，浮层与工作台共享会话、消息与队列
```

- 控制器挂在布局层，路由切换不销毁：跨页导航时后台生成与本地消息保留；登出或布局卸载时 `onUnmounted` 中止全部流并清空队列。
- 所有状态只在内存中，不写 localStorage；刷新页面即丢失排队条目与乐观消息，正在生成的回合由服务端按取消检查点或正常完成落库。
- 浮层是右侧抽屉（480px，窄于 560px 时全宽），`unmount-on-close` 只卸载面板 DOM，控制器状态不随之丢失。

### 11.2 浮层入口与页面上下文

- 入口：顶栏「AI 助手」按钮、快捷键 `Alt + A`（toggle，带确认弹窗等场景也可用）、告警详情的「问 AI 助手」。前两者为默认入口；告警页 `askAI` 先 `syncAlertAIContext` 写入告警上下文，再以 `openOverlay({ selectSession: true })` 打开**会话选择器**。
- 默认入口打开时若尚无激活会话，自动加载会话列表并选中最近会话；选择器入口只列会话不自动打开，可继续历史会话或新建会话（新建会话携带当前页面上下文，如 `alert_id`）。
- 抽屉顶部展示 `aiContext.summary`；页面上下文写入方与字段见契约 §10.8（告警详情、资产、执行、观测查询写资源级字段，路由切换只保留同页资源，其余清理）。`stores/aiContext.spec.ts` 覆盖凭证类自由文本丢弃、`execution_id` 视为资源级、同页保留与显式选择器入口等 6 项。
- 「完整工作台」、回答引用导航（`onNavigate`）和「生成执行计划」成功后都会先关闭浮层再跳转；执行提议跳转 `/executions?task_id=` 并进入 pending_confirm。

### 11.3 同会话排队

```text
send(content)
  -> 归档会话直接拦截提示，不进队列
  -> 目标会话 state.sending=true 时：构造 QueuedChatMessage 入队后返回
       （content/images/context/provider_id/client_message_id 均按入队时刻快照冻结）
  -> 否则 sendNow 直接发送

sendNow 完成/失败 finally
  -> sendNextQueued：取出队首自动发送，直到队列清空
```

- 队列条目独立生成 `client_message_id`；排队期间切换页面、模型或上下文不影响已入队条目的快照。
- 生成中点击停止：按钮文案变为「停止并清空队列」，`stop()` 清空当前会话队列并中止当前流；服务端按取消检查点保留 user、不写 assistant。
- 队列管理（置顶/删除/清空）只作用于**当前激活会话**的队列；后台会话的队列需要切回该会话才能管理，会话列表以「生成中」标记后台在途会话。
- 删除会话会先中止该会话流并清空其队列，再调用删除接口。
- 已归档会话的排队条目发送时被服务端 409 拒绝，队列不会因此整体终止：剩余条目会逐条尝试、逐条失败，最后一条错误留在面板。
- 重试按钮复用旧幂等键与附图（§8.6 缺口不变）；若当前会话生成中，重试同样进入队列而非并发请求。

### 11.4 跨会话并发

每个会话有独立 `SessionChatState` 与 `AbortController`，不同会话可同时生成，互不串消息；`selectSession` 命中本地已加载、生成中或有队列的状态时直接接管本地状态，不回源覆盖乐观消息。同步阻塞只存在于同一会话内。

### 11.5 验证覆盖与差距

- `useChat.spec.ts`：1 个用例覆盖跨会话并发、同会话第二条入队、首条完成后自动发送与列表 `sendingSessionIds` 标记。
- `BasicLayout.spec.ts`：4 个用例覆盖共享控制器跨页语义——路由重挂载不换控制器实例、在途流不被中止（整布局卸载才 abort）、跨页队列顺序/置顶/删除与「停止并清空队列」。
- `AIAssistantOverlay.spec.ts`：4 个用例覆盖选择器不自动打开、选择历史会话、带告警上下文新建会话与默认入口自动选中。
- `QueuedMessageList.spec.ts`：1 个用例覆盖置顶/删除/清空事件透传；`chatSessionState.ts` 类型无独立测试。
- 当前差距：归档会话排队逐条失败、删除会话清理、`Alt + A` 快捷键与窄屏宽度无专项测试；排队条目与乐观消息不持久化，刷新后丢失，不能把浮层当作可恢复的工作区。

## 12. RDS 长事务历史会话诊断

### 12.1 触发与两条取数入口

诊断只在 `SYS.RDS + oldest_transaction_duration` 长事务场景生成：告警 labels/annotations 命中该命名空间与指标，或本轮指标证据已取到该指标数据。文本对话与其它告警类型不会出现 `diagnosis`。

```text
Chat gather    collectPostgresObservation：长事务告警才自动取数（先 checkTool 校验 AI 工具权限）
Chat Loop      refreshPostgresObservation：模型显式申请 postgres.activity.query；
               params 支持 from/to（必须成对，缺省取告警证据窗口）与 limit（0–300）
Analyze        collectPostgresEvidence：告警分析入口，同样先校验工具权限
               -> derivePostgresEvidenceQuery：account/region/project/instance/窗口按告警或资源上下文推导
               -> PostgresCollector.Collect（ak_sk 账号 + 数据范围校验 + 15s 预算）
               -> finalizePostgresEvidence：先落库定稿快照，再进入诊断与模型
               -> buildPostgresDiagnosis
```

查询参数不允许自由指定连接地址或实例：`instance_id` 从告警 labels（`rds_instance_id`/`rds_cluster_id`/`cloud_resource_id`）、指标维度或资源 `cloud_resource_id` 推导；缺失时按 `no_rds_instance` 跳过。

### 12.2 采集与脱敏边界

- Collector 校验 `no_account/no_region/invalid_rds_instance_id`（仅允许 `[A-Za-z0-9-]{1,128}`）、时间范围非法或超 7 天时按 `skipped` 返回；账号非 `ak_sk` 按 `unsupported`；数据范围不足按 `permission_denied`；账号/凭据/project_id 解析失败按 `error`。
- 三组接口（历史会话、等待事件、Top SQL）共享 `ai.postgres_evidence_timeout_ms` 总预算（默认 15000ms）；每页 100、单类最多 300 行，分页保留上游 `total_count`（省略时 -1，达到预算行数仍按 `truncated=true`）。历史会话是主证据：主接口失败整体 error/permission_denied；等待事件或 Top SQL 失败保留会话事实，进 `missing_evidence/collection_errors`，不改写成“没有等待/没有 SQL”。
- 脱敏在映射层完成：用户名转 `user_hash`（SHA-256 前 16 位）；客户端地址 IPv4 保留 `/24`、IPv6 保留 `/64` 网段，非 IP 标识哈希化；`sql_statement` 原文不保存，只留规范化空白后的 `sql_hash`、深度 0 关键字解析的 `sql_operation` 和最长 300 runes 的 `sql_summary`。摘要把字符串/数字字面量统一替换为 `?`，移除行注释、嵌套块注释与 dollar-quoted 内容；未闭合字符串从起点截断，异常 SQL 不会泄露原文。
- 定稿快照先落库再进入模型：payload 超限时先丢 Top SQL、再丢等待事件、逐条丢会话，仍超限按编码失败处理；快照存储未装配或写库失败时，原本 `ok` 的结果**fail-closed** 转为 `error`（`postgres_snapshot_store_disabled`/`postgres_snapshot_persist_failed`）并清空 evidence_id，不把未持久化现场当事实。非 OK 结果清空记录、`retained_count=0` 并追加 `postgres_activity` 缺失。
- 会话排序：锁等待优先，其次事务时长倒序、采样时间倒序、session id 升序；`evidence_type=postgres_activity`、`schema_version=ai.postgres_activity.v1`。

### 12.3 诊断结构与根因边界

```text
buildPostgresDiagnosis
  -> facts = 指标事实（first/peak/last/破限点/保留点）
           + 最多 6 条会话事实（状态、事务时长、采样、应用、Query ID、等待事件、SQL 操作/摘要、阻塞进程）
           + 最多 3 条与会话 Query ID 匹配的 Top SQL 脱敏事实（不匹配不进入）
  -> root_cause 分类：lock_wait > idle_in_transaction > active_long_query > undetermined
  -> undetermined 时生成空闲事务/活跃长 SQL/锁等待三类待验证假设
  -> summary/evidence/recommendations/uncertainties 由服务端按分类覆盖生成
```

| code | 判定 | status | 输出约束 |
| --- | --- | --- | --- |
| `lock_wait` | `wait_event_type=Lock` 或有效阻塞进程（`0`/`-1`/空不算） | suspected | 有无直接阻塞者分别陈述；“阻塞链根因仍需人工复核”；建议不直接 kill |
| `idle_in_transaction` | 会话 `idle in transaction` 且事务时长 >0 | confirmed | 只确认分类；应用为何未结束事务仍未定，追加 `application_logs` 缺失 |
| `active_long_query` | 会话 `active` 且事务时长 >0 | suspected | 慢因需结合 Query ID/执行计划/等待事件，不臆造 |
| `undetermined` | 无现场、采集失败或窗口内无可分类会话 | undetermined | 列出缺失证据与三类假设；不能反推“当时不存在长事务” |

`Client/ClientRead` 等待在事实中固定附注“数据库等待客户端下一命令，不是锁等待”。模型输出中的 diagnosis 字段会被忽略（解析时重置为 nil 后由服务端重建）；回答重放保留原 diagnosis；诊断 JSON 序列化保证 facts/hypotheses/missing_evidence/evidence_ids 为空数组而非 null。

### 12.4 前端展示与差距

- AnswerCard 渲染诊断结论（状态 Tag + statement + 证据 ID）、已确认事实（statement + evidence_id/pointer）、待验证假设（statement + 缺少证据），并把 `missing_evidence` 去重并入不确定性提示；复制全文 Markdown 包含诊断结构。`AnswerCard.spec.ts` 覆盖 diagnosis 各区块渲染。
- 告警页 AI 分析弹窗只展示风险、摘要、建议和引用，**未展示 `diagnosis`**；完整诊断卡片只在 AI 助手回答中，不能把弹窗当作已覆盖 RDS 结构化诊断验收（`internal/alert/README.md` 已登记）。
- 未实现告警触发后的平台主动采集：诊断读取的是华为已采样的历史窗口，不是平台在告警时刻抓取的快照。

### 12.5 验证覆盖

- application：`postgres_chat_test.go`（双入口 diagnosis、工具拒绝不取数、失败产生缺失证据）与 `postgres_evidence_finalizer_test.go`（排序落库、fail-closed、截断计数、四类判定、idle confirmed 边界、无阻塞者不声称、重放保留、模型 diagnosis 忽略、JSON 空数组）。
- infrastructure：`postgres_collector_test.go`（分页+脱敏、部分证据保留、会话权限拒绝、非法窗口、total unknown、ctx 取消、SQL 摘要脱敏与 300 runes）；`rds_history_client_test.go`（total unknown、过期 ctx）。
- E2E：`scripts/e2e-postgres-history.ps1 -AlertId <长事务告警>` 需真实云账号与支持历史会话的实例，使用 mock 模型；验证工具 ok、diagnosis 结构与 idle confirmed 边界；权限不足显式失败不伪造成功。finally 只清理 mock provider，**会话与证据保留**；9.17.2 的 IAM 权限缺口组合需人工验证。

## 13. ai-orchestrator 与类型 c 参考协议

`cmd/ai-orchestrator` 是类型 c 的内部编排参考实现，**不是必需组件**：主对话只用类型 b，告警分析按 `AnalysisProviderID` 优先 b → c → 其他启用项选择，只有环境里没有任何类型 b 时类型 c 才会接管分析（对话仍返回 degraded/model_unavailable）。

### 13.1 平台侧调用（toolgateway）

- 类型 c 注册为 `InternalServiceExecutor`：`POST <base_url>/api/tool/invoke`，鉴权头 `X-API-Key: Bearer <api_key>`；base_url/api_key 必填，响应体上限 1MiB，非 2xx 统一 `UNAVAILABLE`。
- 出站目标经平台侧 `ValidateOutboundURL` / `ai.outbound_allowed_hosts` 校验；SSRF 防护在平台侧，编排服务自身不内置。
- 两条调用链可达：直接 `POST /api/ai/tools/invoke`（转发任意 tool_code）与告警分析（固定 `alarm.analyze`，payload 含 `alert`/`evidence`，长事务场景另含 `postgres_activity`、`similar_cases`）。对话内置工具不经 Provider 转发。

### 13.2 服务侧行为（cmd/ai-orchestrator）

```text
配置（全部环境变量，缺一启动失败）：
  AIOPS_AI_ORCH_ADDR（默认 :8090）、AIOPS_AI_ORCH_API_KEY（必填）
  AIOPS_AI_ORCH_MODEL_BASE_URL / MODEL_API_KEY / MODEL_NAME（必填）
  AIOPS_AI_ORCH_MODEL_PATH（默认 /v1/chat/completions）、AIOPS_AI_ORCH_TIMEOUT_S（默认 30）

POST /api/tool/invoke（兼容 /v1/tools/invoke）
  -> X-API-Key 校验（剥离 Bearer 前缀后常数时间比较；兼容裸 key）
  -> 仅接受 tool_code=alarm.analyze；请求体上限 2MiB
  -> 固定 system prompt + {alert, evidence, task} user prompt
     （temperature 0.2、response_format=json_object；模型响应上限 4MiB）
  -> 429/5xx/网络错误按预算重试 2 次；解析失败要求 summary 非空
  -> references 与 payload.evidence 中的 evidence_id 合并去重
  -> 任何模型/解析失败：HTTP 200 返回保守降级结果
GET /healthz：仅进程存活
```

关键边界：

- 服务**不落库、不执行动作**；日志只记 `tool_code`/`duration_ms`/`tokens`/`degraded`，不落告警内容、payload 或模型密钥。
- 只读 `payload.alert` 与 `payload.evidence` 构建 Prompt，**不消费 `postgres_activity` 与 `similar_cases`**；长事务结构化 `diagnosis` 由平台侧 `buildPostgresDiagnosis` 生成（与 provider 类型无关），并在长事务场景覆盖 summary/recommendations。
- 模型失败时的降级输出是普通分析对象（固定 summary、`risk_level=info`、保留 evidence 引用、uncertainty 注明不可用），**响应体没有 `degraded=true` 字段**——平台不会把该轮分析记为 `provider_degraded` 或审计 `partial`，与类型 b 的降级语义不同，只能靠回答文案与编排服务日志区分。
- HTTP server 超时：ReadHeader 5s / Read 30s / Write 90s，优雅关机 10s；日志固定 json/stdout、服务名 `aiops-ai-orchestrator`。

### 13.3 部署与验证边界

- 仓库不含编排服务的 compose/K8s 服务定义，Makefile `build` 只构建 `cmd/api`；需要时用 `go build ./cmd/ai-orchestrator` 手工构建并内网部署。
- 前端 ProviderManager 类型下拉提供「C - 内部服务」；`model` 字段仅类型 b 显示（类型 a/c 的 model 不会被服务端强制清空，但对执行器无意义）。
- 验证：`cmd/ai-orchestrator/analysis_test.go` 7 个用例（成功、坏 key、Bearer 兼容、未知 tool_code、模型失败回退、summary 必填、引用去重）；toolgateway 类型 c 仅以 stub/协议字段覆盖。**无编排服务专项 E2E**，契约 §9.4 的部署建议需人工验收。
