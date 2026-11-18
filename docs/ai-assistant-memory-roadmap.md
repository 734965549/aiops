# AI 助手记忆与自我优化演进设计

> 状态：**三个阶段均已落地**：阶段 1（migration 0051，契约 §10.14）、阶段 2（migration 0052，契约 §10.15）、阶段 3（无新迁移，契约 §10.16）。2026-11 规则体检场景重启了只读工具扩展的第一阶段（`alert.rules.list` + 自由参数 `metrics.query`，migration 0055，契约 §10.12.1）。本文档保留原始设计与演进记录。 AI 运维助手从“固定管线 + 单轮编排”演进为“带记忆、可自我优化、受限 Agent Loop”的设计意图与实施路径。实现前不得把本文内容当作已交付能力写进 `ops/ai-contract.md` 的接口契约。

## 1. 背景与目标

当前 AI 运维助手（migration 0049）的编排是固定单轮管线：

```text
用户消息 + 页面上下文
  -> 只读工具权限校验（alarm.analyze / metrics.query）
  -> 一次性取数（告警快照 + 指标证据）
  -> 类型 b 模型单次调用
  -> 服务端组装结构化 answer
  -> 会话落库 + 审计
```

该管线安全可控，但存在三个能力缺口：

1. **无记忆**：每条告警分析都是“从零开始”，历史上已确认有效的同类案例分析不会参与后续回答。
2. **无自我优化闭环**：`ai_session_message.feedback`（valid/invalid）和审计质量字段（`risk_level`、`degraded_reason`）已经在采集，但只是存储，没有任何消费方；助手不会因为反馈而变好。
3. **单轮编排**：模型一次拿不到更多证据时只能 `insufficient_context`，不能像 Agent 产品那样在回合内多轮自查、自我修正。

目标形态（对标 Hermes 类 Agent 产品的能力，但保留平台治理边界）：

```text
用户消息 + 页面上下文
  -> 相似案例记忆检索（历史有效案例、沉淀知识）
  -> 受限多轮只读工具循环（每轮过 toolgateway 权限）
  -> 类型 b 模型迭代推理
  -> 服务端组装 answer + 案例引用
  -> 反馈回流 -> 知识沉淀 -> 影响后续检索排序
```

## 2. 不变量（安全边界）

以下边界在整个演进过程中不放松，任何阶段实现都必须遵守：

- AI 只分析、解释、建议、生成计划；真实动作必须进入 Execution 状态机并人工确认。
- 工具调用继承当前用户身份，每轮都必须过 RBAC + 工具权限校验；不允许“系统超级权限”绕过。
- `execution.tasks.dispatch` / `execution.command.run` 保持不可达；Agent Loop 的唯一执行出口仍是 `execution.propose` action。
- 凭据、`api_key`、AK/SK、Token 不进入 Prompt、记忆、日志或审计明文；记忆条目只存脱敏摘要与业务 ID。
- 跨用户记忆隔离：记忆检索按会话归属与数据范围过滤，不把 A 用户案例原文喂给 B 用户。
- 记忆/检索/反馈任何一环失败都不得阻断回答主链路；全部按 degraded/降级处理并写审计。
- 模型输出继续不采信：`actions` 仍由服务端按权限生成，记忆只影响证据与建议质量，不改变权限判断。

## 3. 当前能力与目标能力边界

| 能力 | 当前状态 | 目标状态 |
| --- | --- | --- |
| 会话历史 | `ai_session_message` 全量落库；送模型仅最近 12 条（`chatHistoryLimit`） | 保留现有窗口；新增跨会话相似案例检索注入 |
| 反馈 | `feedback` 字段（valid/invalid）+ `chat_feedback` 审计，无消费方 | 反馈聚合驱动记忆条目质量分与检索排序 |
| 知识沉淀 | 无 | 反馈与审计回流为可审核的知识条目，注入 Prompt |
| 工具循环 | 单轮固定取数 | 受限 Agent Loop：模型可申请追加只读工具，回合内多轮执行 |
| 案例引用 | `references` 仅告警/证据 | 新增 `memory_case` 类型引用，可回溯历史案例 |
| 向量检索 | 无（架构设计中 pgvector 为规划态） | 阶段 1 纯 SQL 相似策略；阶段 2 可选 pgvector，失败回落关键词 |

## 4. 阶段 1：相似案例记忆（RAG-lite）（✅ 已落地）

### 4.1 数据来源

不新建语料管线，直接复用已有数据：

| 来源 | 内容 | 用途 |
| --- | --- | --- |
| `ai_session_message`（`feedback=valid`） | 结构化 answer JSON + 会话上下文 | 正样本案例 |
| `ai_session.context_json` | `alert_id`、`application_id`、`resource_id`、`resource_type`、`page` | 案例匹配键 |
| `chat_message` 审计 | `risk_level`、`tool_codes`、`degraded_reason`、`status` | 质量过滤（degraded 不入记忆） |
| 告警分析审计（analyze-alert） | 分析摘要、evidence 引用 | 补充告警侧案例 |

### 4.2 记忆条目表

迁移 `0051_ai_memory_case`，表前缀 `ai_*`：

```sql
CREATE TABLE IF NOT EXISTS ai_memory_case (
  id BIGSERIAL PRIMARY KEY,
  case_id TEXT NOT NULL,              -- 业务 ID，对外使用
  source_type TEXT NOT NULL,          -- chat_message / alert_analysis / knowledge_item
  source_message_id TEXT,             -- 关联 ai_session_message.message_id
  session_id TEXT,
  user_id TEXT NOT NULL,              -- 归属，用于数据范围过滤
  application_id TEXT,
  resource_id TEXT,
  resource_type TEXT,
  alert_signature TEXT,               -- 告警指纹：alert_name + metric + 维度 hash
  keywords TEXT NOT NULL,             -- 检索关键词（提取自 summary/recommendations）
  summary TEXT NOT NULL,              -- 脱敏案例摘要
  recommendations TEXT,               -- 有效建议摘要（JSON 数组文本）
  risk_level TEXT,
  quality_score INT NOT NULL DEFAULT 0,   -- 反馈聚合分
  status TEXT NOT NULL,               -- active / retired
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ai_memory_case_case_id ON ai_memory_case(case_id);
CREATE INDEX IF NOT EXISTS idx_ai_memory_case_signature ON ai_memory_case(alert_signature);
CREATE INDEX IF NOT EXISTS idx_ai_memory_case_keywords ON ai_memory_case USING gin (to_tsvector('simple', keywords));
```

遵守项目硬约束：业务 ID 独立列、时间戳由 Go 维护、不加外键、种子无。`down.sql` 为人工回滚参考。

### 4.3 写入路径

- **即时写入**：`SendMessage` 成功且模型回答 `status=ok` 时，先不入库；等用户反馈 `valid` 后，由反馈处理逻辑异步创建/更新记忆条目（避免无效回答污染记忆）。`invalid` 反馈对已有条目做 `quality_score` 负向累加，低于阈值转 `retired`。
- **回填任务（可选 Worker）**：离线扫描历史 `feedback=valid` 且 `status=ok` 的消息批量生成条目；重复执行幂等（按 `source_message_id` 唯一判重）。
- 写入失败只记 Warn，不影响反馈接口响应。

### 4.4 读取路径（注入 Prompt）

- `ChatService.SendMessage` 在调用模型前按上下文检索：`alert_signature` 精确匹配优先，其次 `application_id`/`resource_id`，再次关键词全文匹配；最多取 3 条，单条摘要截断（沿用 `truncateRunes` 风格，建议 500 runes）。
- 新增 application 端口 `MemoryCasePort`（`internal/ai/application/memory_port.go`），infrastructure 实现为 `persistence.MemoryCaseRepository`；ChatService 不直接依赖 GORM。
- `ChatPromptInput` 新增 `Memory []ChatMemoryCase`，`buildChatUserPrompt` 在 body 中追加 `"memory_cases"` 字段；`chatSystemPrompt` 增加一句约束：记忆案例仅作参考证据，引用时标注 `case_id`，不得照搬结论。
- answer `references` 新增 `{ "type": "memory_case", "id": "case_id", "label": "历史案例", "url": "" }`；前端 AnswerCard 渲染为“参考了 N 条历史案例”。
- `POST /api/ai/analyze-alert` 同步受益：provider payload 追加 `similar_cases` 字段（与 `evidence` 平级），降级语义不变。

### 4.5 降级语义

| 场景 | 行为 |
| --- | --- |
| 记忆表为空 / 无匹配 | 正常回答，`references` 不含 memory_case |
| 检索超时或出错 | 跳过记忆注入，回答主链路不变；审计记 `memory_status=error` |
| 检索结果跨用户 | 不返回（user 数据范围过滤），日志不输出案例原文 |
| 嵌入模型不可用（阶段 2） | 回落关键词检索，`memory_status=keyword_fallback` |

### 4.6 阶段 1 验收要点

- `valid` 反馈后记忆条目生成；`invalid` 反馈累计降分；`retired` 条目不再被检索。
- 同类告警再次提问时，回答 `references` 含历史案例且 `case_id` 可回溯源消息。
- 无记忆数据 / 检索失败时，回答与现状一致（不降级、不报错）。
- B 用户会话检索不到 A 用户案例。
- 审计 `chat_message` payload 新增 `memory_used`（条数）与 `memory_status`，不含案例原文。

## 5. 阶段 2：反馈闭环与知识沉淀（自我优化）（✅ 已落地）

### 5.1 知识条目表

迁移 `0052_ai_knowledge_item`，表前缀 `ai_*`：

| 字段 | 说明 |
| --- | --- |
| `item_id` | 业务 ID |
| `scope` | 生效范围：`global` / `application` / `resource_type` |
| `scope_ref` | 范围值（application_id 等） |
| `title` / `content` | 知识标题与内容（人工审核后生效） |
| `origin` | 来源：`feedback_aggregate` / `manual` |
| `status` | `draft` / `active` / `disabled` |
| `hit_count` / `useful_count` | 注入次数与有效计数，评估知识质量 |

### 5.2 闭环机制

```text
feedback(valid/invalid) + 审计质量字段
  -> 聚合任务（按 alert_signature / application 分组）
  -> 生成 draft 知识条目（如“payment-service CPU 阈值告警历史上均为发布引起，先查发布记录”）
  -> 管理员在系统管理 / AI 配置审核激活
  -> active 条目按 scope 注入 system prompt 附加段（服务端构建，长度预算固定）
  -> hit_count/useful_count 回流评估，低效条目自动转 disabled
```

约束：

- 知识注入走独立的 system prompt 附加段，不覆盖平台固定 system prompt 的安全约束段落。
- 知识条目内容不包含凭据、告警正文原文（存摘要与业务 ID）；注入前服务端再过一遍脱敏规则。
- 用户负反馈率高的知识条目自动停用，停用写审计。
- 本阶段权限码建议 `app:ai:knowledge:manage`（新增时同步迁移种子 + admin 绑定）。

### 5.3 阶段 2 验收要点

- draft 条目必须人工激活后才影响回答；未激活条目不进 Prompt。
- 回答命中知识时 `references` 含 `knowledge_item` 类型引用，可追溯到条目与审核人。
- disabled 条目立即不再注入（无缓存残留）。
- 注入条数有上限（建议 ≤5 条、总长 ≤3000 runes），超出按 `hit_count` + `updated_at` 截断。

## 6. 阶段 3：受限 Agent Loop（多轮工具循环）（✅ 已落地）

### 6.1 编排形态

把 `SendMessage` 的“一次取数 + 一次模型调用”升级为有界循环：

```text
第 0 轮：服务端常规取数（告警快照 + 指标证据，与现状一致）
  -> 模型输出可能为 answer，或 tool_requests
  -> tool_requests 仅允许申请 read_only 工具（alarm.analyze / metrics.query / 阶段内新增只读工具）
  -> 服务端逐个走 toolgateway 权限校验 + 执行，结果作为 observations 追加
  -> 下一轮模型调用
  -> 循环上限 N 轮（建议 3）或 token/时间预算耗尽
  -> 仍无 answer 时返回 degraded（reason=agent_loop_exhausted）
```

### 6.2 硬约束

| 控制点 | 要求 |
| --- | --- |
| 工具模式 | 仅 `mode=read_only` 工具可被模型申请；`require_confirm` / `deny` 工具不出现在可申请清单 |
| 权限 | 每轮每个工具独立过 RBAC + 工具权限；`allowed=false` 时把 reason 反馈给模型而非中断 |
| 预算 | 轮数 ≤3；累计 token 与耗时预算超限即终止循环，用已有 observations 组装回答 |
| 审计 | 每轮写 `tool_invoke` 审计（与现状同结构），`chat_message` 审计新增 `loop_rounds` 字段 |
| 契约 | `POST .../messages` 仍是同步返回；`tool_calls` 数组自然容纳多轮调用记录；流式/取消见 `ops/ai-contract.md` §10.17 |
| 降级 | 循环异常按现有 `degraded` 语义处理，保留已获取上下文 |

### 6.3 阶段 3 验收要点

- 模型申请未授权工具时，该工具 `allowed=false` 且回答正常返回（拒绝原因可见）。
- 轮数/预算耗尽时返回 `degraded` 或基于已有证据的回答，不悬挂、不超时。
- 循环内不存在任何执行类工具调用；`execution.propose` 仍只出现在服务端生成的 `actions` 中。
- 审计可完整回放每轮工具调用与 token 计数。

### 6.4 工具扩展：规则体检 MVP（2026-11 重启）

- 场景：用户询问「某 namespace 下哪些告警规则阈值设置不合理」，助手需要读取规则阈值并对照最近指标破限点。
- 新增 `alert.rules.list`：按 `namespace` 查询规则（`rule_id` / `name` / `severity` / `enabled` / `metric` / `operator` / `threshold` / `window_seconds` 等），单次最多 50 条；账号级数据范围由 `AccountAccessAuthorizer` 前置校验。huawei `ak_sk` 已接真实 CES V2 `ListAlarmRules`，`auth_type=none` 走 fake，`agency` 返回 unsupported。
- `metrics.query` 改为自由参数：模型可传 `account_id/region/project_id/namespace/metric/dimensions/from/to/period/aggregator`，服务端按告警推导兜底并校验窗口（日聚合最多 30 天、任意情况下最多 1440 点）。
- Loop 白名单扩展为 `alarm.analyze` / `metrics.query` / `alert.rules.list`；轮数、token、单轮申请数与执行类拒绝约束不变。
- 验收：`scripts/e2e-ai-rules.ps1`（脚本化 mock 模型 + fake huawei 账号）覆盖规则查询、自由指标取数、证据引用与审计。

## 7. 数据库与迁移规划

| 阶段 | 迁移 | 内容 |
| --- | --- | --- |
| 阶段 1 | `0051_ai_memory_case` | 记忆条目表 + 索引 |
| 阶段 2 | `0052_ai_knowledge_item` | 知识条目表 + 权限种子 `app:ai:knowledge:manage`（同步 admin 绑定） |
| 阶段 3 | 无新表 | 仅代码编排变更；`ai_session_message.tool_calls_json` 沿用现有结构 |
| 规则体检 MVP | `0055_ai_alert_rules_tool_permission` | 仅种子 `alert.rules.list` AI 工具权限并绑定 admin；无表结构变更 |

- 版本号从当前最新 `0049` 后连续递增，不插队、不改写已发布版本。
- pgvector 为可选增强（阶段 2）：如启用，迁移内 `CREATE EXTENSION IF NOT EXISTS vector` 需 DBA 确认生产实例支持；未启用时关键词检索为默认路径，不得因缺少扩展阻塞迁移。

## 8. 权限、审计与契约影响

| 项 | 变更 | 阶段 |
| --- | --- | --- |
| 权限码 | 复用 `app:ai:chat`；新增 `app:ai:knowledge:manage`（阶段 2）、`app:ai:memory:manage`（记忆管理，migration 0054） | 阶段 2 / 管理 |
| 审计 | `chat_message` 增加 `memory_used` / `memory_status` / `loop_rounds`；知识审核写 `knowledge_review` 审计 | 1–3 |
| `ops/ai-contract.md` | 落地时同步：`references` 新增类型、payload `similar_cases`、错误语义不变 | 落地时 |
| `web/src/api/README.md` | 如新增知识管理 API 同步登记 | 阶段 2 |
| `docs/acceptance-checklist.md` | 新增 9.9 记忆与自我优化验收节 | 落地时 |

## 9. 验证策略

| 范围 | 验证 |
| --- | --- |
| domain | 记忆条目状态机（active/retired）、知识条目状态机（draft/active/disabled）、质量分规则单测 |
| application | 反馈→记忆写入、检索注入、预算控制、降级路径必须有测试；跨用户隔离负向用例 |
| infrastructure | 检索超时/空表/全文索引异常不阻断主链路 |
| interfaces/http | 反馈与知识管理接口的成功/401/403/参数错误；记忆引用回显 |
| E2E | `scripts/e2e-ai-memory.ps1`：valid 反馈 → 生成案例 → 同类告警提问 → 引用回显 → invalid 降分 → retired；阶段 2/3 追加 `e2e-ai-knowledge.ps1`、`e2e-ai-stream.ps1`、`e2e-ai-agent-loop.ps1`（脚本内置 loopback mock 模型，dev 环境确定性触发） |
| 架构检查 | 新文件遵守 800 行上限；`ChatService` 拆分时保持端口接口，避免 application 直接依赖 GORM |

## 10. 实施顺序建议

1. 阶段 1 先做最小闭环：`0051` 建表 + 反馈写记忆 + ChatService 检索注入 + references 回显（约 3–5 天量级，不动模型调用方式）。
2. 阶段 1 验收通过后再评估阶段 2；知识审核 UI 放在「系统管理 / AI 配置」新增页签。
3. 阶段 3 依赖阶段 1 的 `memory_status` 审计字段稳定；Agent Loop 已按「只读工具 + 预算控制」形态落地（契约 §10.16）。2026-11 规则体检 MVP 已重启只读工具扩展的第一阶段：`alert.rules.list` + 自由参数 `metrics.query`（契约 §10.12.1、migration 0055）；日志/资产/变更类工具与 `cloud.*` 仍推迟。

## 11. 文档同步要求

实现任一阶段时同步更新：

- `ops/ai-contract.md`（接口与语义，落地才写）
- `docs/acceptance-checklist.md`（新增验收项）
- `docs/AI运维平台技术架构设计.md` §16.2（标注已落地阶段）
- `web/src/api/README.md`（新增 API）
- 本文档的阶段状态标注（规划态 → 已落地）
