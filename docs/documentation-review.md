# 文档与当前实现复核记录

- 最近复核：2026-11-24
- 代码基线：`dev` 分支 `4b9ad68`
- 复核方式：静态核对代码、路由、前端页面、迁移、测试与 E2E 脚本。本轮未执行单元测试、构建或 E2E，因此“已落地”表示代码路径存在，“有验证覆盖”表示仓库存在对应测试或脚本，均不等于本次运行已通过。

## 1. 复核范围

- 项目入口：`README.md`。
- 后端与共享包：`cmd/`、`internal/`、`pkg/` 及目录级 README。
- 前端：`web/src/api/`、`views/`、`router/`、`stores/`、布局与页面级组件。
- 稳定契约：`ops/*.md`。
- 数据与交付：`migrations/`、`deployments/`、`scripts/`。
- 产品、架构、演示、验收和发布文档：`docs/*.md`。

## 2. 统一状态口径

| 口径 | 含义 |
| --- | --- |
| 已落地 | 对应代码路径、路由或页面存在；完成范围仍需看边界说明 |
| 部分落地 | 主路径存在，但 Provider、自动化、前端或其他子能力尚未完整交付 |
| 规划/未落地 | 只有设计、契约草案或待办，没有可使用的实现路径 |
| 有验证覆盖 | 存在单元测试、前端测试或 E2E 脚本，并注明覆盖范围 |
| 已验证 | 在明确环境和日期实际执行并通过；必须另附执行记录，不能由脚本存在推断 |

fake/mock、真实云账号、可选 worker、后端 API、前端 UI 和独立外部进程必须分别描述，不能用一个“已完成”合并。

## 3. 全局基线与模块修正

### 根 README

- 将单一 `✅` 模块表改为“当前实现、前端入口、验证覆盖、完成边界”矩阵。
- 补充 `cmd/ai-orchestrator` 和 `pkg/auth`，更新 Alert、Asset、Execution、AI、Observability、Inspection 职责。
- 修正 AI Provider 配置：空库时从 YAML 播种，之后以数据库为事实源，不再描述为单纯内存注册表。
- 补充前端实际页面、AI 浮层与内存队列、静态菜单和路由仅校验登录的边界。
- 补齐 AI 证据脚本，以及真实 CES、RDS PostgreSQL 历史会话的外部资源前提。
- 增加 `npm test`，避免只构建而不执行现有 Vitest 覆盖。
- 统一默认管理员语义：`0016` 种子后，`0044` 会锁定仍使用已知默认密码的账号；dev/test bootstrap 与生产管理员初始化分开说明。
- 补齐 Audit、CES 资源同步、CES 告警同步等契约索引。
- 将云接入、观测、巡检和 Agent 的“下一阶段新增”旧表述改为当前能力与未完成边界。

### 后端与契约索引

- `internal/README.md`：补齐全部业务上下文职责、输入输出和运行时形态，明确 Asset goroutine、可选 Alert worker、可选 ai-orchestrator 和外部 Execution Agent。
- `ops/README.md`：收录全部 14 份契约，建立契约到代码、前端、迁移和验证证据的映射，明确 fake 与真实验收、规划与已实现章节的区别。

### Identity 模块（第二轮）

- 重写 `internal/identity/README.md`，移除粤语和历史“已验证”结论，补齐认证、外部身份、授权、权限管理、前端、迁移和测试覆盖。
- 更新 `ops/auth-contract.md`：补全 logout、公开路由、refresh 轮换/降级、限流、OAuth state、access token 边界和认证审计语义。
- 更新 `ops/identity-api-contract.md`：补全权限管理范围、数据范围实际调用边界、`attr_subject`、授权 403 响应和审计失败语义。
- 更新前端 API 与验收说明，明确 E2E 只覆盖用户角色主链路，动态菜单和全部权限写接口不能据此标记通过。
- 同步演示与 Compose 文档中的默认管理员前提，避免把完整迁移误解为 admin/admin123 必然可登录。
- 登记前端 access/refresh token 写入 localStorage 与项目安全约束的冲突，本轮未修改认证实现。

### Alert 模块（第三轮）

- 新增 `internal/alert/README.md`，说明 Webhook、生命周期、状态机、接入源、CES 拉取、前端、审计、迁移和真实验证覆盖。
- 更新 `ops/alert-contract.md`：手动同步改为已实现；补充当前创建/更新源的 Integration 权限、列表/详情数据范围边界、静默无自动到期 worker、实际审计与迁移。
- 将历史验收记录改为当前 E2E 覆盖，明确转派、静默、备注、通用 Webhook 等不在 `e2e-alert.ps1` 中。
- 更新 `ops/huawei-ces-alert-sync-contract.md`，区分已落地代码、fake E2E 和真实云/多实例剩余验收。
- 修正演示中的告警源字段为 `id/secret`，明确响应不回传明文 Token；同步前端 API 与验收清单。

### Asset / CMDB / 资源拓扑（第四轮）

- 新增 `internal/asset/README.md`，覆盖注册表、匹配规则、云同步、拓扑、资源解析、前端、迁移和验证覆盖。
- 修正拓扑前端文档：独立为 `asset-topology.ts`，手工关系允许跨应用；普通拓扑/解析只有静态 RBAC。
- 统一 CES 默认资源组口径：默认候选名未命中可进入全部资源组并集，显式组未命中失败；同步修正云观测契约、ADR、Runbook 与验收清单。
- 重排验收章节为 2.1–2.7，补充 E2E 真实覆盖及前后端 updated_at 排序差异。
- 将 Sync 数据范围、审计生命周期、triggered_by/后台 reaper、云应用命名四项代码差距写入契约、ADR、Runbook、backlog 和本记录，未将其伪装为文档已完成。
- 更新 dimension 支持方案状态，实施前背景与实现后真实账号记录分开表述。

### Runbook 处置预案（第五轮）

- 新增 `internal/runbook/README.md`，说明模板、推荐、Execution 展开、风险、dry-run、前端、审计和验证覆盖。
- 重写 `ops/runbook-contract.md`，区分已落地的模板/推荐/模拟多步任务与未落地的 Command Spec、medium_selector、Runbook→Agent。
- 明确推荐 API 不限制状态，只有创建 Execution 时强制 processing；DELETE 是软停用，更新全量替换步骤，历史任务使用 snapshot。
- 明确 `default_dry_run` 只是元数据，实际 dry-run 由任务请求和步骤支持共同决定。
- 同步前端 API、验收和演示：后端 CRUD 完整，前端仅列表、详情、启停，当前无独立 Runbook 前端 Vitest。

### Execution / Execution Agent（第六轮）

- 新增 `internal/execution/README.md`，说明核心状态机、确认/拒绝、模拟执行、Runbook、Medium/Spec、Agent 协议、前端、审计和验证覆盖。
- 重写 `ops/execution-contract.md`：补齐任务日志、AI/Inspection 来源、confirm=false 拒绝、Agent 同步执行拦截和 adapter 保留边界。
- 重写 `ops/execution-agent-contract.md`：修正 Medium 权限、静态注册令牌、Agent Token、无 mTLS、显式介体、租约响应、日志/结果和实际 E2E 范围。
- 明确 Agent 创建/lease/result 不回写 Alert execution_created/started/finished，不能套用普通模拟任务时间线结论。
- 同步前端 API、验收和演示：Agent 是服务端控制面 + PowerShell fake，不是独立 Agent 产品或完整管理 UI。

### Dashboard 首页驾驶舱（第七轮）

- 新增 `internal/dashboard/README.md` 和 `ops/dashboard-contract.md`，把 Dashboard 从“只有一页聚合摘要”补齐为独立模块与稳定契约。
- 明确聚合字段：active 只排除 closed，recent 最多 10 条，processing_alerts 包含 new/acknowledged/processing 且最多 5 条。
- 明确后端单项失败降级为 0/空且没有 degraded 字段，真实 0 与降级 0 不可区分；前端整体失败显示 `—`，无专门错误 Alert。
- 补充 Runbook 推荐组装、Readiness 独立加载、静态 RBAC 与无数据范围过滤。
- 同步前端 API、验收和演示：当前无专用 Dashboard E2E 或前端 Vitest，Identity E2E 只调用一次摘要。

### Audit 审计中心（第八轮）

- 新增 `internal/audit/README.md`，重写 `ops/audit-contract.md`，补齐写入链路、真实 resource_type/action 目录、精确查询、权限、认证审计和完整性边界。
- 明确前端 Resource/User/Trace 聚合只基于当前页，分类卡与 total 口径不同；CSV 请求 1000 条会被后端截为 100，实际最多导出第一页 100 条。
- 登记资源类型快捷选项与真实写入类型不一致、标准 recorder 不传 IP/UA、服务端无通用 Payload 脱敏、无专用 Audit E2E/前端 Vitest 等差距。
- 明确审计写入不与业务共享统一事务，当前主要调用点为 best-effort；修正 Alert CES“最终状态与审计同事务提交”的旧描述，实际为先提交状态再独立写审计。
- 明确 login/refresh/logout 属于 Identity `iam_auth_audit` 和独立接口，不在 `/api/audits`；无修改/删除 HTTP API 不等于 WORM 或存储级不可篡改。
- 顺带修正 Identity 文档中 Asset Sync 数据范围旧引用，与 Asset 轮次结论对齐；未修改任何业务实现。

### Integration 接入账号（第九轮）

- 重写 `internal/integration/README.md`，移除旧粤语说明，补齐六条路由、五类权限、Provider/auth 矩阵、凭据、事务、extra_config、前端和验证覆盖。
- 更新 `ops/cloud-observability-contract.md` §4/§10/§11：明确列表 keyword、部分更新、软删除清密文、字段级 checker、failed 业务状态、默认 capabilities 和密钥轮换边界。
- 明确当前所有 checker 都不访问真实外部服务，ok/capabilities 不代表网络、云端权限或 Adapter 实测通过；失败结果落库后 HTTP 仍可为 200 + OK。
- 登记 agency 前端缺字段、失败检查仍显示成功 Toast、无关键字/检查历史/凭据清空 UI，以及无多版本解密、外部 Secret Manager 和在线重加密流程。
- 明确凭据省略/空对象保留、切 none 不清密文、删除软账号+硬凭据、审计事务外 best-effort；新华为默认 CES 与 0029 历史账号 native 分开。
- 同步根 README、internal/ops 索引、整体调用关系、前端 API、验收和演示；E2E 仅覆盖 none 主路径，未修改业务实现。

### Observability 与证据快照（第十轮）

- 重写 `internal/observability/README.md`，更新云观测契约 §5/§8/§10，明确四个公共查询路由与内部资源/规则/RDS Port 的区别。
- 按 Adapter 重写真实/fake 矩阵：华为 AK/SK 指标、资源/规则有真实路径，真实 logs/traces/topology unsupported，agency 不降级；SigNoz/Prometheus 当前均为 fake。
- 修正公共指标 project_id 旧示例、拓扑缺失 account_id 和 P99 旧描述；补充真实 CES period/filter 枚举与公共静态 RBAC/无数据范围边界。
- 区分查询 DTO、普通证据摘要与 AI 定稿 payload：普通 Metrics 有查询参数但无 payload；AI 先 collecting，定稿 hash 落库后才进入模型，OK 快照失败 fail-closed。
- 补入 `ai.postgres_activity.v1` 快照、普通证据保存失败与审计 best-effort 的不同语义；当前无公共证据详情 API/UI。
- 明确两个 E2E 都使用 fake/mock：普通查询脚本不是真实云验收，AI 证据脚本验证模型输入与快照 hash，也不是实际 CES 验收。

### Inspection 巡检中心（第十一轮）

- 重写 `internal/inspection/README.md`，更新云观测契约 §6–§8/§10，明确 10 个路由、静态 RBAC、同步手动运行和后端转执行。
- 明确 EvidenceAnalyzer 是固定阈值规则，不是 AI Agent；schedule 无 worker，cancelled 无调用点，notification 和 accepted/dismissed 未落地。
- 登记采集固定最近 1h/cn-north-4/SYS.ECS/空 dimensions，真实华为指标会失败；affected_resources 仍是固定示例，fake E2E 不能代表真实云巡检。
- 明确 Run 通常直接返回终态，全部证据失败也可 HTTP 200 + status=failed；Findings/Recs/终态 Run 同事务，审计 best-effort。
- 明确转执行只允许 agent 且 medium_id 必填，无自动介体；Recommendation 不存 task_id 或完成回写，创建与状态更新非同一事务。
- 修正 E2E 归属：Inspection 脚本只覆盖 fake 主路径，Execution Agent 脚本不经过 Recommendation；前端无转执行入口和 Vitest。

### AI Provider 与工具网关（第十二轮）

- 新增 `internal/ai/README.md`，更新 AI 契约 §3–§8、审计目录、整体调用关系与调用关系图，固定 Provider 生命周期、三类 Executor、出站限制和工具权限边界。
- 明确数据库与当前进程运行时两层事实源，单条加载失败、多实例不广播、upsert 全量覆盖和删除语义；enabled 不是健康状态，保存不做测试连接。
- 修正 api_key 只返回 has_api_key、headers 原样保存/返回、页面编辑清 headers，以及 0048 DB 30000ms 与应用/GORM 300000ms 默认值差异。
- 拆分直接 Provider 转发、告警分析和对话本地工具三条链；对话 8 个内置只读工具补齐 postgres.activity.query，执行类工具不进入 Agent Loop。
- 按代码修正 require_confirm 的 permits_unconfirmed_invoke 豁免、直接调用非 all 数据范围入口 403，以及告警数据范围拒绝 allowed=false。
- 登记 Provider CRUD 无操作审计、直接 tool_invoke 拒绝仍 result=success且字段不足；同步前端、验收、演示和 Inspection 工具种子边界。

### AI 会话与 Agent Loop（第十三轮）

- 扩充 `internal/ai/README.md` 与 AI 契约 §10.1–§10.17，覆盖 10 个会话/消息路由、归属、生命周期、最近 200 条详情和 12 条模型历史。
- 明确用户/助手消息两个事务、取消检查点、client_message_id 重放/冲突/不同内容语义，以及无会话级串行锁；degraded/incomplete 同样重放。
- 对齐模型选择、回退和持久化字段：顶层 provider_id 只在无会话模型时锁定，页面需同时传 context；无默认模型才 model_unavailable。
- 明确 Loop 的 8 个工具、4 轮/80000 token/600 秒/单轮 4 个申请、成功组合去重和强制收敛；强制成功可正常回答，不再误写为必然 degraded。
- 修正 execution.tasks.propose：finalize 的权限校验会进入 tool_calls/审计但不创建任务，不能把它当作 Loop 内可执行工具。
- 登记归档输入清空、生成中模型未禁用、详情无通用刷新、重试只重放且本地可能重复、历史重试不可用和多轮展示差距；同步前端 90 秒与后端 600 秒预算不一致。
- 更新整体与详细调用关系、记忆演进背景、前端 API、验收和演示；三份 AI E2E 的 mock、SKIP、保留数据和部分断言边界已分开。

### AI 记忆与知识沉淀（第十四轮）

- 扩充 `internal/ai/README.md` 与 AI 契约 §10.14/§10.15/§10.19，固定记忆沉淀/降分/检索、知识审核/注入/计数、反馈聚合和管理接口。
- 明确当前唯一记忆来源是 chat 回答 valid；alert_analysis/knowledge_item 只是枚举，没有写入方。降级回答不入记忆，invalid 对不存在案例 no-op，自动反馈变化不写独立 memory 审计。
- 对齐 alert_signature 维度键、keywords、match_rank、80 字引用 label、3 条注入上限和 user/active 隔离。
- 明确知识 active/draft/disabled、5 条/3000 runes 注入、hit/useful、invalid 只评估、auto_disable actor 为空；编辑 active 回 draft 并清空 reviewer但保留计数。
- 修正聚合语义：min_cases 是阈值而非入参上限，聚合取数固定 20 条；大阈值通常 insufficient_cases。生成 ID 为 agg-<application_id>，active 不被覆盖。
- 登记 MemoryManager/KnowledgeManager 当前筛选、分页、展示、二次确认和聚合入参差距，以及 memory/knowledge E2E 的部分覆盖、SKIP、写数据和清理边界。
- 同步根 README、internal/ops 索引、前端 API、验收 9.9/9.10 和演示 8F；未修改业务代码。

## 4. 已确认的实现边界

这些边界应在对应模块文档中保留，不能在后续整理时被简化成“全部完成”。

| 模块 | 当前实现 | 尚未完整交付或需单独验收 |
| --- | --- | --- |
| Identity | 本地/LDAP/OAuth、JWT、RBAC、数据范围、AI 工具权限及管理接口 | 动态菜单、MFA；数据范围按调用点生效；E2E 不覆盖全部权限写接口；前端 Token 持久化与安全约束冲突 |
| Alert | Webhook、状态流转、CES 手动拉取 | 创建/更新所有源均需 Integration 读权限；列表/详情无自动数据范围过滤；静默无到期自动恢复；worker 默认关闭；真实账号验收与 fake E2E 分开 |
| Asset | 注册表、匹配规则、异步同步、批次、CMDB 拓扑和关系 | Sync 数据范围、审计生命周期、triggered_by/后台 reaper、云应用命名有差距；真实云对账与增强缺口单独验收 |
| Runbook | 模板、推荐、模拟路径结构化多步任务 | 前端仅列表/详情/启停；Command Spec 与 Runbook→Agent 未落地；default_dry_run 不单独触发模拟 |
| Execution | 核心状态机、模拟执行、Agent 服务端控制面 | 独立 Agent/管理 UI、自动选择、mTLS、Runbook→Agent 未落地；Agent Alert 时间线未闭环；前端无拒绝和日志流 |
| Dashboard | 聚合摘要 API 与页面 | 无专用 Dashboard E2E/前端 Vitest；降级 0 与真实 0 不可区分；无数据范围过滤 |
| Audit | 操作审计写入与精确分页检索，前端详情、当前页聚合和 CSV | CSV 实际最多 100 条；资源类型快捷选项未对齐；标准 recorder 无 IP/UA；写入多为 best-effort；认证审计独立；无专用 E2E/前端 Vitest，无存储级防篡改/保留机制 |
| AI | Provider、直接工具转发、8 个对话只读工具、会话、幂等、SSE、工具循环、记忆/知识、分享、多模态、执行提议、证据诊断 | Provider 无健康/测试连接且 CRUD 无操作审计；页面编辑清 headers；直接调用审计拒绝语义不完整；会话详情最近 200 条，重试只重放，归档输入/模型禁用/详情刷新有缺口；记忆/知识管理筛选展示有限且 E2E 覆盖不全；前端消息队列不持久化；分享 token/审计/404 语义已按代码核对，分享页不渲染附图，图片链路无 AI E2E；告警 AI 弹窗诊断存在前端缺口；type=c 仅为参考服务 |
| Integration | 账号、AES-GCM 凭据、默认能力声明、字段级检查、软删除清密文 | checker 不访问外部服务，capabilities 非实测；agency 前端缺字段，failed 仍显示成功 Toast；无完整密钥轮换/外部 Vault；E2E 仅 none 主路径 |
| Observability | 四类统一查询、华为部分真实 Adapter、普通证据与 AI 定稿快照 | 真实 logs/traces/topology 不支持；SigNoz/Prometheus 为 fake；公共指标无 project_id；快照无公共详情 API/UI；查询与快照 E2E 均为 fake/mock |
| Inspection | 策略、同步规则运行、Finding、Recommendation、后端转执行 | 非 AI Agent；定时 worker/取消/通知未落地；真实 scope 采集受限；转执行无前端、自动介体、跨模块事务或完成回写；E2E 仅 fake 主路径 |

## 5. 后续模块文档待处理项

### 5.1 契约和目录 README

- `ops/ai-contract.md`：AI 契约各轮次（Provider/工具网关、会话/Agent Loop、记忆/知识、分享/多模态、浮层/消息排队、RDS 诊断、ai-orchestrator）已按代码修正完毕；后续如改动 AI 行为需按相同口径回补。
- `cmd/api/README.md`：装配说明仍偏 Identity/AI，需对齐当前 11 个业务上下文及后台任务。
- `web/src/views/README.md`、`web/src/stores/README.md`、`web/src/router/README.md`：仍停留在早期概念说明，需补实际页面、aiContext、共享会话和权限边界。
- `web/src/api/README.md`：拓扑函数归属和跨应用关系已修正；Inspection `getPolicy` 已核对为后端与封装均存在、页面未使用，其余模块继续按轮次复核。

### 5.2 部署、演示和发布

- `deployments/kubernetes.md`：手工 SQL/手工账本与自研 runner 冲突；迁移清单停在 `0017`；关于镜像不含迁移二进制的描述已过时。
- Kubernetes 示例还需核对生产必需的 Execution Agent 注册令牌、SSE/Ingress 超时和安全配置。
- `docs/release-checklist.md`：存在旧迁移号示例、旧前端静态目录、E2E 覆盖偏旧及 `go test` 与 race 表述不一致等问题。
- `docs/demo-flow.md`：仍有旧迁移基线和“全部 E2E”范围不清等问题；Alert 告警源字段与 secret 语义已随本轮修正。
- `deployments/README.demo.md`：历史验证结果需按当前迁移基线重述；`deployments/README.md` 的默认管理员前提已随 Identity 轮次修正。
- `scripts/` 没有统一索引；后续应说明每个脚本的前置条件、外部资源、写数据/清理行为和适用环境。

### 5.3 不能只靠文档关闭的实现差距

- 分享会话页未复用普通用户消息的图片渲染：接口已返回 `images`，仅 `SharedSession.vue` 缺渲染，与多模态分享验收项（9.16.6）不完全一致。图片链路无任何 AI E2E，ShareDialog/SharedSession/useChat 图片路径无专项前端测试。
- 后端告警分析已有 `diagnosis`，但告警 AI 弹窗类型和展示尚未完整接入；AI 助手回答卡片已有诊断展示。
- Execution Agent 独立进程、管理 UI、自动介体选择、mTLS 和 Runbook→Agent 未实现；Agent 创建/lease/result 不写 Alert 时间线。Inspection 建议转执行入口、动态菜单仍未实现。
- 前端 `stores/auth.ts` 将 access/refresh token 写入 localStorage，与项目“不将 Token 写入前端持久化状态”的安全约束冲突，需独立安全整改。
- Asset Sync 未按早期契约接入 owner_team/region 数据范围，批次列表和详情也未完整隔离；AI 资源读取的独立校验不能替代该缺口。
- Asset Sync 审计仍统一 `action=sync`，缺少独立生命周期 action 和完整 summary/scopes 载荷；`triggered_by` 未写入，周期后台 reaper 未装配。
- `ensureCloudApplication` 新建应用名仍为 `<provider>-cloud`，历史迁移归一化不能保证后续新建名称符合目标。
- Runbook 步骤尚无 command_spec_id/medium_selector，Agent 模式不会展开 Runbook 模板；不能由 Execution Agent 控制面存在反推该集成已完成。
- Audit 前端资源类型快捷选项与真实写入目录不一致，聚合只覆盖当前页，CSV 实际最多 100 条；标准 recorder 不传 IP/UA，写入多为 best-effort，不能靠文档将其升级为全量统计、完整客户端审计或强事务保证。
- Integration checker 当前不访问外部服务，capabilities 是默认声明；前端缺 agency 凭据字段且 failed 检查仍显示成功 Toast。Vault 只有当前密钥/版本，外部 Secret Manager、多版本解密和在线重加密未落地。
- Observability 真实 logs/traces/topology 与 SigNoz/Prometheus 在线 Adapter 未落地；公共指标 Handler 未接收 project_id，证据详情 API/UI 未提供。普通证据与 AI 定稿 payload 不能合并描述，fake/mock E2E 不能替代真实云验收。
- Inspection 定时 worker、取消、通知、真实 scope 资源映射和前端转执行未落地；Recommendation 不存 task_id，任务创建与状态更新非事务，Execution 完成不回写，不能凭后端路由存在认定巡检闭环完成。
- AI Provider 无健康检查/测试连接，数据库与运行时无统一事务或跨实例广播；页面普通编辑清 headers。Provider CRUD 无操作审计，直接工具调用的 allowed=false 仍记 result=success且缺 reason；工具确认豁免、非 all 数据范围入口限制和固定工具目录必须按实际实现展示。
- AI 会话详情仅最近 200 条，用户/助手消息非同一事务且后端无会话级串行锁；前端归档输入清空、生成中模型未禁用、详情无通用刷新，重试复用旧 key 只重放并可能重复显示，历史重试不可用。同步封装 90 秒与默认 600 秒 Loop 预算不一致。
- AI 取数链路三项缺口已于 2026-11-24 修复：资源分组/云产品级规则会在 provider 层展开实例维度并按规则最多 3 个实例抽样（不再只取首实例）；曲线定稿上限提升到 288 点，峰值时刻与破限片段/时长在降采样前按全量序列计算；Agent Loop 预算放宽到 4 轮/80000 token/单轮 4 个申请。小时级 max 聚合仍不能还原原始秒级误报，需模型缩窗复查。
- AI 记忆管理 UI 只传 status/keyword/user_id、limit=100，未使用 application/resource/offset 和分页，详情不展示 session_id；自动反馈增减不写独立 memory 审计。知识 UI 不展示审核/创建元数据，激活/停用无确认，聚合固定 min_cases=3。记忆 E2E 不查管理列表，knowledge E2E 未覆盖 403、编辑回草稿、自动停用和聚合幂等完整断言。
- 这些项目应在文档中标为缺口；是否补代码由后续需求单独确认。

## 6. 逐模块更新队列

每轮只处理一个模块，同时核对后端、前端、契约、权限、审计、迁移和验证证据。

1. 全局基线：根 README、文档复核记录、internal/ops 索引。
2. P0 主链路：Identity（已完成）→ Alert（已完成，CES 审计事务旧描述已修正）→ Asset/CMDB/拓扑（文档已完成，实现差距已登记）→ Runbook（文档已完成，Command Spec 边界已登记）→ Execution/Execution Agent（文档已完成，控制面边界已登记）→ Dashboard（文档已完成，降级边界已登记）→ Audit（文档已完成，聚合/导出与写入边界已登记）。
3. 云与观测：Integration（文档已完成，字段检查/凭据/前端边界已登记）→ Observability/证据快照（文档已完成，真实/fake 与定稿边界已登记）→ Inspection（文档已完成，同步规则/转执行边界已登记）；CES 告警拉取归入 Alert，CES 资源同步归入 Asset。
4. AI：Provider/工具网关（文档已完成）→ 会话/Agent Loop（文档已完成）→ 记忆/知识（文档已完成，反馈/聚合/管理 UI/E2E 覆盖差距已登记）→ 分享/多模态（文档已完成，共享页附图渲染与图片 E2E 缺口已登记）→ 浮层/消息排队（文档已完成，队列快照/不持久化/停止清队与测试差距已登记）→ RDS 诊断（文档已完成，告警弹窗无诊断展示、平台主动采集未实现与真实账号 E2E 边界已登记）→ ai-orchestrator（文档已完成，降级无 degraded 标记、不消费 postgres_activity、无部署模板与 E2E 已登记）。AI 模块队列全部完成。
5. 前端公共能力：API/request/SSE → Router/导航 → Stores/布局与跨页状态。
6. 工程与交付：cmd/bootstrap/server/version/pkg → migrations → Compose/Kubernetes → scripts/E2E → demo/验收/发布清单。

## 7. 全局基线检查记录

- 修改范围：仅 `README.md`、`internal/README.md`、`ops/README.md`、`docs/documentation-review.md`。
- `git diff --check`：通过，无尾随空格或空白错误。
- Markdown 本地链接与代码围栏检查：4 份修改文档均通过，未发现缺失链接目标或未闭合围栏。
- 索引完整性检查：14 份契约均进入根 README 与 ops 索引；19 个默认 E2E 和 2 个带外部前提的可选 E2E 均有入口；`internal` 顶层模块均有说明。
- 本轮为文档修改，未执行 Go 测试、前端构建/Vitest 或业务 E2E；以上结果不构成业务验收通过结论。

## 8. Identity 模块检查记录

- Identity 轮次修改：`internal/identity/README.md`、`ops/auth-contract.md`、`ops/identity-api-contract.md`、`web/src/api/README.md`、`docs/acceptance-checklist.md`、`docs/demo-flow.md`、`deployments/README.md` 及本记录。
- 路由/权限自动比对：当前 37 个公开或授权路由注册（31 个唯一路径）均能在两份 Identity 契约中找到；12 个 Identity 权限码均有契约说明。
- 累计 11 份已修改 Markdown（含全局基线）均通过本地链接与代码围栏检查；`git diff --check` 通过。
- Identity 的单元测试、E2E 覆盖范围来自静态核对，本轮未执行 Go 测试、前端测试或 `e2e-identity-access.ps1`，不记为运行通过。

## 9. Alert 模块检查记录

- Alert 轮次修改：新增 `internal/alert/README.md`，更新 `ops/alert-contract.md`、`ops/huawei-ces-alert-sync-contract.md`、`web/src/api/README.md`、`docs/acceptance-checklist.md`、`docs/demo-flow.md`、`internal/README.md` 及本记录。
- 路由/权限自动比对：19 个 Alert 路由注册（16 个唯一路径）均能在两份 Alert 契约中找到；7 个 Alert 权限码及 `app:integrations:read` 均有说明。
- 累计 14 份已修改 Markdown（含前两轮）均通过尾随空格、代码围栏和本地链接检查；新建 Alert README 也纳入检查。
- Alert 的测试与 E2E 覆盖范围来自静态核对，本轮未执行 Go 测试、前端测试、`e2e-alert.ps1` 或 `e2e-alert-sync.ps1`，不记为运行通过。

## 10. Asset 模块检查记录

- Asset 轮次修改：新增 `internal/asset/README.md`，更新根 README、internal/ops 索引、云观测与 CES 契约、ADR、Runbook、backlog、dimension 方案、前端 API、验收、演示及本记录，共 14 份文档。
- 路由/权限自动比对：21 个 Asset 路由注册（15 个唯一路径）均能在云观测契约中找到；`app:assets:read`、`app:assets:write` 均有说明。应用/资源/匹配规则写接口的路径参数已补齐为代码使用的 `:id`。
- 验收编号检查：Asset 子章节为 2.1、2.2、2.3、2.4、2.5、2.6、2.7，无重复编号。
- Asset 相关 14 份文档均通过尾随空格、代码围栏和本地链接检查；本轮未执行 Go 测试、前端测试或 Asset E2E，不记为运行通过。
- 本轮仅修改文档，未修改 Sync 数据范围、审计、reaper 或云应用命名实现。

## 11. Runbook 模块检查记录

- Runbook 轮次修改：新增 `internal/runbook/README.md`，重写 `ops/runbook-contract.md`，更新前端 API、验收、演示、internal/ops 索引及本记录，共 8 份文档。
- 路由/权限自动比对：6 个 Runbook 路由注册（3 个唯一路径）均能在契约中找到；`app:runbooks:read/create/update/delete` 均有说明。
- Runbook 相关 8 份文档均通过尾随空格、代码围栏和本地链接检查；`git diff --check` 无错误。
- 本轮未执行 Go 测试、前端测试或 `e2e-runbook.ps1`，不记为运行通过；Command Spec 和 Runbook→Agent 仅登记为规划，未修改实现。

## 12. Execution 模块检查记录

- Execution 轮次修改：新增 `internal/execution/README.md`，重写两份 Execution 契约，更新根 README、internal/ops 索引、前端 API、验收、演示及本记录，共 10 份文档。
- 路由/权限自动比对：16 个用户侧与 Agent 路由注册（14 个唯一路径）均能在契约中找到；7 个用户侧权限码均有说明，Agent Token 路由单独标注。
- Execution 相关 10 份文档均通过尾随空格、代码围栏和本地链接检查；`git diff --check` 无错误。
- 本轮未执行 Go 测试、前端测试或 Execution E2E，不记为运行通过；未修改独立 Agent、自动选择、mTLS、Runbook→Agent 或 Agent Alert 时间线实现。

## 13. Dashboard 模块检查记录

- Dashboard 轮次修改：新增 `internal/dashboard/README.md` 和 `ops/dashboard-contract.md`，更新根 README、internal/ops 索引、前端 API、验收、演示及本记录，共 9 份文档。
- 路由/权限自动比对：`GET /api/dashboard/summary` 与 `app:dashboard:read` 均进入独立 Dashboard 契约。
- 契约索引检查：当前 14 份 `ops/*-contract.md` 均进入根 README 与 ops 索引。
- Dashboard 相关 9 份文档均通过尾随空格、代码围栏和本地链接检查；本轮未执行 Go 测试、前端测试或业务 E2E，不记为运行通过。

## 14. Audit 模块检查记录

- Audit 轮次修改：新增 `internal/audit/README.md`，重写 `ops/audit-contract.md`，更新根 README、internal/ops 索引、前端 API、验收、演示及本记录；同步修正 Alert CES 审计事务旧描述和 Identity 的 Asset Sync 数据范围旧引用。
- 路由/权限自动比对：唯一路由 `GET /api/audits`、权限 `app:audits:read`、migration `0009_init_audit` 均进入 Audit 契约；认证审计独立接口和权限另行标注。
- 前端行为比对：精确筛选、当前页聚合、分类卡口径、`NO_TRACE`、CSV 100 条上限和资源类型选项差距均来自页面代码，不把视觉存在等同于全量能力。
- 相关 Markdown 通过尾随空格、代码围栏和本地链接检查；本轮未执行 Go 测试、前端测试或业务 E2E，不记为运行通过，未修改任何业务代码。

## 15. Integration 模块检查记录

- Integration 轮次修改：重写 `internal/integration/README.md`，更新云观测契约 §4/§10/§11、根 README、internal/ops 索引、整体调用关系、前端 API、验收、演示及本记录。
- 路由/权限自动比对：6 个路由注册（3 个唯一路径模式）和 `app:integrations:read/create/update/delete/check` 均进入契约；migration 0018/0024/0029 均有说明。
- 实现行为比对：checker 无网络调用、失败结果 HTTP 语义、capabilities 默认值、凭据保留/清除、事务与审计边界均按代码记录，未把 E2E 的 none 主路径扩大为全 Provider 验证。
- 相关 Markdown 通过尾随空格、代码围栏和本地链接检查；本轮未执行 Go 测试、前端 Vitest 或 Integration E2E，不记为运行通过，未修改任何业务代码。

## 16. Observability 模块检查记录

- Observability 轮次修改：重写 `internal/observability/README.md`，更新云观测契约、演进 Roadmap、根 README、internal/ops 索引、整体调用关系、前端 API、验收、演示及本记录。
- 路由/权限自动比对：4 个公共路由及 `app:observability:read` 均进入契约；内部 resources/alerts/RDS/evidence 不再被误写为公共 HTTP。
- 实现行为比对：Provider 支持矩阵、公共 project_id 绑定、拓扑必填参数、CES 枚举、普通证据与 AI 定稿 fail-closed 均按代码核对；migration 0019/0060/0061/0062 边界已说明。
- 相关 Markdown 通过尾随空格、代码围栏和本地链接检查；本轮未执行 Go 测试、前端 Vitest 或 Observability/AI 证据 E2E，不记为运行通过，未修改任何业务代码。

## 17. Inspection 模块检查记录

- Inspection 轮次修改：重写 `internal/inspection/README.md`，更新云观测契约、演进 Roadmap、根 README、internal/ops 索引、整体调用关系、前端 API、验收、演示及本记录。
- 路由/权限自动比对：10 个路由注册（7 个唯一路径模式）及 inspections read/write、executions:create 均进入契约；getPolicy 后端与前端封装一致。
- 实现行为比对：同步终态、scope 采集限制、规则阈值、Artifact 事务、显式介体、Recommendation 状态和非事务边界均按代码记录；0020/0045 与 AI 工具权限种子边界已说明。
- 相关 Markdown 通过尾随空格、代码围栏和本地链接检查；本轮未执行 Go 测试、前端 Vitest 或 Inspection E2E，不记为运行通过，未修改任何业务代码。

## 18. AI Provider 与工具网关检查记录

- AI Provider/工具网关轮次新增 `internal/ai/README.md`，更新 AI/审计契约、根 README、internal/ops 索引、整体与详细调用关系、前端 API、验收、演示、配置示例注释及本记录；同步修正 Inspection 工具种子表述。
- 路由/权限比对：Provider 三路由、直接 tools/invoke、analyze-alert、chat/models 及 Identity 工具权限管理接口均按当前注册和权限码记录；直接调用的非 all 数据范围入口限制单独标注。
- 实现行为比对：Provider 空库种子、全量 upsert、密钥保留、出站限制、运行时选择与多实例边界，工具确认豁免、8 个本地只读工具和三条调用链均按代码核对。
- 审计与前端比对：Provider CRUD 无操作审计、直接调用拒绝审计语义、Provider headers 清空、ToolConsole 字段和固定工具目录差距均已登记，未把 mock/fake E2E 写成真实模型通过。
- 自动校验通过：14 份相关 Markdown 的尾随空格、代码围栏和本地链接，14 份契约索引，6 个本轮路由、8 个权限码、8 个内置工具及 9.7.1–9.7.16 验收编号均一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。

## 19. AI 会话与 Agent Loop 检查记录

- AI 会话/Loop 轮次扩充 `internal/ai/README.md`，更新 AI 契约、整体与详细调用关系、记忆演进背景、根 README、internal/ops 索引、前端 API、验收、演示及本记录。
- 路由/权限比对：10 个会话与消息路由及 app:ai:chat、app:ai:chat:feedback 均按当前实现记录；0049 只绑定这两个 chat 权限，不再误写为同时绑定知识/记忆权限。
- 实现行为比对：最近 200 条详情、两个消息事务、幂等重放/冲突、模型回退、8 个 Loop 工具、预算/去重/强制收敛、SSE 取消和最终 answer 竞态均按代码核对。
- 前端与验证比对：归档输入、模型禁用、详情刷新、重试重放、历史重试、多轮展示及同步 90 秒预算差距已登记；三份 E2E 的 mock、SKIP、保留数据和部分断言不再写成完整 UI/真实模型覆盖。
- 证据覆盖比对：CES 规则资源分组展开、RuleHealthCollector 多实例抽样、288 点定稿派生统计与 Loop 预算已于 2026-11-24 补齐；资源分组仍受单次展开 10 条与每规则 3 实例抽样上限约束，小时级聚合仍不能替代原始秒级复查。
- 自动校验通过：12 份相关 Markdown 的尾随空格、代码围栏和本地链接，10 个会话路由、14 份契约索引、8 个 Loop 工具及 9.8.1–9.8.31、9.11.1–9.11.10、9.12.1–9.12.25 验收编号一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。

## 20. AI 记忆与知识沉淀检查记录

- 记忆/知识轮次扩充 `internal/ai/README.md`，更新 AI 契约 §10.14/§10.15/§10.19、根 README、internal/ops 索引、前端 API、验收 9.9/9.10、演示 8F 及本记录。
- 路由/权限比对：6 个知识管理路由、2 个记忆管理路由及 app:ai:knowledge:manage、app:ai:memory:manage 均按当前实现记录；无授权适配器时路由不注册。
- 实现行为比对：记忆唯一 chat valid 来源、幂等索引、invalid 降分、match_rank、keywords/signature、知识注入上限、hit/useful、invalid 评估、auto_disable、聚合 created/updated/skipped_active/insufficient_cases 均按代码核对。
- 前端与验证比对：MemoryManager/KnowledgeManager 筛选、分页、字段展示、确认和聚合入参差距已登记；两份 E2E 的迁移前置、SKIP、写数据、清理和未覆盖项不再写成完整通过。
- 自动校验通过：相关 Markdown 的尾随空格、代码围栏和本地链接，8 个 memory/knowledge 路由、2 个权限码及 9.9.1–9.9.10、9.10.1–9.10.13 验收编号一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。

## 21. AI 分享与多模态检查记录

- 分享/多模态轮次扩充 `internal/ai/README.md`（新增 §10 与迁移表 0053 行），更新 AI 契约 §10.18/§10.20、前端 API、验收 9.13/9.16、演示 8G 及本记录。
- 路由/权限比对：4 个分享路由均复用 `app:ai:chat`；owner 校验在 application 层 `loadOwnedSession`，访问分享不校验归属；`SetShares` 未注入时 503，标准装配已注入。无新增迁移权限种子。
- 实现行为比对：token 32 字节 base64url/SHA-256 落库、TTL 1–720h 默认 168、单会话 10 条 active 上限、撤销幂等但每次仍写 `chat_share_revoke` 审计、四类不可用统一 404、会话删除不级联分享记录（无外键）、分享视图最近 200 条且归档可看、路径日志脱敏均按代码核对。
- 多模态比对：normalizeChatImages 规则（3 张/png/jpeg/webp/2MB）、视觉门控在幂等与归档之后落库之前、请求>会话>默认模型回退、OpenAI text+image_url 报文、历史只保留文本、Loop 各轮与强制收敛轮均携带、images_json 与 image_count 审计均按代码核对。
- 前端与验证比对：确认 SharedSession.vue 不渲染附图（验收 9.16.6 原描述不成立，已修正）；图片链路无 AI E2E、ShareDialog/SharedSession/useChat 图片路径无专项 Vitest 已登记；分享 E2E 覆盖创建/访问/撤销/审计，列表/过期/owner/上限由单测覆盖。
- 自动校验通过：本轮修改 Markdown 的尾随空格、代码围栏和本地链接检查通过，4 个分享路由、9.13.1–9.13.9、9.16.1–9.16.7 验收编号一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。

## 22. AI 浮层与消息排队检查记录

- 浮层/排队轮次扩充 `internal/ai/README.md`（新增 §11），更新 AI 契约 §10.8/§10.9、前端 API、验收 9.8.32/9.8.33、演示 8H 及本记录。
- 实现行为比对：控制器在 BasicLayout provide、浮层与工作台 inject 同一实例、路由切换不销毁但刷新即丢失（不写 localStorage）；排队条目 content/images/context/provider_id/client_message_id 按入队快照冻结；完成 finally 自动 sendNextQueued；stop 清空当前会话队列并中止流；删除会话先清流与队列；归档会话排队逐条尝试逐条失败不整体终止；selectSession 对已加载/生成中/有队列会话直接接管本地状态均按代码核对。
- 入口与上下文比对：顶栏按钮与 `Alt + A`（keydown 挂载/卸载已确认）默认入口、告警详情 askAI 先 syncAlertAIContext 再 openOverlay({selectSession:true})、新建会话携带 alert_id、执行提议与引用导航先关浮层再跳转均按代码核对；上下文字段与写入方以契约 §10.8 为准。
- 前端与验证比对：useChat.spec 仅 1 个并发/排队用例、AIAssistantOverlay.spec 4 个选择器用例、aiContext.spec 6 个上下文用例、QueuedMessageList.spec 仅事件透传；停止清队/队列管理接线/归档排队失败/删除清理/快捷键/窄屏宽度无专项测试已登记。排队为纯前端行为，后端无队列或会话锁。
- 自动校验通过：本轮修改 Markdown 的尾随空格、代码围栏和本地链接检查通过，9.8.1–9.8.33、9.12.1–9.12.25 验收编号一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。

## 23. AI RDS 诊断检查记录

- RDS 诊断轮次扩充 `internal/ai/README.md`（新增 §12），更新 AI 契约 §10.21、验收 9.17.9、演示 8I 及本记录。
- 双入口与触发比对：Chat gather 自动取数、Loop 显式补采（from/to 成对、limit 0–300）与 analyze-alert 三条路径均先校验 `postgres.activity.query` 工具权限与账号数据范围；触发条件为长事务告警或指标证据已含 `oldest_transaction_duration` 数据，均按代码核对。
- 采集与脱敏比对：`invalid_rds_instance_id` 白名单、7 天窗口、15s 总预算（`ai.postgres_evidence_timeout_ms` 默认 15000）、每页 100/单类 300 分页、total_count=-1 视为 unknown 仍按 truncated、会话主证据失败整体失败而等待事件/Top SQL 失败进 missing_evidence、user_hash/网段/sql_hash/300 runes sql_summary（字面量与注释移除、未闭合截断）均按代码核对。
- 定稿与诊断比对：先落库再进模型、超限截断次序（Top SQL→等待事件→逐条会话）、快照缺失/写库失败 fail-closed 转 error 并清 evidence_id、四类根因分类优先级、最多 6 条会话事实与 3 条匹配 Top SQL、模型输出 diagnosis 被忽略重建、重放保留、JSON 空数组均按代码核对。
- 前端与验证比对：AnswerCard 诊断区块与 AnswerCard.spec 覆盖已确认；告警 AI 弹窗不展示 diagnosis（alert/internal、前端 API README 原有登记与本轮一致）；E2E 只清 mock、会话与证据保留、IAM 缺口组合需人工已写入验收前置。
- 自动校验通过：本轮修改 Markdown 的尾随空格、代码围栏和本地链接检查通过，9.17.1–9.17.9 验收编号一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。

## 24. ai-orchestrator 检查记录

- ai-orchestrator 轮次扩充 `internal/ai/README.md`（新增 §13），更新 AI 契约 §9.4、internal/README 运行时行、验收 9.7.17 及本记录。
- 平台侧比对：类型 c 注册为 `InternalServiceExecutor`（`/api/tool/invoke` + `X-API-Key: Bearer`，1MiB 响应上限、非 2xx UNAVAILABLE、出站校验在平台侧）；`AnalysisProviderID` 优先 b→c→其他，对话只用 b；直接调用与告警分析两条链可达均按代码核对。
- 服务侧比对：6 个环境变量（3 个模型项必填 + ADDR/API_KEY/MODEL_PATH/TIMEOUT_S）、双路径监听与 `/healthz`、X-API-Key 剥离 Bearer 后常数时间比较、仅 `alarm.analyze`、2MiB 请求/4MiB 模型响应上限、429/5xx 重试 2 次、references 合并去重、失败返回 200 保守降级均按代码核对。
- 关键边界登记：降级响应**无 `degraded` 字段**，平台不记 `provider_degraded`/审计 partial，与类型 b 语义不同；服务不消费 `postgres_activity`/`similar_cases`，长事务 `diagnosis` 始终由平台侧生成并覆盖 summary/recommendations；无 compose/K8s 定义、无专项 E2E，Makefile build 只构建 `cmd/api`。
- 验证比对：`analysis_test.go` 7 个用例、toolgateway 类型 c 仅 stub 覆盖；验收 9.7.17 登记为手工构建人工验收项。
- 自动校验通过：本轮修改 Markdown 的尾随空格、代码围栏和本地链接检查通过，9.7.1–9.7.17 验收编号一致；`git diff --check` 无空白错误。
- 本轮未执行 Go 测试、前端 Vitest 或 AI E2E，不记为运行通过，未修改任何业务代码。
