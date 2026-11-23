# 文档与当前实现复核记录

- 最近复核：2026-11-23
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
- `ops/README.md`：收录全部 13 份契约，建立契约到代码、前端、迁移和验证证据的映射，明确 fake 与真实验收、规划与已实现章节的区别。

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

## 4. 已确认的实现边界

这些边界应在对应模块文档中保留，不能在后续整理时被简化成“全部完成”。

| 模块 | 当前实现 | 尚未完整交付或需单独验收 |
| --- | --- | --- |
| Identity | 本地/LDAP/OAuth、JWT、RBAC、数据范围、AI 工具权限及管理接口 | 动态菜单、MFA；数据范围按调用点生效；E2E 不覆盖全部权限写接口；前端 Token 持久化与安全约束冲突 |
| Alert | Webhook、状态流转、CES 手动拉取 | 创建/更新所有源均需 Integration 读权限；列表/详情无自动数据范围过滤；静默无到期自动恢复；worker 默认关闭；真实账号验收与 fake E2E 分开 |
| Asset | 注册表、匹配规则、异步同步、批次、CMDB 拓扑和关系 | Sync 数据范围、审计生命周期、triggered_by/后台 reaper、云应用命名有差距；真实云对账与增强缺口单独验收 |
| Runbook | 模板、推荐、模拟路径结构化多步任务 | 前端仅列表/详情/启停；Command Spec 与 Runbook→Agent 未落地；default_dry_run 不单独触发模拟 |
| Execution | 核心状态机、模拟执行、Agent 服务端控制面 | 独立 Agent 程序、介体/Agent/Command Spec 专门 UI 未提供 |
| Dashboard | 聚合摘要 API 与页面 | 无独立 Dashboard E2E |
| Audit | 后端分页检索，前端详情、聚合和 CSV 导出 | 无独立 Audit E2E；导出范围不是服务端全量导出任务 |
| AI | 会话、SSE、工具循环、记忆/知识、分享、多模态、执行提议、证据诊断 | 前端消息队列不持久化；分享页图片和告警 AI 弹窗诊断存在前端缺口；type=c 仅为参考服务 |
| Integration | 账号、凭据密文、能力与连通性 | 连通性不代表所有观测能力都有真实 Adapter |
| Observability | 四类统一查询、华为 CES 部分真实路径、AI 证据快照 | 真实日志/链路/拓扑仍有限制；快照无公共详情 API/UI |
| Inspection | 策略、手动运行、Finding、Recommendation | 定时 worker 未落地；建议转执行有后端路由但无前端入口 |

## 5. 后续模块文档待处理项

### 5.1 契约和目录 README

- `ops/execution-agent-contract.md`：仍含草案措辞，且注册 token、介体权限码、验收范围与当前代码存在差异；不能只把“草案”改成“稳定”。
- `ops/ai-contract.md`：前部“当前工具”清单需补 `postgres.activity.query`，并统一分享、多模态、证据和诊断边界。
- `cmd/api/README.md`：装配说明仍偏 Identity/AI，需对齐当前 11 个业务上下文及后台任务。
- `web/src/views/README.md`、`web/src/stores/README.md`、`web/src/router/README.md`：仍停留在早期概念说明，需补实际页面、aiContext、共享会话和权限边界。
- `web/src/api/README.md`：拓扑函数归属文件和跨应用关系已修正；Inspection `getPolicy` 留待 Inspection 轮次核对。

### 5.2 部署、演示和发布

- `deployments/kubernetes.md`：手工 SQL/手工账本与自研 runner 冲突；迁移清单停在 `0017`；关于镜像不含迁移二进制的描述已过时。
- Kubernetes 示例还需核对生产必需的 Execution Agent 注册令牌、SSE/Ingress 超时和安全配置。
- `docs/release-checklist.md`：存在旧迁移号示例、旧前端静态目录、E2E 覆盖偏旧及 `go test` 与 race 表述不一致等问题。
- `docs/demo-flow.md`：仍有旧迁移基线和“全部 E2E”范围不清等问题；Alert 告警源字段与 secret 语义已随本轮修正。
- `deployments/README.demo.md`：历史验证结果需按当前迁移基线重述；`deployments/README.md` 的默认管理员前提已随 Identity 轮次修正。
- `scripts/` 没有统一索引；后续应说明每个脚本的前置条件、外部资源、写数据/清理行为和适用环境。

### 5.3 不能只靠文档关闭的实现差距

- 分享会话页未复用普通用户消息的图片渲染，与多模态分享验收项不完全一致。
- 后端告警分析已有 `diagnosis`，但告警 AI 弹窗类型和展示尚未完整接入；AI 助手回答卡片已有诊断展示。
- Execution Agent 管理 UI、Inspection 建议转执行入口、动态菜单仍未实现。
- 前端 `stores/auth.ts` 将 access/refresh token 写入 localStorage，与项目“不将 Token 写入前端持久化状态”的安全约束冲突，需独立安全整改。
- Asset Sync 未按早期契约接入 owner_team/region 数据范围，批次列表和详情也未完整隔离；AI 资源读取的独立校验不能替代该缺口。
- Asset Sync 审计仍统一 `action=sync`，缺少独立生命周期 action 和完整 summary/scopes 载荷；`triggered_by` 未写入，周期后台 reaper 未装配。
- `ensureCloudApplication` 新建应用名仍为 `<provider>-cloud`，历史迁移归一化不能保证后续新建名称符合目标。
- Runbook 步骤尚无 command_spec_id/medium_selector，Agent 模式不会展开 Runbook 模板；不能由 Execution Agent 控制面存在反推该集成已完成。
- 这些项目应在文档中标为缺口；是否补代码由后续需求单独确认。

## 6. 逐模块更新队列

每轮只处理一个模块，同时核对后端、前端、契约、权限、审计、迁移和验证证据。

1. 全局基线：根 README、文档复核记录、internal/ops 索引。
2. P0 主链路：Identity（已完成）→ Alert（已完成）→ Asset/CMDB/拓扑（文档已完成，实现差距已登记）→ Runbook（文档已完成，Command Spec 边界已登记）→ Execution/Execution Agent → Dashboard → Audit。
3. 云与观测：Integration → Observability/证据快照 → Inspection；CES 告警拉取归入 Alert，CES 资源同步归入 Asset。
4. AI：Provider/工具网关 → 会话/Agent Loop → 记忆/知识 → 分享/多模态 → 浮层/消息排队 → RDS 诊断 → ai-orchestrator。
5. 前端公共能力：API/request/SSE → Router/导航 → Stores/布局与跨页状态。
6. 工程与交付：cmd/bootstrap/server/version/pkg → migrations → Compose/Kubernetes → scripts/E2E → demo/验收/发布清单。

## 7. 全局基线检查记录

- 修改范围：仅 `README.md`、`internal/README.md`、`ops/README.md`、`docs/documentation-review.md`。
- `git diff --check`：通过，无尾随空格或空白错误。
- Markdown 本地链接与代码围栏检查：4 份修改文档均通过，未发现缺失链接目标或未闭合围栏。
- 索引完整性检查：13 份契约均进入根 README 与 ops 索引；19 个默认 E2E 和 2 个带外部前提的可选 E2E 均有入口；`internal` 顶层模块均有说明。
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
