# AI 证据驱动诊断设计

> 状态：P0-1（0060）与 P1-1（0061）已实施；P0-2/P0-3 通用能力和 P1-2 告警触发采集待实施
> 日期：2026-11-20  
> P0-1 实施日期：2026-11-20  
> 首个验证场景：华为云 CES `SYS.RDS / oldest_transaction_duration` 长事务告警  
> 目标读者：Alert、Observability、Asset、AI、Integration、Execution 与前端协作者

## 1. 背景

平台的长期目标不是让 AI 根据告警文本自由推测，而是形成可回放、可核验、可审计的运维诊断链路：

```text
告警/用户问题
  -> 明确分析对象与时间窗口
  -> 采集真实证据
  -> 持久化证据快照
  -> 判断证据覆盖度
  -> 平台生成事实
  -> AI 生成有引用的推断与建议
  -> 服务端校验引用
  -> 页面展示证据链与缺失项
  -> 人工确认结论/处置
  -> 反馈沉淀为案例
```

首个场景来自生产 PostgreSQL 实例的“最长事务存活时长”告警。CES 指标可以证明当时存在长事务，但不能单独证明是哪个会话、哪条 SQL、哪个应用或哪种原因造成。

### 1.1 已具备的基础

- 华为云 CES 告警拉取、标准化、资源匹配和原始字段追溯已落地。
- 告警详情可将 `alert_id`、应用、资源和时间窗口带入 AI 助手，见 [alertAIContext.ts](../web/src/views/alerts/composables/alertAIContext.ts)。
- AI 助手已有告警读取、CES 指标证据、规则查询、批量规则体检（`alert.rules.health`）、账号/资源发现、受限 Agent Loop、权限校验和审计，见 [AI 契约](../ops/ai-contract.md)。
- Observability 已生成 `evidence_id`，并持久化证据引用，见 [evidence_repository.go](../internal/observability/infrastructure/persistence/evidence_repository.go)。
- Asset 已有云资源同步、应用归属和部分 CMDB 拓扑。
- Execution 已具备人工确认边界，AI 不需要也不应获得直接执行能力。

### 1.2 当前差距

1. Observability 第一阶段只保存证据摘要，不保存原始查询结果，见 [repository.go](../internal/observability/domain/repository.go)。仅凭证据 ID 不一定能复查模型当时看到的数据点和记录。
2. 当前回答以 `evidence[]` 和整篇 `references[]` 为主，尚未把每个事实、推断与具体证据逐项绑定。
3. 当前解析主要校验回答结构，不能阻止模型引用不存在的证据，也不能把无证据内容自动降级为假设。
4. PostgreSQL 现场已支持 RDS 历史会话采样，但尚不包含应用日志中的提交/回滚路径，不能仅凭会话确认应用代码级最终原因。
5. 真实日志、链路和变更工具仍未形成统一证据链；现有 fake Provider 不能作为生产诊断事实来源。
6. 现有 Prompt 已要求“不臆造”，但 Prompt 只是软约束，不能替代确定性的数据和服务端校验，见 [analysis_prompt.go](../internal/ai/application/analysis_prompt.go)。

## 2. 目标与非目标

### 2.1 总目标

所有展示给用户的诊断信息必须明确属于以下四类之一：

| 类型 | 定义 | 证据要求 |
| --- | --- | --- |
| 事实 Fact | 平台从真实数据直接计算或渲染的描述 | 必须引用证据及具体数据位置 |
| 推断 Inference | 基于一个或多个事实形成的分析判断 | 必须引用支持证据，并标明置信度 |
| 假设 Hypothesis | 尚未被当前证据证明的候选原因 | 必须标明未证实及验证所需证据 |
| 建议 Recommendation | 下一步排查或处置建议 | 必须标明依据、知识来源或“通用实践”，不得冒充已验证结论 |

系统必须允许“不知道”和“证据不足”。证据不足时输出缺失项和补采方式，不使用确定语气补全故事。

### 2.2 P0 目标

- 保存实际参与分析的脱敏证据快照，而不只是查询摘要。
- 建立统一证据模型、证据查看入口和告警/会话/消息关联。
- 事实由平台按结构化数据确定性生成，模型不负责生产事实。
- AI 推断逐条引用证据，服务端校验证据 ID、权限、状态和数据位置。
- 明确展示证据覆盖度、缺失证据、采集失败和降级原因。
- 留存模型、Prompt 版本、输入证据、输出和校验结果，支持完整回放。

### 2.3 P1 目标

只针对 PostgreSQL 长事务告警补齐：

- 告警发生时的只读现场快照。
- 长事务会话、等待事件、脱敏 SQL 标识和阻塞关系。
- 基于证据区分“空闲事务”“活跃长 SQL”“锁等待”和“无法判断”。
- 恢复后仍能查看当时现场，不依赖事后实例还保留相同状态。

### 2.4 非目标

P0/P1 不做：

- 一次性接完所有云厂商、日志、链路和 CMDB 类型。
- 让模型执行自由 SQL、自由命令或直接连接生产系统。
- 自动 `pg_cancel_backend`、`pg_terminate_backend`、kill 会话或修改参数。
- 用拓扑、相似案例或知识库内容冒充本次故障事实。
- 以“引用了真实证据”为由宣称自然语言已经通过完全的语义证明。
- 建设独立第二套执行、权限、审计或迁移体系。

## 3. 核心设计约束

1. **AI 不生产事实。** 告警值、时间、指标点、会话状态、锁关系等事实由平台从证据生成。
2. **模型可见即可复查。** 实际送入模型的每份证据都应有持久化快照、内容哈希和版本。
3. **先落证据，再调用模型。** 取数成功但未成功持久化时，不把它作为可核验事实交给最终回答。
4. **引用必须属于本次运行。** 模型不能引用任意历史或他人证据 ID。
5. **无证据不定根因。** 候选原因进入假设区，并给出缺失证据。
6. **采集失败也是证据链的一部分。** 记录状态、原因、时间和 `trace_id`，不能把失败改写成“无异常”。
7. **历史案例只提供排查方向。** 当前结论必须重新取得当前对象和时间窗口的证据。
8. **事实与建议分离。** 通用经验可以说“建议检查”，不能说“本次就是该原因”。
9. **权限沿用现有体系。** 新增证据查看和采集不能绕过 RBAC、数据范围、工具权限与审计。
10. **AI 只读。** 所有真实变更继续进入 Execution 状态机和人工确认。
11. **周期性证据数据化。** 规则体检与指标证据必须提供本地时段画像（`breach_by_hour` / `breach_days` / 本地峰值时刻）；领域成因与处置清单不得固化在系统提示词中，由模型基于证据形成假设并通过只读工具（如 RDS 备份策略/最近备份记录）验证；未取得证据的假设列入 `uncertainties`，阈值调整不能作为唯一建议。

## 4. 总体链路

```text
Alert / Chat
   |
   v
Diagnosis Context
(alert_id, resource, application, event window, trace_id)
   |
   v
Evidence Plan（按告警类型选择采集模板）
   |
   +--> 告警快照 / 规则快照
   +--> CES 指标与原始数据点
   +--> CMDB 资源及关系
   +--> PostgreSQL 现场（P1）
   +--> 日志 / 变更 / 链路（P2）
   |
   v
Evidence Store（脱敏、限额、快照、hash、关联）
   |
   v
Coverage Evaluator
(required / optional / missing / failed / stale)
   |
   +--> Fact Builder（平台确定性生成）
   |
   v
AI Inference（只允许使用已落库证据）
   |
   v
Answer Validator（结构、引用、权限、指针、确认边界）
   |
   v
Answer + Evidence Drawer + Missing Evidence + Audit
```

任何一个采集器不可用都不得阻断已有证据展示；但必须降低覆盖度，并限制最终结论强度。

## 5. P0：证据链底座

### 5.1 统一证据逻辑模型

优先扩展现有 `obs_evidence_ref`，不新建并行的 `ai_evidence` 证据账本。字段为逻辑设计，具体迁移在实施前评审：

| 字段 | 说明 |
| --- | --- |
| `evidence_id` | 对外业务 ID，沿用 `ev-*` |
| `trace_id` | 触发采集的请求/任务链路 |
| `account_id` | 集成账号或平台内部来源标识 |
| `evidence_type` | alert / rule / metrics / resource / postgres_activity / postgres_lock 等 |
| `provider` / `collector` | 真实来源与采集器版本 |
| `query_type` / `query_hash` | 沿用现有查询类型和规范化查询哈希 |
| `query_params` | 脱敏后的完整查询参数，不只保留哈希 |
| `resource_id` / `cloud_resource_id` | 可唯一确定的资源标识；无法唯一解析时保持为空并记录原因 |
| `window_start` / `window_end` | 证据描述的业务时间窗口 |
| `collected_at` | 实际采集时间，不能与分析请求时间混用 |
| `status` / `reason` | ok / skipped / denied / unsupported / error 等 |
| `summary` | 用于列表和检索的脱敏摘要 |
| `payload` | 实际参与分析的脱敏数据快照 |
| `payload_hash` | 规范化 payload 的 SHA-256，防止内容被静默替换 |
| `schema_version` | payload 结构版本 |
| `truncated` / `original_count` | 截断标记与原始数量，防止把样本当全集 |
| `created_at` / `updated_at` | 按项目数据库规范维护 |

证据关联通过独立关系表达，避免在证据表中不断增加可空业务列。**P0-1 仅落地 `obs_evidence_ref` 快照字段，尚未创建关系表；alert / session / message / analysis_run 关联随 P0-2/P0-3 实施。**

```text
evidence_id -> subject_type + subject_id + relation
```

首版主体包括 `alert`、`ai_session`、`ai_message`、`analysis_run`、`asset_resource`。新表沿用项目 BIGSERIAL、业务 ID、时间戳和应用层完整性约束，不引入数据库外键。

### 5.2 快照内容要求

- **指标**：保存本次实际用于分析的序列、数据点、单位、维度、聚合方式、窗口和 period。若降采样，统计口径和降采样前后数量必须可见。
- **告警**：保存本次分析使用的标准化快照及原始载荷引用；告警后续状态变化不能改写历史分析快照。
- **规则**：保存本次使用的阈值、操作符、窗口和规则版本/采集时间。
- **资源/拓扑**：保存已验证的资源标识、应用归属和关系来源；未匹配、多候选和陈旧数据必须明确标记。
- **PostgreSQL**：保存固定 SQL 模板返回的脱敏行、采集时间、实例和数据库名，不保存连接密码。
- **日志/链路（P2）**：保存实际送入模型的入选记录及其上游时间戳、查询条件和截断信息，不默认保存整段原始日志流。

payload 必须限额。超限时先按固定规则裁剪并记录 `truncated=true`，再把同一份裁剪结果同时用于持久化和模型输入，禁止“模型看一套、落盘另一套”。

### 5.3 事实生成与回答结构

事实由平台 Fact Builder 根据证据类型和固定模板生成，例如：

```text
事实：2026-11-20 19:23:03（GMT+8），最长事务存活时长为 659877ms，
超过规则阈值 600000ms。
证据：ev-ces-xxx，pointer=/metrics/0/last
```

模型输出只负责推断、假设、建议、不确定性和它引用的证据。服务端组装最终回答：

```json
{
  "status": "insufficient_context",
  "facts": [
    {
      "statement": "最长事务存活时长超过 600000ms",
      "evidence_ids": ["ev-ces-xxx"],
      "evidence_pointers": ["/metrics/0/last"]
    }
  ],
  "inferences": [],
  "hypotheses": [
    {
      "code": "idle_in_transaction",
      "statement": "可能存在空闲事务，尚未证实",
      "missing_evidence": ["postgres_activity"]
    }
  ],
  "recommendations": [
    {
      "statement": "采集告警发生时的长事务会话和阻塞关系",
      "source_type": "runbook",
      "source_id": "postgres-long-transaction-v1"
    }
  ],
  "root_cause": {
    "status": "undetermined",
    "evidence_ids": []
  },
  "missing_evidence": ["postgres_activity", "postgres_lock"]
}
```

`root_cause.status` 至少区分 `undetermined / suspected / confirmed`。AI 不能自行把原因置为 `confirmed`；只有满足平台明确诊断规则或经人工确认后才能确认。平台规则确认必须保留直接证据，人工确认还必须保留确认人；空闲长事务规则的 confirmed 只确认分类，不代表应用代码级最终根因。

### 5.4 服务端回答校验

模型返回后必须依次执行：

1. JSON 和字段结构校验。
2. 忽略模型自行生成的事实话术，事实只接受 Fact Builder 输出。
3. 证据 ID 必须属于本次运行，当前用户必须仍有权查看。
4. `status=ok` 的证据才能支撑事实或推断；denied/error/skipped 只能进入缺失和降级说明。
5. `evidence_pointer` 必须指向 payload 中真实存在的字段或记录。
6. 推断必须有支持证据；没有证据的候选原因只能进入假设。
7. 建议必须标明证据、Runbook/知识条目或 `general_practice`，不能使用“已验证可修复”等无依据措辞。
8. 涉及变更时必须保持 `require_confirm=true`，动作只能进入 Execution。
9. 未知证据 ID、非法指针或越权引用不得原样展示；按统一策略重试一次或降级，并记录校验失败原因。

需要承认边界：**引用校验能证明证据存在且被引用，不能数学证明每句自然语言完全等同于证据语义。**因此必须使用“平台生成事实 + 结构化诊断码 + 指针 + 保守措辞 + 人工确认”共同降低风险，不能只增加一个引用正则就宣称彻底解决幻觉。

### 5.5 证据覆盖度

按诊断场景定义 Evidence Profile，而不是所有问题都采集全部数据。

`postgres.long_transaction.v1` 建议：

| 证据 | 级别 | 作用 |
| --- | --- | --- |
| 告警与规则快照 | 必需 | 证明对象、阈值和事件时间 |
| CES 原始指标点 | 必需 | 证明指标变化和恢复情况 |
| 唯一资源解析 | 必需 | 防止分析错实例 |
| PostgreSQL 长事务现场 | 根因分类必需 | 区分 idle / active / wait |
| PostgreSQL 锁与阻塞关系 | 锁结论必需 | 证明 blocked / blocking 关系 |
| CPU、内存、连接、IO | 可选 | 判断是否伴随资源瓶颈 |
| 应用日志、发布和配置变更 | 可选，P2 | 关联应用侧触发原因 |

缺少“根因分类必需”证据时仍可以展示告警事实和排查建议，但 `root_cause` 必须为 `undetermined`，页面明确显示缺失项及补采入口。

### 5.6 证据查看与交互

AI 回答页固定分区：

1. 已确认事实。
2. 分析推断与置信度。
3. 待验证假设。
4. 缺失/失败证据。
5. 推荐操作、风险和确认边界。
6. 本次使用的模型、Prompt 版本、分析时间和 `trace_id`。

每个事实和推断显示可点击证据标识。证据抽屉至少展示来源、资源、采集时间、业务窗口、脱敏查询条件、payload、截断状态和哈希。无权限时不返回证据内容，也不能通过错误信息泄露敏感记录。

“补充采集”只允许调用服务端登记的只读采集器，不能将用户输入或模型文本直接转成 SQL/命令。

### 5.7 回放、审计与留存

每次分析至少留存：

- 分析对象、问题、事件窗口和 `trace_id`。
- 输入证据 ID、payload hash 和证据覆盖结果。
- Provider、实际模型、Prompt 版本和采集器版本。
- 模型原始结构化输出（脱敏）、服务端最终回答和引用校验结果。
- 降级、重试、取消、工具拒绝和人工反馈。

不要求在审计表复制完整 Prompt。可通过“版本化 Prompt 模板 + 已保存问题/上下文 + 证据快照”重建输入，审计继续避免记录凭据和敏感原文。

证据需要配置保留期、单条大小和总存储预算。到期清理必须保留审计所需的最小元数据；具体保留天数和容量阈值在实施前确认，不在本设计中写死。**P0-1 已提供 `ai.evidence_payload_max_bytes`（默认 32768 字节），但未交付保留期清理 worker 或总容量治理，不能把该切片视为已完成留存管理。**

## 6. P1：PostgreSQL 长事务现场

### 6.1 接入前置条件

**P1-1 已优先采用华为云现成历史会话 API，而非数据库直连**：`ListHistorySessions`、`ListHistoryWaitEvents`、`ListHistoryTopSqls` 已能提供历史会话状态、事务时间、等待事件、阻塞进程和 Query ID，所需 IAM 动作分别为 `rds:instance:getHistorySession/getWaitEvent/getTopSql`。该路径复用现有 Integration AK/SK，窗口最长 7 天，避免额外数据库账号和网络依赖。官方契约：[历史会话](https://support.huaweicloud.com/api-rds/rds_12_0014.html)、[等待事件](https://support.huaweicloud.com/api-rds/rds_12_0015.html)、[Top SQL](https://support.huaweicloud.com/api-rds/rds_12_0016.html)。

如果现场已消失、实例不支持历史会话或需要更完整的锁对象，才回退到数据库直连。直连需要额外准备：

华为云 AK/SK 只能访问云 API，不能代替数据库登录。需要额外准备：

- 平台或专用只读采集器到 RDS 私网地址和端口的网络连通。
- 目标实例、数据库名、SSL 要求和资源绑定的凭据引用。
- 独立只读数据库账号；按需授予查看其他会话统计信息的最小权限。
- 连接、语句和锁等待超时，固定结果行数和采集总预算。
- 实例/数据库允许采集清单，禁止模型任意指定连接地址。

凭据只通过现有 Integration 凭据引用获取，不进入 Prompt、证据 payload、日志或审计明文。

### 6.2 固定采集模板

只允许服务端版本化模板：

- 长事务：`pid`、数据库、用户名的必要脱敏摘要、`application_name`、`client_addr`、`state`、`backend_type`、`xact_start`、`query_start`、`wait_event_type`、`wait_event`、事务/语句存活时长。
- 锁与阻塞：blocked PID、blocking PID、锁模式和必要对象信息。
- 实例辅助信息：当前采集时间、数据库名、快照是否完整、权限是否足够。

SQL 文本可能包含业务参数。优先使用 `pg_stat_statements.queryid` 或数据库提供的规范化语句标识；无法可靠脱敏时不得把原文送入 Prompt，只保留受控摘要或哈希，并明确这会限制 SQL 级判断。

### 6.3 采集时机与幂等

- **告警触发采集**：告警达到规则条件并进入平台后异步执行，保留告警发生时的现场。
- **AI 查询补采**：用户发起分析时再采一次，作为“当前状态”，不能冒充“告警时状态”。
- **恢复补记**：恢复后只记录指标和状态，不伪造已消失的数据库现场。

采集幂等键至少包含告警业务 ID、告警生命周期序号/发生批次、采集器模板版本和采集时机。重复通知不能无上限重复连接数据库。

采集超时、权限不足、网络失败和实例已恢复都要落状态记录；告警接入主链路不能被慢采集阻塞。

### 6.4 可以下的结论边界

| 现场证据 | 平台可以陈述的事实 | 仍不能直接断言 |
| --- | --- | --- |
| `state=idle in transaction` 且事务年龄超阈值 | 当时存在空闲长事务及对应应用/客户端摘要 | 应用为什么未提交、具体代码缺陷 |
| `state=active`、非锁等待且语句年龄超阈值 | 当时存在活跃长 SQL | SQL 为什么慢、执行计划是否回归 |
| `wait_event_type=Lock` 且有阻塞链 | 会话正在等待锁，以及直接阻塞者 | 阻塞者的根因、是否应终止会话 |
| 只有 CES 指标 | 当时存在最长事务年龄超阈值 | idle、慢 SQL、锁等待中的任何一种 |
| 告警后现场已消失 | 当前未采到相同会话 | 当时不存在长事务或已经“自愈”的根因 |

## 7. 后续演进

### P2：根因关联

- 发布、配置和数据库变更统一为带时间、对象和来源的证据。
- 接入真实华为 AOM/LTS、应用日志和调用链。
- 通过已验证 CMDB 关系扩展分析范围，不按名称、IP 或同 VPC 猜关系。
- 对“时间相近”只描述相关性；没有因果证据不得写成“由发布导致”。

### P3：持续性能分析

- 定期保存 PostgreSQL 会话摘要和 `pg_stat_statements` 趋势。
- 建立指标基线、同比/环比、容量和慢 SQL 变化分析。
- 将人工确认的根因、有效处置和证据链沉淀为案例。
- 案例检索只决定优先排查顺序，不替代当前证据采集。

## 8. 验收标准

### 8.1 P0 验收

1. 每次指标分析可通过证据 ID 查看实际参与分析的脱敏数据点、窗口、维度和查询条件。
2. payload 与发送给模型的同版本内容 hash 一致；截断状态可见。
3. 每个事实均能跳转到具体证据和有效 pointer。
4. 模型引用不存在的证据 ID 时，服务端拒绝该输出或降级，并留下校验审计。
5. 模型把无证据候选原因写成推断时，服务端不接受为事实或已确认根因。
6. 同一证据快照重复分析时，平台生成的事实保持一致，不依赖模型随机表达。
7. 无 PostgreSQL 现场时，长事务告警只能输出“原因未定 + 缺失证据”，不能输出确定根因。
8. 无权限用户不能通过证据 ID、会话分享或前端直连读取证据。
9. Provider 不可用时只展示平台事实和保守降级，不生成新推断。
10. 证据内容、日志、审计和测试 fixture 均不包含密码、Token、AK/SK 或未脱敏敏感 SQL 参数。

### 8.2 P1 验收

1. 构造 idle、active、锁等待和已恢复四类场景，输出与现场证据一致。
2. 告警触发快照在告警恢复后仍可查看，并明确其采集时间。
3. 查询补采不会被展示成告警时快照。
4. 数据库慢响应不会阻塞告警接入；超时、拒绝和权限不足均有明确状态。
5. 重复告警和重复采集满足幂等与频率限制。
6. 系统不存在 AI 直接终止会话、执行自由 SQL 或绕过 Execution 的入口。

### 8.3 最小验证范围

实施时按改动范围执行：

- `go test ./internal/ai/... ./internal/observability/...`
- 涉及 Integration/Asset 时增加对应模块测试。
- 涉及迁移 runner 或 splitter 时执行 `go test ./pkg/database/...`。
- `cd web && npm run build`。
- 扩展 `scripts/e2e-ai-assistant.ps1` 或新增聚焦 E2E，覆盖证据快照、引用拒绝、权限和降级。
- 更新 `docs/acceptance-checklist.md`，保留现有主链路回归。

## 9. 实施切片与契约同步

每个切片独立可验证，不在一个提交里同时完成所有数据源：

1. **P0-1：证据快照模型与持久化（已完成，migration 0060）**：覆盖现有 CES 指标证据；模型输入前统一降采样/裁剪并保存 payload/hash，写失败 fail-closed；E2E 为 `scripts/e2e-ai-evidence.ps1`。本切片不包含关系表、证据查询 API、前端抽屉或保留期清理。
2. **P0-2：证据查询接口与证据抽屉（待实施）**，打通权限和原始数据复查。
3. **P0-3：Fact Builder、回答结构、引用校验和回放元数据（长事务专用子集已落地）**：已实现平台确定性事实、`undetermined/suspected/confirmed` 分类、假设和缺失项；空闲长事务 confirmed 仅确认分类，应用原因仍未定；通用 Fact Builder、证据详情、完整回放和人工 confirmed 仍待实施。
4. **P1-1：PostgreSQL 只读适配器与手动补采（已完成，migration 0061）**：复用 RDS 历史会话三接口，支持分页、超时、部分失败、用户/客户端/SQL 脱敏和证据落库；会话及匹配 Top SQL 可输出操作类型和最长 300 字的脱敏摘要，原始 SQL 仍不落库；真实账号验收脚本为 `scripts/e2e-postgres-history.ps1`。
5. **P1-2：告警触发快照、覆盖度 profile（部分完成）**：长事务诊断输出已完成；告警触发后的平台主动采集和完整覆盖度 profile 待实施。
6. **P2+：按诊断价值逐项增加变更、日志、链路和性能基线。**

涉及接口、权限码、迁移、工具清单、状态或健康检查时，必须同步：

- [AI 契约](../ops/ai-contract.md)
- [Alert 契约](../ops/alert-contract.md)
- [Observability 文档](../internal/observability/README.md)
- [Migration 契约](../ops/migration-contract.md)
- [前端 API 说明](../web/src/api/README.md)
- [验收清单](../docs/acceptance-checklist.md)

文档与实现冲突时先停下评审，明确是调整设计、修改实现还是拆分阶段，不在编码中静默改变证据语义。

## 10. 实施前需要确认的配置项

以下不阻塞 P0 设计评审，但对应切片开始前必须明确：

- 证据保留期和存储预算（待确认）；单条 payload 上限已由 P0-1 确定为可配置 `ai.evidence_payload_max_bytes`，默认 32768 字节。
- 证据详情接口与前端展示所需的权限码。
- SQL 文本脱敏策略已确定为默认不保存原文，只保留 Query ID、规范化空白后的 SQL hash、操作类型及最长 300 字的脱敏摘要；摘要中的字符串/数字字面量、注释和 dollar-quoted 内容必须移除；用户只保留 hash，客户端只保留网段或地址摘要。
- PostgreSQL 采集已优先复用 RDS 历史会话 API，需要 `rds:instance:getHistorySession/getWaitEvent/getTopSql`；数据库直连的网络位置和只读账号仅作为后续补充方案。
- 告警自动采集的频率、并发、超时和重试上限。
- 除空闲长事务分类外，哪些诊断规则允许平台标记 `confirmed`，哪些必须人工确认。

这些配置用于收紧安全与容量边界，不允许通过配置重新打开自由 SQL、绕过数据权限或 AI 直接执行能力。
