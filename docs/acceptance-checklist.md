# AIOps 阶段验收清单

**范围**：告警接入 → 资产匹配 → Runbook 推荐 → 执行记录 → Dashboard 摘要 → 权限校验
**环境**：PostgreSQL + API（8080）+ 前端（UI 验收时）
**默认账号**（仅限 dev/演示）：`admin` / `admin123`（须叠加 `docker-compose.dev.yml` 或 bootstrap 启用）。迁移 `0044` 后生产默认锁定该账号，须 `scripts/provision-prod-admin.ps1` 创建安全管理员后再验收。

---

## 0. 前置准备

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 0.1 | 依赖启动 | `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d` | Postgres 健康 |
| 0.2 | 迁移 | `go run ./cmd/migrate` | 含 0001–0062，无报错；当前仓库未包含 `0021` Notification 迁移 |
| 0.3 | API 启动 | `go run ./cmd/api` 或 compose 内 api 服务 | 登录接口可用 |
| 0.4 | 前端（UI 验收） | `cd web && npm run dev` | 可登录并访问各页面 |
| 0.5 | 自动化冒烟 | 见 [§7 推荐验收顺序](#7-推荐验收顺序自动化) | 全部输出 `PASS` |

---

## 1. 告警接入

**权限**：`app:alerts:read/acknowledge/assign/update/close/silence/ingest`；当前创建/更新源及手动同步还需要 `app:integrations:read`
**脚本**：`scripts/e2e-alert.ps1`、`scripts/e2e-alert-sync.ps1`（fake CES）

### 1.1 Webhook 接入

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 1.1.1 | 创建告警源 | `POST /api/alerts/sources`（Bearer） | `code=OK`，源 `enabled=true` |
| 1.1.2 | Firing 首次接入 | `POST /api/alerts/ingest/alertmanager/{source_id}` + `X-AIOPS-Webhook-Token` | `created >= 1` |
| 1.1.3 | 同 fingerprint 重复 | 再次发送相同 firing | `updated >= 1`，`occurrence_count` 递增 |
| 1.1.4 | Resolved 恢复 | 发送 resolved payload | `recovered >= 1`，告警 `status=recovered` |
| 1.1.5 | 关闭后再 firing | close → 再 firing | 产生新告警 `status=new` |
| 1.1.6 | 无效 Token | 错误/缺失 webhook token | 401/403，不写入告警 |

### 1.2 状态流转与时间线

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 1.2.1 | 认领 | `POST /api/alerts/{id}/acknowledge` | `status=acknowledged` |
| 1.2.2 | 开始处理 | `POST /api/alerts/{id}/start-processing` | `status=processing` |
| 1.2.3 | 手动恢复 | `POST /api/alerts/{id}/recover` | 状态更新，时间线有对应 event |
| 1.2.4 | 详情完整性 | `GET /api/alerts/{id}` | 含 `labels`、`events[]`、`severity`、`environment`；`related` 当前为空对象 |
| 1.2.5 | 转派 | `POST /api/alerts/{id}/assign` | 处理人更新，时间线含 `assigned`；closed 返回 400 |
| 1.2.6 | 静默与取消 | `POST /silence`、`/unsilence` | `silenced → new`，记录截止时间和原因；最长 30 天 |
| 1.2.7 | 静默到期边界 | 等待或构造已过期 `silenced_until` | 当前无到期自动恢复 worker；状态不会仅因到期自动变回 new，需手动取消或外部恢复 |
| 1.2.8 | 备注 | `POST /api/alerts/{id}/comments` | 时间线含 `commented`；closed 返回 400，最长 2000 字符 |
| 1.2.9 | 数据范围边界 | 用非 all 数据范围用户调用列表/详情，再调用 AI 分析 | 列表/详情当前只做 RBAC；AI 分析对象校验是独立链路，不得混写为同一隔离能力 |

> 1.2.5–1.2.8 不在当前 `e2e-alert.ps1` 内，需通过领域/应用测试、专项接口或 UI 单独记录。

### 1.3 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 1.3.1 | 告警列表 `/alerts` | 可按源/状态筛选，活跃告警可见 |
| 1.3.2 | 告警详情抽屉 | 时间线、标签、状态操作按钮可用 |
| 1.3.3 | 资源链接 | 已匹配资产时显示应用/资源链接 |
| 1.3.4 | 友好字段 | 打开含 `prod`、`critical`、Region、资源类型和规则注解的告警 | 环境、严重程度、字段名和规则摘要使用中文并保留原值；长 ID 可复制，未知 key 不丢失 |
| 1.3.5 | 原始追溯与未匹配 | 切换“友好字段 / 原始字段”，并打开未匹配应用和未注册资源的告警 | 原始 key/value 完整可见，敏感 key 仍掩码；显示“未匹配应用”和“未注册资源”，可跳转匹配规则页 |
| 1.3.6 | 指标语义与资源解析 | 打开 `oldest_transaction_duration > 600000ms` 的 RDS 告警，分别准备唯一匹配、无匹配和多候选资源 | 标题显示“最长事务存活时长超过 10 分钟”，阈值含原毫秒值；唯一匹配显示真实资源/应用名，无匹配和多候选不乱选；原始摘要、描述与 ID 可追溯 |
| 1.3.7 | AI 入口与诊断边界 | 分别打开“AI 分析”和“问 AI 助手” | 前者写时间线并展示风险/摘要/建议/引用；后者携带告警上下文并选择会话。告警弹窗尚未完整展示 `diagnosis`，不得以 AI 助手卡片替代该项验收 |

### 1.4 CES 拉取接入

已随迁移 0046/0047 落地。手动同步需要 `app:alerts:ingest` 和绑定账号的 `app:integrations:read`。冒烟脚本：`scripts/e2e-alert-sync.ps1`。详见 [CES 告警拉取契约](../ops/huawei-ces-alert-sync-contract.md)。

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 1.4.1 | 创建和校验 | 创建 `delivery_mode=pull`、`type=huawei_ces`、绑定合格账号的源 | 创建成功。遗漏账号、非华为云账号、无效区域和非法间隔返回 400 |
| 1.4.2 | 多资源拉取 | 同一云规则下两个资源分别触发，调用手动 sync | 两条流水分别入库，使用正确的资产和区域。缺少唯一资产匹配时留空 |
| 1.4.3 | 重复观测 | 同一 V2 响应再次 sync，另测人工恢复或关闭后的重放 | `created=updated=recovered=0`，触发计数和时间线保持一致，人工处理结果保留 |
| 1.4.4 | 独立恢复 | 测试 fixture 仅让一个流水进入 `ok` | 对应告警恢复且只写一次恢复事件，另一个仍处理。旧 firing 不重开告警，缺页和 `invalid` 不触发恢复 |
| 1.4.5 | 接入方式兼容 | 迁移前后的 `huawei_ces` Webhook、互调接入入口 | 存量 token 继续可用。Webhook 源调 sync 返回 400。公开 Webhook 调拉取源返回 404 |
| 1.4.6 | 跨实例互斥 | 两实例同源手动争抢，另测手动与定时重叠、在途停用与租约过期 | 仅一轮获租约。竞争手动请求和在途停用返回 `ABORTED`（409），过期持有者无法写入 |
| 1.4.7 | scope 隔离与重放 | 单 region 失败，另测后续页超时及部分记录入库后写失败 | 报告已提交计数与 `failed_scopes`，失败窗口不推进游标，重试无重复变更 |
| 1.4.8 | 权限及账号状态 | 用只有告警 ingest 权限的用户绑定或同步，另测已停用和已删除账号 | 权限不足为 403，账号停用为 412，账号删除为 404 |
| 1.4.9 | 审计与收尾 | 完成后查 `GET /api/audits`，分别模拟同步状态提交失败和审计写入失败 | 正常完成时存在 `alert_source_sync`，状态、计数与 trace 可关联；状态提交失败返回 500 并保留原最近同步状态；状态已提交后的审计失败不回滚同步结果 |
| 1.4.10 | 可选 worker | 默认启动核对配置，再在单实例显式启用 worker | 默认 `alert_sync.worker_enabled=false`；`pull_interval_seconds=0` 只手动；多实例只在一个 API 实例启用 |

> 当前 `e2e-alert-sync.ps1` 实际覆盖 Pull 创建、Webhook sync 400、三轮拉取、独立恢复、重复观测、最近同步状态和审计。1.4.3–1.4.9 中的真实云、多实例、权限负向、失效/缺页及全部规则富化分支不因 fake 脚本 PASS 而自动通过。

---

## 2. 资产匹配

**权限**：`app:assets:read`、`app:assets:write`
**脚本**：`scripts/e2e-asset.ps1`

### 2.1 注册表 CRUD

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.1.1 | 创建应用 | `POST /api/assets/applications` | 返回 `id`，字段正确入库 |
| 2.1.2 | 创建资源 | `POST /api/assets/resources` | 绑定 `application_id`，pod/namespace 等正确 |
| 2.1.3 | 孤儿资源拦截 | 对不存在的 `application_id` 创建资源 | `NOT_FOUND`，不落库 |
| 2.1.4 | 列表查询 | `GET /api/assets/applications`、`GET .../resources` | 与创建一致 |
| 2.1.5 | 有规则时删应用 | 存在引用该 `application_id` 的匹配规则时删除应用 | `FAILED_PRECONDITION`（HTTP 412） |
| 2.1.6 | 有规则时删资源 | 存在引用该 `resource_id` 的匹配规则时删除资源 | `FAILED_PRECONDITION`（HTTP 412） |
| 2.1.7 | 资源列表筛选 | `GET .../resources?keyword=xxx&source=cloud_sync&cloud_resource_type=ecs&region=cn-north-4&sync_status=active` | 服务端过滤后 `total` 与 `items` 一致；`keyword` 跨字段模糊匹配，非法 `%` / `_` 按字面处理 |
| 2.1.8 | 资源列表排序 | `GET .../resources?sort_by=name&sort_order=asc` / `desc` | 全页按字段升/降序；非法 `sort_by` 回退 `created_at ASC, id ASC` |
| 2.1.9 | 前端表头排序与筛选 | `/assets` 点击列头、输入关键词并选择筛选条件 | 表头箭头与结果顺序一致；切换排序/筛选回到第 1 页；筛选条件以可移除标签回显 |
| 2.1.10 | 资源筛选字典 | `GET .../resource-facets`；页面云类型/Region 下拉 | 仅返回当前应用下非空去重值且按字典序；无数据返回 `[]`；下拉选中后仍由服务端精确过滤 |
| 2.1.11 | 筛选状态入 URL | 设置筛选/排序后复制或刷新 `/assets?application_id=...&keyword=...&sort_by=...` | 刷新后筛选、排序、页码（第 1 页）与列表结果一致；清空筛选同步移除 URL 参数 |
| 2.1.12 | URL 前进/后退 | 改变筛选条件后用浏览器后退/前进，或手工改地址栏 `keyword` / `sort_by` | 筛选控件、表头箭头与列表结果随 URL 复原，且不产生重复请求 |
| 2.1.13 | 空结果态 | 应用无资源 / 筛选条件无匹配 | 分别显示「该应用下还没有资源 + 新建资源」与「没有符合条件 + 清空筛选条件」，加载中不闪现空态 |
| 2.1.14 | 列设置持久化 | 勾选附加列（含创建/更新时间）后刷新页面 | 列保持勾选；后端已支持 created_at/updated_at 排序，前端当前只开放 created_at 排序；localStorage 脏值被白名单过滤 |
| 2.1.15 | 导出 CSV | 设置筛选后点击「导出 CSV」 | 导出的是当前筛选+排序下的全量结果（非仅当前页）；CSV 带 UTF-8 BOM，逗号/引号/Labels JSON 正确转义；超过 2000 条时提示上限 |

### 2.2 §9.1 标签匹配

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.2.1 | 应用名匹配 | 告警 labels 含 `service=<app_name>` + `env=prod` | `application_id` 命中 |
| 2.2.2 | Pod 匹配 | labels 含 `pod=<registered_pod>` | `resource_id` 命中 |
| 2.2.3 | 精确环境优先 | 同名应用：一条 `environment=''`，一条 `environment=prod` | prod 告警归 prod 应用 |
| 2.2.4 | 未匹配降级 | 无对应注册表 | 告警仍保存，资产 ID 为空 |

### 2.3 云资源同步（migration 0023 起，部分能力仍有差距）

**脚本**：`scripts/e2e-asset-sync.ps1`（fake）；`scripts/e2e-asset-sync-real.ps1`（真实账号，可选）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.3.1 | 触发与轮询 | `POST /api/assets/sync` 后查询批次详情 | 立即返回 `batch_id` 和 running；轮询到 success/partial/failed 后读取 summary/message |
| 2.3.2 | fake 账号同步 | `huawei_cloud` + `auth_type=none` | 资产表出现 `source=cloud_sync` 资源，含 `cloud_resource_id` |
| 2.3.3 | 批次查询 | `GET /api/assets/sync/batches?account_id=` | 分页返回历史批次 |
| 2.3.4 | stale 标记 | 通过单元测试或变化清单二次同步 | 未出现资源标记 `sync_status=stale`，不物理删除；当前 fake E2E 只验证稳定清单 stale_count=0 |
| 2.3.5 | 前端展示 | `/assets` 资源列表与批次详情 | 来源、云资源 ID、region、同步状态和 scopes 明细可见 |
| 2.3.6 | P0 不受影响 | 同步失败或禁用账号 | 告警 ingest / 匹配 / 执行闭环仍可跑 |
| 2.3.7 | CES 资源分组口径 | 分别测试显式组、默认候选名未命中和全部资源组并集 | 显式组未命中失败；默认候选未命中可进入 `all_groups`；聚合数量差异可由 summary 解释，不宣称覆盖未分组资源 |
| 2.3.8 | CES 类型覆盖 | CES 中存在 EVS/VPC/OBS/DCS/DMS 等非 ECS 资源 | 对应资源进入 `asset_resource`，`cloud_resource_type` 与 namespace 映射正确 |
| 2.3.9 | hybrid 增强 | `sync_mode=hybrid` 且授予部分云服务只读权限 | CES 基础资源数不下降；ECS/RDS/DCS/DMS/VPC 子资源增强已生效；EVS 不命中、ELB/CCE 空跑、OBS 等无增强；逐类型失败、阶段错误或回写失败置 partial，warnings 单独判断 |
| 2.3.10 | native 兼容 | `sync_mode=native` | 沿用旧 ECS/CCE/RDS/ELB 同步路径，但不承诺与 CES 总览数量一致 |
| 2.3.11 | 多区域 project_id | `extra_config.region_projects` 配置多 region 映射 | 每个 region 使用对应 project_id 调 CES；未命中回落账号 `project_id` |
| 2.3.12 | 真实账号对账（可选，不进 CI） | 手工执行 `scripts/e2e-asset-sync-real.ps1` | 打印 region 摘要与 type 分组计数，人工比对 CES 控制台总数 |
| 2.3.13 | 数据范围差距 | 用非 all 数据范围用户触发 Sync、查询批次列表和详情 | 当前只有 RBAC 和账号 capability 校验；owner_team/region 二次授权与批次过滤未完整接入，不得按早期契约记为通过 |
| 2.3.14 | 审计、reaper 与命名差距 | 查询同步审计、过期批次和新建云应用名 | 当前统一 `action=sync`，triggered_by 未写入，无周期后台 reaper；新应用名仍可能为 `provider-cloud`，均需单独整改 |

### 2.4 可配置匹配规则（migration 0014）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.4.1 | 创建规则 | `POST /api/assets/match-rules`（`label_key=service`, `label_value_pattern=payment-*`） | 返回 `id`，`enabled=true` |
| 2.4.2 | 规则优先匹配 | 告警 `service=payment-xxx` 但应用名不同 | 按规则绑定 `application_id`/`resource_id` |
| 2.4.3 | 禁用规则 | `enabled=false` 后 ingest | 规则不生效，回退默认匹配 |
| 2.4.4 | 删除规则 | `DELETE /api/assets/match-rules/:id` | 规则移除，后续 ingest 走默认逻辑 |

### 2.5 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 2.5.1 | 资产页 `/assets` | 注册表、云同步、匹配规则 Tab 可用，可选中/新建/编辑/删除 |
| 2.5.2 | 告警跳转 | 从告警详情点资源链 → `?application_id=&resource_id=` | 选中应用，资源行高亮并滚动到视口 |
| 2.5.3 | 时间列与排序 | 展示 created_at/updated_at，并检查排序白名单 | 后端已支持两者排序；前端当前只开放 created_at 排序，updated_at 可展示和导出，不误写为后端缺口 |

### 2.6 CMDB 资源拓扑与手工关系（migration 0058）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.6.1 | 真实归属图 | 创建应用和两个资源后访问 `/assets/topology?application_id=...` | 应用与资源来自注册表，显示两条 `contains` 边，不出现 fake Provider 节点 |
| 2.6.2 | 手工关系 | 在同一应用及跨应用资源间分别创建 `depends_on`，再刷新拓扑 | 返回唯一关系、两端应用 ID 和手工边；重复创建返回 `ALREADY_EXISTS`，自环返回 400，跨应用链路可见 |
| 2.6.3 | 链路与全景 | 选择根资源、切换 1–3 层和上下游方向，再切全景与列表 | 链路按上下游分层并高亮完整路径；全景使用力导向；列表数据一致，循环依赖不死循环 |
| 2.6.4 | 云派生关系 | 准备带真实 `vpc_id` / `subnet_id` / `attached_to` / `bandwidth_id` / Peering 字段的同步资源 | 仅账号、Region、类型和云 ID 唯一匹配时生成 `cloud_sync` 边；不匹配端点计入 `unresolved_relation_count`，无幽灵节点 |
| 2.6.5 | 筛选与截断 | 使用 Region、云类型、关键字、关系类型、stale 筛选；构造超过 299 个资源 | 服务端过滤生效；默认不含 stale；超限时明确显示 `truncated` 提示 |
| 2.6.6 | 删除保护 | 删除仍有关联的资源，随后先删关系再删资源 | 首次返回 `FAILED_PRECONDITION`；关系删除写审计后可继续删除资源 |
| 2.6.7 | 云资源解析 | 调 `/api/assets/resources/resolve`，分别使用唯一、缺失、重复及跨账号/Region 云 ID | 唯一匹配返回真实资源与应用；缺失为 `unregistered`，重复为 `ambiguous`；跨账号/Region 不误匹配 |
| 2.6.8 | 权限与数据范围 | 无 token、只读用户写关系、非 all 数据范围用户查询拓扑 | 分别为 401、403、403；普通拓扑/解析目前只有静态 RBAC，无自动数据范围过滤，页面保留结构 |

> 拓扑目前没有独立 E2E 脚本，以上项目需结合后端单元/HTTP 测试和前端 Vitest 验收，不能写成 `e2e-asset.ps1` 已覆盖。

### 2.7 Asset E2E 覆盖边界

- `e2e-asset.ps1` 覆盖注册表、规则、筛选排序/facets、告警绑定和删除保护，不覆盖云同步、拓扑、资源解析、CSV 或完整 UI。
- `e2e-asset-sync.ps1` 覆盖 fake running/终态、批次列表、云资源入库和稳定清单 stale_count=0，不覆盖真实 CES、变化清单 stale、hybrid、数据范围或租约并发。
- `e2e-asset-sync-real.ps1` 是带真实凭据的可选对账脚本，不属于默认可直接运行的 CI 验收。

---

## 3. Runbook 处置预案

**权限**：`app:runbooks:read/create/update/delete`
**脚本**：`scripts/e2e-runbook.ps1`（依赖 migration 0012 种子模板）

### 3.1 推荐与状态边界

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 3.1.1 | 推荐告警 | `GET /api/runbooks/recommendations?alert_id={id}` | 返回匹配项，含 template_id、steps_count、risk_level、dry_run_supported、parameter_schema |
| 3.1.2 | 匹配维度 | HighCPU + pod + prod 场景 | 命中种子或自定义模板，matched_reason 可解释 |
| 3.1.3 | 推荐状态 | 对 processing 和非 processing 告警分别调推荐 API | API 只要求告警存在，不强制 processing；前端只在 processing 显示创建入口 |
| 3.1.4 | 创建状态拦截 | 对非 processing 告警创建 Runbook Execution Task | 后端返回 `INVALID_ARGUMENT`，不能仅依赖前端隐藏按钮 |
| 3.1.5 | 禁用/空步骤 | 使用 disabled 或无步骤模板创建任务 | 后端拒绝，不生成任务 |

### 3.2 模板 CRUD 与审计

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 3.2.1 | 列表与详情 | `GET /api/runbooks/templates`、`GET .../:template_id` | 分页返回模板，详情含有序 steps；支持 keyword/enabled 查询 |
| 3.2.2 | 创建 | `POST /api/runbooks/templates`，至少一个步骤 | 模板和步骤同事务创建；空步骤、非法 action/risk 返回 400 |
| 3.2.3 | 全量更新 | `PUT /api/runbooks/templates/:template_id` | 模板与步骤全量替换；历史任务 snapshot 不变 |
| 3.2.4 | 软停用 | `DELETE /api/runbooks/templates/:template_id` | enabled=false，不再推荐或创建新任务；模板、历史任务和 snapshot 保留 |
| 3.2.5 | 启停审计 | 切换 enabled 后查询审计 | 有 runbook_update，并追加 runbook_enable 或 runbook_disable；失败不回滚主业务 |

### 3.3 Runbook 到 Execution

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 3.3.1 | 多步与 snapshot | 从 processing 告警创建 Runbook 任务 | 按 step_order 生成多个 exec_step，保存 runbook_template_id 和 runbook_snapshot |
| 3.3.2 | 参数合并 | 提供模板/步骤默认参数和用户 parameters | 用户值覆盖默认值，模板与步骤 schema 均通过；非法参数不创建任务 |
| 3.3.3 | 风险与确认 | 分别验证 low、medium/prod command 和降低 override | low 为 pending_execute，其余 pending_confirm；override 只能提高 |
| 3.3.4 | Dry-run | 任务 dry_run=true，包含支持和不支持 dry-run 的步骤 | 有效值为任务 dry_run 且步骤支持；支持步骤输出 dry_run 标记，不支持步骤为 skipped_real_execution |
| 3.3.5 | default_dry_run 边界 | 仅设置模板步骤 default_dry_run，任务 dry_run=false | 不单独触发 dry-run；default_dry_run 仅保留为模板/snapshot 元数据 |
| 3.3.6 | 时间线 | 创建、确认、执行后查看告警 | 出现 execution_created/started/finished，Runbook 信息可关联 |
| 3.3.7 | Agent 边界 | 检查 Runbook 步骤字段及 execution_mode=agent | 当前无 command_spec_id/medium_selector，Agent 走独立 createAgentTask，不按 Runbook 展开；不得按已落地验收 |

### 3.4 前端与 E2E 覆盖

| # | 检查项 | 预期 |
|---|--------|------|
| 3.4.1 | Runbook 页 `/runbooks` | 列表、详情、启停可用；当前无新建、完整编辑或删除 UI |
| 3.4.2 | 告警页推荐区 | processing 告警可选择模板，填写任务名、dry-run 和 JSON object 参数 |
| 3.4.3 | Dashboard Runbook 区 | 显示已启用/总数，最近 Runbook 执行列表 |
| 3.4.4 | 前端测试边界 | 检查 Runbook 专项 Vitest | 当前无独立 Runbook 前端 Vitest，不能用告警页其他测试替代 |

`e2e-runbook.ps1` 实际覆盖 processing 告警、推荐、prod medium 多步任务、pending_confirm、CONFIRM、模拟执行成功、首步 dry_run 标记和三类执行时间线。它不覆盖模板 CRUD E2E、非 processing 拒绝、low 风险、步骤失败、完整 UI、Command Spec 或 Runbook→Agent。

---

## 4. Execution 核心任务

**权限**：`app:executions:read/create/confirm/execute`
**脚本**：`scripts/e2e-execution.ps1`（普通模拟）、`e2e-runbook.ps1`（Runbook 多步）

### 4.1 创建、确认、拒绝与执行

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 4.1.1 | 简单 alert 任务 | processing 告警 + `operation_type=restart` | `status=pending_confirm`，非 processing 后端拒绝 |
| 4.1.2 | Runbook 任务 | `runbook_template_id` + `dry_run=true` | 多步骤，保存 template_id 和 snapshot |
| 4.1.3 | 低风险 | 创建 low 任务 | `pending_execute`，不是创建后自动执行 |
| 4.1.4 | 确认 | `confirm=true, confirm_text=CONFIRM` | `pending_confirm → pending_execute`；错误文本、重复确认失败 |
| 4.1.5 | 拒绝 | `confirm=false` | `pending_confirm → cancelled`，写 reject 审计；不是预留状态 |
| 4.1.6 | 模拟执行 | `POST .../execute` | 顺序步骤，全部成功为 success；失败停止，后续 pending |
| 4.1.7 | Dry-run | Runbook dry-run 任务 | 支持步骤 output.dry_run=true，不支持步骤 skipped_real_execution |
| 4.1.8 | Agent 同步执行拦截 | 对 execution_mode=agent 调 `/execute` | `FAILED_PRECONDITION`，必须确认后由 Agent lease |
| 4.1.9 | adapter 边界 | 检查执行模式和实现 | adapter 只是保留枚举，没有独立执行链路，不按已落地验收 |

### 4.2 普通 Alert 任务时间线

| # | 检查项 | 预期 |
|---|--------|------|
| 4.2.1 | `execution_created` | 普通 alert 创建后写入，payload 含 execution_id、Runbook 信息（如有） |
| 4.2.2 | `execution_started` | 模拟执行开始后写入 |
| 4.2.3 | `execution_finished` | 完成后写 success/failed 和结果摘要 |
| 4.2.4 | Agent 时间线差距 | Agent 创建/lease/result 当前不写这三类 Alert 事件，不得套用普通模拟结论 |

### 4.3 列表、详情与日志

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 4.3.1 | 列表 | `GET /api/executions/tasks` | 支持 page/page_size/status/source_type/source_id/keyword |
| 4.3.2 | 详情 | `GET /api/executions/tasks/{id}` | 含 task + steps，Runbook/Agent 字段按模式返回 |
| 4.3.3 | 日志查询 | `GET /api/executions/tasks/{id}/logs?step_id=` | step_id 必填，返回脱敏日志；当前前端未封装和展示 |

### 4.4 前端与 E2E 覆盖

| # | 检查项 | 预期 |
|---|--------|------|
| 4.4.1 | 执行页 `/executions` | 列表、详情、CONFIRM、同步执行、Runbook/dry-run 和深链可用 |
| 4.4.2 | 当前缺口 | 检查拒绝、Agent、日志和专项测试 | 无拒绝按钮、Agent 创建/管理/日志 UI、execution.ts Agent 封装或独立 Execution 前端 Vitest |
| 4.4.3 | Dashboard 最近执行 | 与 API 一致，点击可跳转 |

`e2e-execution.ps1` 只覆盖 processing alert、restart pending_confirm、CONFIRM、模拟 success 和三类 Alert 时间线；`e2e-runbook.ps1` 覆盖 Runbook 多步 dry-run。二者不覆盖拒绝、low、步骤失败、日志、adapter、完整 UI 或 Agent。

---

## 5. Dashboard 首页驾驶舱

**权限**：`app:dashboard:read`（migration 0013）
**API**：`GET /api/dashboard/summary`；契约：`ops/dashboard-contract.md`

### 5.1 聚合字段

| # | 字段 | 预期 |
|---|------|------|
| 5.1.1 | `alerts.active_total / p0 / p1` | 只排除 closed；recovered 和 silenced 仍可能计入，不与“仅 firing”混淆 |
| 5.1.2 | `executions.pending_confirm` | 与待确认任务数一致 |
| 5.1.3 | `executions.recent` | 最近最多 10 条，按 created_at DESC，不限 success |
| 5.1.4 | `assets.applications / resources` | 与注册表总量一致；当前无数据范围过滤 |
| 5.1.5 | `runbooks.total / enabled` | 与模板总数/启用数一致，推荐模板不直接包含在 summary |
| 5.1.6 | `processing_alerts` | 最多 5 条，按 last_seen_at DESC，包含 new/acknowledged/processing，不是仅 processing |

### 5.2 容错与降级

| # | 检查项 | 预期 |
|---|--------|------|
| 5.2.1 | 单项统计失败 | 其他块继续返回，失败块为 0/空数组 |
| 5.2.2 | 降级可观测性 | 响应没有 degraded/error 字段，真实 0 与降级 0 不可区分；不得把 0 当作依赖健康证明 |
| 5.2.3 | 数据范围 | 只有 `app:dashboard:read` 静态 RBAC，不能按各业务模块专用数据范围推断聚合已隔离 |

### 5.3 前端聚合与降级

| # | 检查项 | 预期 |
|---|--------|------|
| 5.3.1 | 指标卡与跳转 | 活跃/P0/P1/待确认/资源/平台状态正确，可跳转对应模块 |
| 5.3.2 | Runbook 推荐 | 取前 3 条提示告警分别推荐，每组最多 3 个模板；单条失败不影响其他组；最近 Runbook 执行最多 5 条 |
| 5.3.3 | 最近执行 | 展示最近任务，点击 `/executions?task_id=` |
| 5.3.4 | Readiness | `/version` 与 `/readyz` 独立加载，展示版本、环境、运行时长和 ok/down/degraded |
| 5.3.5 | summary 整体失败 | 指标显示 `—`，最近执行和推荐为空；当前没有专门错误 Alert |
| 5.3.6 | 刷新 | 并行重取 summary 和健康信息，状态与错误不混淆 |

### 5.4 验证覆盖

- 单元测试覆盖正常聚合和 Alert 块失败；Handler 覆盖未登录、权限不足和成功响应。
- `e2e-identity-access.ps1` 只以 viewer 调用一次摘要，不逐项验证字段、降级或 UI。
- 当前没有专用 Dashboard E2E 或前端 Vitest；processing_alerts 排序/状态、全部块失败、推荐组装和页面跳转需专项验收。

---

## 5A. Audit 审计中心

**权限**：`app:audits:read`（migration 0009，默认绑定 admin）
**API**：`GET /api/audits`；契约：`ops/audit-contract.md`

### 5A.1 查询与权限

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 5A.1.1 | 认证与授权 | 分别使用无 Token、无审计权限和 admin 请求列表 | 分别为 401、403 和成功；migration 0009 只默认绑定 admin，其他角色需单独授权 |
| 5A.1.2 | 精确筛选 | 分别传 resource_type/resource_id/user_id/action，另试资源 ID 子串 | 条件均按相等匹配；子串不会变成模糊搜索，不能相信输入框提示而误判 |
| 5A.1.3 | Trace 筛选 | 对同一请求的多个写操作使用 trace_id 查询 | 精确匹配 `payload.trace_id`，与本次响应顶层 trace_id 区分；无 Payload Trace 的记录不被误命中 |
| 5A.1.4 | 分页与排序 | 构造多页数据，传 page_size=1000 并检查时间相同记录 | 每页最多 100 条，按 created_at DESC、数据库 id DESC 排序；`total` 是筛选总数，`items` 只含当前页 |
| 5A.1.5 | 响应字段 | 检查普通、系统/匿名及空 IP/UA 记录 | 审计 id 为 UUID，created_at 为 Unix 秒，payload 为对象；空 user_id/ip/user_agent 可省略 |
| 5A.1.6 | 数据范围 | 给非 all 数据范围角色只授 app:audits:read 后查询跨团队记录 | 当前只做静态 RBAC，不按业务对象数据范围过滤；不得把 Alert/Asset 的专用校验套用到 Audit |

### 5A.2 写入、认证审计与完整性

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 5A.2.1 | 真实资源目录 | 按 `application/resource/match_rule/resource_relation/asset_sync_batch`、`identity_user/identity_role`、`integration_account`、`observability_query`、`inspection_*` 等实际类型查询 | 能按对应模块真实类型和 action 检索；完整目录以 Audit 契约为准，不使用 `asset` 等臆造类型替代 |
| 5A.2.2 | best-effort | 让审计仓储写失败后分别执行业务写操作和 Alert CES 轮次 | 按当前主要调用点，业务结果不被审计失败回滚；CES 先提交同步状态再独立写审计，不是同事务 |
| 5A.2.3 | IP/UA 与 Payload | 检查标准模块 recorder 产生的记录，并核对敏感值 | 当前标准 recorder 不传 IP/UA，字段通常为空；服务端无通用 Payload 脱敏，写入方不得保存凭据、Token、完整 Prompt 或原始敏感日志 |
| 5A.2.4 | 认证审计分离 | 完成本地/外部登录、refresh、logout 后分别查两类接口 | `/api/identity/admin/auth-audits` 可查认证事件；`/api/audits` 不包含 login/refresh/logout，当前 `/audits` 页面也不是统一认证审计视图 |
| 5A.2.5 | 完整性边界 | 检查可用 API、表结构和保留机制 | 无修改/删除操作审计的 HTTP API；同时没有哈希链、WORM、签名、保留或归档策略，不宣称存储级不可篡改或永久保留 |

### 5A.3 前端明细、聚合与导出

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 5A.3.1 | 明细与详情 | 组合筛选、翻页、点击资源/用户/Trace，打开详情并复制 Trace | 页面通过 `audit.ts` 精确查询，详情展示结构化 Payload；敏感键掩码只影响显示，不能据此推断 API/数据库已脱敏 |
| 5A.3.2 | 指标卡口径 | 在筛选总数大于当前页条数时比较四张卡 | 总卡使用筛选 `total`；Alert/Execution/AI 卡只统计当前页，数值口径不同，不要求分类卡总和等于 total |
| 5A.3.3 | 三类聚合 | 构造跨页同资源/用户/Trace 数据并切换页码和 page_size | Resource/User/Trace 只聚合当前页；跨页记录不会自动合并，不能当作服务端全量统计 |
| 5A.3.4 | NO_TRACE | 混合有 Trace 和无 Trace 的当前页记录 | 无 Trace 记录进入本地 `NO_TRACE` 占位组；该值不是真实 Trace，不应作为链路查询依据 |
| 5A.3.5 | CSV 上限 | 准备超过 100 条同筛选记录并导出 | 文件带 UTF-8 BOM，实际最多包含第一页 100 条；页面成功条数不等于筛选 total，无全量导出保证 |
| 5A.3.6 | 类型快捷选项差距 | 分别用页面下拉和 API 查询 Asset、Identity、Integration、Observability、Inspection 记录 | 页面 `asset/identity/integration/observability/inspection` 与真实类型不一致且缺少多个真实选项；API 可按真实类型查询，前端整改前不记该快捷项通过 |
| 5A.3.7 | 权限体验 | 无权限用户直接访问 `/audits` | 页面路由可进入但后端返回 403 并提示，不出现越权数据；侧栏为静态配置，不代表动态菜单已落地 |

### 5A.4 验证覆盖

- `operation_service_test.go` 只覆盖 Record、自动 Trace 注入和 Trace 查询命中/未命中；当前没有 Audit Handler、真实 GORM 仓储、分页上限或 401/403 专项测试。
- `e2e-identity-access.ps1`、`e2e-alert-sync.ps1` 和多个 `e2e-ai-*.ps1` 含跨模块审计断言，但没有专用 `e2e-audit.ps1`，不覆盖上述全部查询和页面边界。
- 当前没有 Audit 前端 Vitest；当前页聚合、分类卡、`NO_TRACE`、类型选项、CSV 100 条上限及脱敏展示需要专项验收。

---

## 6. Identity 认证与权限校验

### 6.1 权限矩阵（admin 应具备）

执行到目标迁移版本后再核对权限；只跑到早期版本不代表后续权限种子已齐全。

| 资源 | 权限 code | 典型 API |
|------|-----------|----------|
| identity.profile | `app:identity.profile:read`、`app:identity.profile.roles:read` | `/api/identity/me*` |
| identity.roles / permissions | `app:identity.roles:read`、`app:identity.permissions:read` | `/api/identity/roles`、`/permissions` |
| identity.users | `app:identity.users:read`、`app:identity.users:create` | `/api/identity/admin/users*` |
| identity.external_identities | `app:identity.external_identities:create` | 外部身份预置与 LDAP/AD 导入 |
| identity.data_scopes / ai_tool_permissions | `app:identity.data_scopes:read`、`app:identity.ai_tool_permissions:read` | 数据范围与 AI 工具权限字典、角色绑定查询 |
| identity.access_control | `app:identity.access_control:write` | 用户角色及三类角色授权 PUT |
| identity.auth_audits | `app:identity.auth_audits:read` | `/api/identity/admin/auth-audits` |
| identity.authorization | `app:identity.authorization:execute` | `/api/identity/authorize` |
| alerts | `app:alerts:read/acknowledge/update/close/silence/ingest` | `/api/alerts/*` |
| assets | `app:assets:read/write` | `/api/assets/*` |
| runbooks | `app:runbooks:read/create/update/delete` | `/api/runbooks/*` |
| executions | `app:executions:read/create/confirm/execute` | `/api/executions/*` |
| dashboard | `app:dashboard:read` | `/api/dashboard/summary` |
| audits | `app:audits:read` | `/api/audits` |

### 6.2 自动化脚本的实际覆盖

`e2e-identity-access.ps1` 只证明以下主链路，不得扩大为全部 Identity E2E：

| 覆盖项 | 预期 |
| --- | --- |
| admin 登录、创建本地用户 | 返回业务用户 ID，不返回密码哈希 |
| viewer 查询与 manual 绑定 | 绑定来源为 `manual` |
| viewer 登录与 Dashboard 读取 | 请求成功 |
| viewer 越权修改用户角色 | `403 PERMISSION_DENIED` |
| 用户角色审计 | 可查询 `identity_user / set_user_roles` |

该脚本不覆盖 refresh/logout、Redis JTI 轮换/吊销、LDAP/OAuth、认证审计查询、角色权限/数据范围/AI 工具权限写接口和前端 UI。这些项目按 6.3、6.4 单独验收。

### 6.3 负向与运行时用例

| # | 场景 | 操作 | 预期 |
|---|------|------|------|
| 6.3.1 | 未登录 | 无 Bearer 访问受保护 API | `401 UNAUTHENTICATED` |
| 6.3.2 | 无权限用户 | 登录后访问 `GET /api/dashboard/summary` | `403 PERMISSION_DENIED` |
| 6.3.3 | Webhook 无 Token | ingest 不带 token | 拒绝，不创建告警 |
| 6.3.4 | 越权写 | 无 `assets:write` 创建资源 | `403` |
| 6.3.5 | 越权执行 | 无 `executions:execute` 触发 execute | `403` |
| 6.3.6 | 统一认证失败 | 分别使用不存在用户、错误密码、disabled/locked 用户登录 | 均不泄露账号存在性，按认证失败处理 |
| 6.3.7 | 登录限流/IP 白名单 | 超过限流阈值；从白名单外 IP 调用登录/refresh | 分别 `429 RESOURCE_EXHAUSTED`、`403 PERMISSION_DENIED` |
| 6.3.8 | 未预置外部身份 | 即使用户名与本地账号相同，也使用未绑定 external_subject 登录 | 不自动开户、不自动绑定、不签发 token |

### 6.4 会话、外部身份与授权管理

| # | 检查项 | 操作与预期 |
| --- | --- | --- |
| 6.4.1 | Redis refresh 轮换 | refresh 后旧 JTI 失效、新 TokenPair 可用；dev 无 Redis 的 Noop 结果不计入此项 |
| 6.4.2 | Logout | 当前 refresh token 不可再换发；既有 access token 是否仍可用按 TTL 边界说明，不误判为即时吊销 |
| 6.4.3 | LDAP/AD 导入 | 连接、OU 浏览、预览、勾选/整 OU 导入、角色绑定和逐条结果符合契约；`attr_subject` 与登录配置一致 |
| 6.4.4 | OAuth state | state 与 provider/IP/UA 绑定且只能消费一次；错误 state 不换发 token |
| 6.4.5 | 用户角色来源 | PUT 只替换 manual；ldap_import、external_group 绑定保留且不重复写成 manual |
| 6.4.6 | 三类角色授权 | 权限、数据范围、AI 工具权限均可全量替换和清空；无效引用返回 `NOT_FOUND`，对应审计动作可查询 |
| 6.4.7 | 数据范围调用点 | 分别验证动态对象校验和查询过滤；不得仅凭绑定数据范围就认定所有列表已隔离 |
| 6.4.8 | 认证审计 | 本地/外部/OAuth/refresh/logout 成功与失败事件可按用户、provider、event、result 分页查询 |

### 6.5 前端路由与页面

| # | 检查项 | 当前预期 |
|---|--------|----------|
| 6.5.1 | 未登录直链 | 重定向 `/login` |
| 6.5.2 | 无权限直链 | API 返回 403，权限管理页保留结构、提示并禁用操作，不白屏 |
| 6.5.3 | 静态菜单边界 | 当前无权限入口仍可能显示；动态隐藏菜单属于未落地能力，不能按“菜单已隐藏”验收 |
| 6.5.4 | 前端 Token 存储 | 当前 access/refresh token 会进入 localStorage，与项目安全约束存在差距，应记录为整改项而非通过项 |

---

## 7. 推荐验收顺序（自动化）

按依赖关系依次执行，全部 `PASS` 即主链路通过：

```powershell
# 1. 环境
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
go run ./cmd/migrate

# 2. 告警主链路
.\scripts\e2e-alert.ps1

# 2.1 CES 告警拉取（fake provider，migration 0046；非生产环境才允许 auth_type=none）
.\scripts\e2e-alert-sync.ps1

# 3. 资产匹配（独立 RunId，可重复跑）
.\scripts\e2e-asset.ps1

# 3.1 云资源同步（fake provider，migration 0023）
.\scripts\e2e-asset-sync.ps1

# 3.2 真实 CES 账号对账（可选，不进 CI；需真实 AK/SK 与 CES 只读权限）
#     用于核对平台同步数量与 CES 控制台"全部资源"总数，验证 hybrid 增强是否命中。
#     hybrid 只要任一增强失败，批次状态就必须为 partial，且 summary 需要带 enrichment_failed_count。
#     推荐通过环境变量提供凭据，或运行脚本后按提示交互输入 SecretKey，避免密钥进入命令行历史。
#     $env:HUAWEI_ACCESS_KEY="AKXXX"; $env:HUAWEI_SECRET_KEY="SKXXX"
#     $env:HUAWEI_PROJECT_ID="0xxx"; $env:HUAWEI_REGIONS="cn-south-1"
#     .\scripts\e2e-asset-sync-real.ps1

# 4. Runbook 推荐 + 多步执行 + 时间线回写
.\scripts\e2e-runbook.ps1

# 5. 简单执行任务（可选）
.\scripts\e2e-execution.ps1 -AlertId <processing_alert_id>

# 6. Identity：本地用户、viewer manual 角色绑定、403 与 set_user_roles 审计
.\scripts\e2e-identity-access.ps1

# 7. AI 运维助手（migration 0049 / 0050；建议先配置类型 b provider）
.\scripts\e2e-ai-assistant.ps1

# 7.1 AI 相似案例记忆（migration 0051；需要一个未降级的模型回答）
.\scripts\e2e-ai-memory.ps1

# 7.2 AI 知识管理（migration 0052 / 0054；脚本自带本地 mock 模型，需 dev 环境）
.\scripts\e2e-ai-knowledge.ps1

# 7.3 AI 流式对话与取消（migration 0049 / 0050；脚本自带本地 mock 模型，需 dev 环境）
.\scripts\e2e-ai-stream.ps1

# 7.4 AI Agent Loop（migration 0049 / 0056；需要一条 active 告警，脚本自带本地 mock 模型；
#     同时覆盖 accounts.list + resources.list 上下文发现分支）
.\scripts\e2e-ai-agent-loop.ps1

# 7.5 AI 规则体检（migration 0055/0062；脚本自带本地 mock 模型和 fake huawei 账号，批量体检由单测覆盖）
.\scripts\e2e-ai-rules.ps1

# 7.6 AI 证据快照（migration 0060；本地 mock + Docker PostgreSQL，比对模型输入与 payload hash）
.\scripts\e2e-ai-evidence.ps1

# 7.7 RDS for PostgreSQL 历史会话（migration 0061；传长事务告警 ID，使用现有真实云账号）
.\scripts\e2e-postgres-history.ps1 -AlertId <long-transaction-alert-id>

# 8. Dashboard / Audit API 抽检
# curl -H "Authorization: Bearer <token>" http://127.0.0.1:8080/api/dashboard/summary
# curl -H "Authorization: Bearer <token>" "http://127.0.0.1:8080/api/audits?page=1&page_size=100&resource_type=alert"
# Audit 无专用 E2E；跨页聚合、类型快捷选项和 CSV 100 条上限按 5A 专项验收
```

---

## 8. 验收结论模板

| 模块 | API | E2E 脚本 | UI | 权限 | 结论 |
|------|-----|----------|-----|------|------|
| 告警接入 | ☐ | `e2e-alert.ps1` ☐ | ☐ | ☐ | ☐ 通过 / ☐ 阻塞 |
| 资产匹配 | ☐ | `e2e-asset.ps1` ☐ | ☐ | ☐ | ☐ |
| Runbook 推荐 | ☐ | `e2e-runbook.ps1` ☐ | ☐ | ☐ | ☐ |
| 执行记录 | ☐ | `e2e-runbook.ps1` ☐ | ☐ | ☐ | ☐ |
| Dashboard 摘要 | ☐ | 手工/API ☐ | ☐ | ☐ | ☐ |
| Identity 认证与权限 | ☐ | 用户角色主链路 `e2e-identity-access.ps1` ☐；其余专项 ☐ | ☐ | ☐ | ☐ |
| AI 运维助手 | ☐ | `e2e-ai-assistant.ps1` / `e2e-ai-memory.ps1` / `e2e-ai-knowledge.ps1` / `e2e-ai-stream.ps1` / `e2e-ai-agent-loop.ps1` / `e2e-ai-rules.ps1` / `e2e-ai-evidence.ps1` / `e2e-postgres-history.ps1` ☐ | ☐ | ☐ | ☐ |
| Audit 审计中心 | 精确筛选/分页 ☐ | 跨模块审计断言 ☐；无专用 E2E | 明细/当前页聚合/CSV 上限 ☐ | 401/403/静态 RBAC ☐ | ☐ |
| 权限校验 | ☐ | 负向用例 ☐ | ☐ | ☐ | ☐ |

**签字**：验收人 ______　日期 ______　环境 ______（dev/staging/prod）

---

## 9. 云厂商只读接管与观测智能体验收（P1+）

本节用于 P1+ 收口验收，不影响 P0 闭环验收。验收前建议先读 `docs/AI运维平台整体流程与调用关系.md`，确认账号接入、观测查询、巡检证据、建议转执行与审计之间的调用关系。

### 9.1 Integration 接入账号

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.1.1 | Provider/auth 与创建 | 分别创建华为 AK/SK、agency、SigNoz api_token、Prometheus 非 none，并试不兼容组合 | 合法组合返回 `acc-...` 和 `has_credential=true`，不返回 credential/密文/指纹/ref；缺字段或不兼容组合为 400，custom provider 被拒绝 |
| 9.1.2 | 字段级检查 | 对各 Provider 调 `POST .../:account_id/check`，再构造缺 region/token/base_url 场景 | 当前不访问外部服务；ok 只表示字段通过。checker 失败结果落库后 HTTP 仍可为 200 + OK，必须看 `data.status=failed` 和脱敏 message；disabled 为 412 |
| 9.1.3 | 权限与数据范围 | 分别缺少 read/create/update/delete/check 调对应接口，并用非 all 数据范围账号列表 | 对应 401/403 语义正确；五个权限独立。当前只有静态 RBAC，不按 owner_team/region 自动过滤 |
| 9.1.4 | 审计 | 创建/更新/删除/检查后查 `resource_type=integration_account` | 可查同名 action，Payload 无凭据；审计在业务事务后 best-effort，模拟写失败不回滚业务，业务成功不等于审计必达 |
| 9.1.5 | 更新与凭据保留 | 省略 credential、传空对象、替换凭据，并切换到 none | 前两者保留，非空替换；无清空语义，切 none 不删除旧密文。provider 更新会校验凭据并刷新默认 capabilities，页面则禁用 provider 编辑 |
| 9.1.6 | 禁用、删除与事务 | 先 enabled=false 调检查，再 DELETE 并查详情、列表、密文和历史 | 禁用保留凭据且检查 412；删除将账号软删除并 enabled=false、密文硬删除，详情 404、列表排除；capabilities/检查历史保留，无恢复接口 |
| 9.1.7 | 列表与详情 | 使用 provider、enabled、keyword 和分页查询 | keyword 对 name/account_id/owner_team ILIKE；每页最多 100，created_at DESC；返回当前 capabilities 和最近 last_check_status，无检查历史 API |
| 9.1.8 | capabilities 口径 | 新建及分别执行 ok/failed 检查后核对 capabilities | 三类 Provider 默认值符合契约；空结果会补默认声明，failed 也可能保留默认声明，不能记为在线能力实测通过 |
| 9.1.9 | extra_config 与历史默认 | 新建省略/传 `{}`、编辑保留或清空、输入敏感键/非法 region_projects；核对 0024/0029 | 新华为默认 ces，其他为 `{}`；更新省略保留、`{}` 恢复默认；敏感键和非法映射拒绝。历史空配置华为由 0029 回填 native，不与新账号混淆 |
| 9.1.10 | 前端边界 | 检查筛选、编辑凭据、agency、检查失败提示和同步按钮 | 页面无关键字/agency 凭据字段/检查历史/凭据清空；provider 编辑禁用。status=failed 仍显示成功 Toast，必须以 status 为准；同步按钮由 Asset 后端再校验 |
| 9.1.11 | 密钥与外部引用 | 使用正确/错误版本解密，检查 external_ref 和轮换路径 | AES-GCM 往返正确，版本不匹配拒绝；当前只有当前密钥，无多版本解密、外部 Secret Manager 或在线重加密流程，不能只改 key/version 认定轮换完成 |
| 9.1.12 | 前端专项 | 运行华为配置纯函数及页面交互验收 | region/region_projects 解析、转义、未知 key 保留正确；现有 Vitest 不覆盖完整页面、agency、检查失败 Toast 或凭据生命周期 |

`e2e-integration.ps1` 只使用 `huawei_cloud + auth_type=none` 覆盖创建、列表/详情、检查 ok、更新、viewer 创建 403 和删除。它不覆盖真实凭据或网络、AK/SK/agency/api_token、检查 failed、keyword、审计断言、软删除密文核查或完整 UI；脚本 PASS 不能替代 9.1.1–9.1.12 的专项验收。

### 9.2 Observability 查询与证据账本

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.2.1 | 权限与账号解析 | 无 Token/无权限调用四类接口；使用 deleted/disabled 账号、provider 不匹配和非 all 数据范围账号 | 未认证 401、无权限 403；删除 404、禁用 412、provider mismatch 400。公共接口只有静态 RBAC，不按 owner_team/region 自动隔离 |
| 9.2.2 | 指标通用约束 | 调整 from/to/period，尝试超 7 天、超 1440 点，并在公共请求传 project_id | 公共窗口最长 7 天，period 默认 60、平台最小 10，超限 400；当前 Handler 不接收 project_id，多项目只能由账号或 region_projects 解析，不能按旧示例宣称逐请求覆盖 |
| 9.2.3 | 真实 CES 指标约束 | 使用 AK/SK 账号分别测试未知 region、缺 namespace/dimensions、超过 4 维、period=10 及合法 CES 枚举 | 非法输入返回 400 且不发云端请求；真实 period 仅 CES 枚举，filter 仅 average/avg/max/min/sum/variance/var。unit 优先 Provider 返回，未登记推断为空，不臆造 |
| 9.2.4 | Provider 支持矩阵 | 分别用华为 none/AK/SK/agency、SigNoz、Prometheus 调 metrics/logs/traces/topology | 华为 none 全 fake；AK/SK 指标真实、其余三类 unsupported；agency 不降级 fake；SigNoz/Prometheus 当前均为 fake，不能将样本当真实外部数据 |
| 9.2.5 | 日志与证据脱敏 | fake 查询带 keyword，分别传默认/limit=500/501 | 默认 100、最大 500，超限 400；响应为摘要，evidence.summary 只含 has_keyword/keyword_len/keyword_hash，无 keyword 原文；真实华为日志未实现 |
| 9.2.6 | 链路查询 | 传 service/operation/trace_id/error_only/min_latency，limit 分别取默认/1000/1001 | 默认 50、最大 1000，超限 400；fake error_only 只生成模拟错误 Span，不记为真实 APM 验证 |
| 9.2.7 | 运行时拓扑 | 省略 account_id 后查询，再传完整参数并对比 CMDB 拓扑 | account_id/from/to 必填；返回 nodes/edges/error_rate/P95/evidence_id，无 P99；与 `/assets/topology` 的注册表关系图不混用 |
| 9.2.8 | 普通证据落库 | 分别查询 Metrics 和 Logs，检查 obs_evidence_ref，并模拟证据 Create 与审计失败 | 普通 Metrics 有 query_params/window/collected_at 但 payload=NULL，Logs 以 summary 为主；生产证据保存失败终止查询，审计失败 best-effort，不混淆两种失败策略 |
| 9.2.9 | AI 快照 fail-closed | 对 SnapshotRequired 指标检查 collecting → SaveSnapshot → 模型输入，并模拟存储失败 | 只有定稿保存成功才转 ok 并作为支持性证据；OK 快照失败会清 metrics/evidence_id，非 OK 可留失败追踪但不能作为支持引用。普通 evidence_id 不代表存在 payload |
| 9.2.10 | 审计动作 | 完成四类公共查询及内部 resources/alerts 调用后查操作审计 | `observability_query` 包含 metrics_query/logs_search/traces_query/topology_get/resources_list/alerts_list，resource_id 为 evidence_id，Payload 无凭据和原始云端响应 |
| 9.2.11 | 前端边界 | 检查账号加载、指标多序列、日志/链路 limit、拓扑和证据 ID | 只取前 100 个 enabled 账号，指标只显示首条序列，Logs=100/Traces=50；无证据详情抽屉。现有 Vitest 只覆盖指标参数和 AI 上下文 |
| 9.2.12 | 无公共内部能力路由 | 请求 resources、alert rules、RDS 历史和 evidence 详情的推测路径 | 这些属于内部 Port/AI/Asset 能力，不存在对应公共 Observability 路由；不能用 QueryService 方法存在推断 HTTP 已完成 |

`e2e-observability.ps1` 只覆盖 `huawei_cloud + none` 的四类 fake 查询与 evidence_id，不覆盖真实 CES、权限负向、审计、证据落库失败或详情。`e2e-ai-evidence.ps1` 使用 fake 指标 + 本地 mock 模型 + Docker PostgreSQL，验证模型输入与定稿 payload 的规范化 SHA-256 一致，也不是实际 CES 验收。真实指标需使用独立只读账号单独验收，当前没有专用真实指标 E2E 脚本。

### 9.3 Inspection 巡检中心

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.3.1 | 策略校验与元数据 | 创建缺少 account_id/checks、未知 check、未知 application_id 和合法策略 | 非法输入 400；合法策略 enabled 默认 true、agent_profile=sre_default。schedule/agent_profile/notification_policy_id 仅保存，不宣称定时、AI 或通知已生效 |
| 9.3.2 | 策略 CRUD 与权限 | 验证列表/详情/部分更新/启停/删除，以及 read/write 越权 | getPolicy 后端存在；checks 省略保留、空数组拒绝，disabled 触发 412。删除软删除并清 application_ids，历史运行保留；静态 RBAC 无 scope 数据范围过滤 |
| 9.3.3 | 同步运行终态 | 手动触发并立即检查响应与 Run 详情 | 同一请求完成采集分析，通常返回 success/partial/failed 而非 pending；全部证据失败也可 HTTP 200 + status=failed，必须检查 run.status |
| 9.3.4 | partial/failed 与事务 | 分别让部分和全部 checks 失败，另模拟分析/结果保存失败 | 部分成功为 partial 并保留证据；全部失败为 failed。Findings/Recs/终态 Run 同事务，启动和审计不在该事务；失败时尽量写 failed 并返回相应结果/错误 |
| 9.3.5 | scope 与真实云限制 | 检查采集 region/namespace/dimensions，使用真实 AK/SK 和 fake 分别运行 | 当前固定最近 1h、cn-north-4、SYS.ECS、无 dimensions，真实华为指标失败，logs/traces 也不支持；fake 主路径不代表真实云巡检完成 |
| 9.3.6 | 规则与 Finding | 构造各阈值上下边界和未命中数据 | CPU 80/60、内存 75、磁盘 70、错误 Span、延迟样本、error>=2 规则符合实现；confidence/uncertainty 为规则常量，未命中不建 Finding，affected_resources 仍为固定 payment-service |
| 9.3.7 | Finding/Recommendation 状态 | 按 run_id/policy_id/risk_level 查询并检查建议状态 | 返回 evidence_refs 和嵌套建议；初始 open，创建任务后 execution_created。accepted/dismissed 无 API，Execution 完成不回写 |
| 9.3.8 | 转执行准入 | 对不可执行/已创建建议及不同 mode/medium/Spec 参数调转执行 | 不可执行 412、重复 409；只允许 agent，medium_id 必填，无自动介体；confirm_intent 只能 create_task_only，Spec/arguments 由 Execution 校验，中高风险 pending_confirm |
| 9.3.9 | 默认 Spec 与事务边界 | 省略 command_spec_id/arguments，另模拟任务创建后 Recommendation 更新失败 | 标题选择默认只读 Spec，未命中兜底磁盘，磁盘空参 mount_point=/，需人工复核；Recommendation 不存 task_id，创建与状态更新非事务，重试前核对 Execution/审计避免重复 |
| 9.3.10 | 审计与 AI 工具边界 | 查策略/运行/建议审计及 AI 工具注册 | 实际为 policy create/update/delete/enable/disable、run create/start/finish、recommendation create/create_execution，best-effort；cancel/notify 未实现。0020 种子不代表已有平台内置工具；通用 Provider 转发不等于调用 Inspection 用例 |
| 9.3.11 | 前端边界 | 检查策略、运行和发现三个 Tab 以及失败运行提示 | 有新建/删除/立即巡检、运行列表和发现筛选；无编辑/启停/时间线/取消/通知/转执行，无前端 Vitest。failed 也显示成功 Toast，需以 status 为准 |
| 9.3.12 | E2E 覆盖边界 | 运行并核对 `e2e-inspection.ps1` 与 `e2e-execution-agent.ps1` | 前者仅 fake 下创建应用/账号/策略、同步运行、Run 详情、Findings 和清理；后者直接创建 manual Agent 任务，不经过 Recommendation。两者不覆盖真实云、转执行、审计或完整 UI |

RecommendationService 当前没有独立单元测试，现有 Run/Analyzer 测试也不覆盖全部阈值、partial/failed 和转执行异常；上述项目需按实际结果单独记录，不能因 fake E2E PASS 自动记为通过。

### 9.4 推荐自动化脚本

```powershell
.\scripts\e2e-integration.ps1
.\scripts\e2e-observability.ps1
.\scripts\e2e-inspection.ps1
.\scripts\e2e-execution-agent.ps1
.\scripts\e2e-execution-agent-permission.ps1
```

> Notification 模块与 `scripts/e2e-notification.ps1` 暂未落地，不纳入当前 P1+ 自动化验收。

### 9.5 Execution Agent 服务端控制面验收

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.5.1 | 创建/查询 Medium | `POST/GET /api/executions/media` | 权限为 media:create/read，不返回登录凭据；无 update/disable/delete 管理路由 |
| 9.5.2 | Command Spec 查询 | `GET /api/executions/command-specs*` | 返回种子只读规格；无创建/更新/禁用管理路由 |
| 9.5.3 | 注册 fake Agent | 静态 X-Register-Token 调 `/agents/register` | 返回随机 Agent Token，数据库只存 hash；注册令牌可复用，不是一次性；当前无 mTLS |
| 9.5.4 | 心跳 | Agent Token 上报 online/running_tasks/free_slots | 只有 online 可 lease；无自动离线 worker 或管理禁用接口 |
| 9.5.5 | 创建受控命令任务 | 顶层 execution_mode=agent、medium_id、command_spec_id、arguments | 显式字段必填，校验 Medium/Spec/schema/argv/风险；无自动介体选择，不展开 Runbook |
| 9.5.6 | 未确认不可领取 | fake Agent 领取 pending_confirm 任务 | 返回空任务，不提前执行 |
| 9.5.7 | 确认后领取 | CONFIRM 后 fake Agent lease | 返回 lease_id 和受控 argv；当前无独立 redaction 字段、续租或过期重派 |
| 9.5.8 | 日志写入与读取 | fake Agent 回传 stdout，再调用户侧日志 API | 服务端双重脱敏，查询需 step_id；当前 E2E 只写不读，读取/脱敏需专项断言 |
| 9.5.9 | 结果与幂等 | fake Agent 回传 exit_code/result，再重复请求 | 首次更新 step/task 并释放租约；终态重复请求返回当前详情；当前 E2E 未验证幂等 |
| 9.5.10 | 参数校验 | 提交不符合 schema 或含 shell 元字符的 arguments | `INVALID_ARGUMENT`，不创建任务 |
| 9.5.11 | 审计 | 查询 register/heartbeat/dispatch/agent_log/agent_result | 业务摘要可追踪，敏感 Token/日志不进入审计；当前 E2E 未断言 |
| 9.5.12 | Alert 时间线差距 | 使用 alert 来源 Agent 任务检查时间线 | 当前创建/lease/result 不写 execution_created/started/finished，不按已闭环验收 |
| 9.5.13 | 前端与独立 Agent | 检查 `/executions` 和 cmd | 无 Agent 管理/创建/日志 UI，无 cmd/execution-agent；PowerShell fake 不是实际命令执行 |

推荐脚本及真实覆盖：

```powershell
.\scripts\e2e-execution-agent.ps1
# Medium 创建、静态 token 注册、心跳、manual high 任务、确认前后 lease、单条 stdout、success result、非法参数

.\scripts\e2e-execution-agent-permission.ps1
# 实际只验证错误注册令牌 PERMISSION_DENIED，不覆盖低权限 Medium/Execution 403
```

真实 Agent、自动选择、Runbook→Agent、日志读取/脱敏、审计、幂等、禁用、Alert 时间线和前端不得因上述脚本 PASS 自动记为通过。

### 9.6 AI 告警分析指标证据

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.6.1 | 证据采集成功 | AKSK CES 账号（含 `metrics` 能力）触发带结构化条件的告警，调用 `POST /api/ai/analyze-alert` 且 `include_metrics=true` | provider payload 含 `evidence.status=ok`、指标统计与 `evidence_id`；审计含 `evidence_status=ok` 和 `evidence_ids` |
| 9.6.2 | 无指标自然跳过 | 事件类告警或无维度/指标告警勾选包含指标 | `evidence.status=skipped`，`reason` 为 `no_metric`/`no_dimensions` 等，分析仍返回 |
| 9.6.3 | 数据权限降级 | 无绑定账号数据范围的用户发起分析 | `evidence.status=permission_denied`，分析不中断，审计记录该状态 |
| 9.6.4 | 上游/配置降级 | 停用 provider 或使用 agency 账号 | 分析仍返回本地摘要；`evidence.status=error/unsupported`，无凭据或底层原始错误泄露 |
| 9.6.5 | 快照与模型一致（migration 0060） | 运行 `.\scripts\e2e-ai-evidence.ps1`（fake 指标 + 本地 mock 模型 + Docker PostgreSQL） | 数据库 `payload/query_params` 含定稿数据点与查询窗口；模型观测和 payload 的规范化 SHA-256 相同，且等于 `payload_hash`；不代表真实 CES 通过 |
| 9.6.6 | 降采样与截断计数 | 构造单序列 >288 点的对话指标查询；运行 `go test ./internal/ai/application -run TestFinalizeMetricEvidence` | 模型与数据库共享同一份定稿序列；`original_count=原始点数`、`retained_count<=288`、`truncated=true`、`sampling.strategy=stride_first_last`，首末点保留；峰值时刻与破限片段/时长在降采样前按全量序列计算，裁剪后仍保留 |
| 9.6.7 | 快照写失败 fail-closed | 运行 `go test ./internal/ai/application -run 'TestFinalizeMetricEvidenceFailsClosedWhenPersistFails|TestFinalizeMetricEvidenceRequiresSnapshotStore'` | 返回 `error` 与稳定 reason，指标及支持性引用被移除，不把未持久化数据交给模型作事实 |
| 9.6.8 | 失败证据不作为支持引用 | 触发账号数据范围拒绝并检查工具观测、审计与最终 references | 失败快照可有 `evidence_id` 供追踪，但最终支持性 `observability_evidence` 引用不包含该 ID，指标为空 |
| 9.6.9 | 普通证据与详情边界 | 对比普通 Observability Metrics、AI 定稿记录和现有前端/API | 普通 Metrics 可有 query_params/window 但无 payload；AI 定稿才有模型输入 hash。当前无公共证据详情 API/抽屉，不得用 evidence_id 存在推断可在线回放 |

> 9.6.1 需已配置并启用 AI provider 和真实只读账号，当前没有专用真实 CES 指标 E2E；无 provider 时 `include_metrics` 不会发起取数，仅返回本地摘要。9.6.5 需要 migration `0060`、本地 mock 模型和 Docker PostgreSQL，使用 fake 指标；证据详情 API / 前端抽屉不在当前实现范围。

### 9.7 AI Provider 与工具网关（migration 0001/0002/0048/0055–0065）

| # | 检查项 | 操作 | 预期 |
| ---|--------|------|------|
| 9.7.1 | 权限与持久化 | 分别以无权限和有权限角色调用 Provider 列表/upsert/delete；新增类型 b 后重启 API | 未认证 401、无权限 403；有权限时写入 `ai_provider`，重启后仍从数据库加载，密钥只显示 has_api_key/“已配置” |
| 9.7.2 | 密钥与全量 upsert | 新增时省略 key；更新时分别省略、空串和全空白 key，再修改名称/模型 | 新增 400；更新保留原密文和 has_api_key=true，不返回 api_key 或独立掩码字段；没有通过空白 key 清空密钥的语义 |
| 9.7.3 | 其他字段覆盖 | 通过 API 配置 headers/enabled/model 后省略这些字段更新；再在页面普通编辑一次 | headers 省略写为空对象，enabled 省略为 false；页面当前不携带 headers，会清空旧自定义头。0048 DB 默认 30000ms 与应用/GORM 300000ms 不可混写 |
| 9.7.4 | 类型与参数校验 | 分别保存类型 a/b/c、缺 model 的 b、非法 reasoning_effort、未知类型 | a/c 需要 base_url/key，b 还需要 model；reasoning 仅 low/high/max。未知类型当前可能落库但调用 503，不能当作已支持协议 |
| 9.7.5 | 运行时与健康边界 | 构造解密失败/不可达但 DNS 合法的 Provider，查看列表并调用；检查页面按钮 | 启动只跳过坏记录，数据库列表仍可 enabled=true；保存不做模型请求。无健康状态、最近错误或测试连接 API/UI，enabled 不等于可用 |
| 9.7.6 | 当前实例与多实例 | 保存、禁用、启用、删除并立即分析；如有多副本，再检查另一实例 | 当前实例无需重启；数据库与内存非统一事务，无跨实例配置广播，不能宣称所有副本即时刷新。删除不存在 ID 当前也可 deleted=true |
| 9.7.7 | 出站安全 | 分别配置非 HTTP(S)、metadata、私网、loopback、白名单外主机、重定向和合法地址 | 非法目标在保存或调用时被拒绝，重定向不跟随；dev/显式开关才允许 loopback。校验通过不证明上游凭据或模型有效 |
| 9.7.8 | Provider 选择 | 配置多个启用的 b/c/a，直接调用、告警分析和对话分别观察 provider_id | 直接调用只用请求 provider_id；分析按 b→c→其他、同类型按 ID；对话只用 b 且默认 ID 最小。没有持久化默认 Provider 字段 |
| 9.7.9 | 直接调用业务拒绝 | 用 mock Provider 调 `/api/ai/tools/invoke`，分别撤销工具权限、传 deny、错误数据范围，再恢复 | 进入 Gateway 后的拒绝为 HTTP 200 + OK + allowed=false/reason，Provider 不被调用；前端 ToolConsole 弹出原因，不能只判断外层 OK |
| 9.7.10 | confirmed 与豁免 | 对 read_only/require_confirm/deny 分别切换 confirmed；检查 permits_unconfirmed_invoke 与现有种子 | read_only 不要求确认，deny 总拒绝；require_confirm 默认需 confirmed，但豁免标志为 true 可放行。execution.runbook、inspection.runs.create、execution.tasks.propose 种子带豁免，不等于绕过 Execution 状态机 |
| 9.7.11 | 直接调用数据范围 | 给测试角色绑定非 all 数据范围后调用 `/tools/invoke`，body 再补 owner/dept/team/region/tags | 路由入口因没有对象上下文直接 403，请求体字段不会先进入 Handler；无范围/all 才进入 Gateway 的二次对象校验 |
| 9.7.12 | 三条工具调用链 | 对比直接调用、analyze-alert 和对话中的 alarm.analyze，再检查 8 个内置工具 | 直接调用和分析会交给 Provider；对话 alarm.analyze 只本地读告警。metrics/postgres/rules/accounts/resources 等 8 个内置工具均由本地 Port 执行，模型补全只用类型 b |
| 9.7.13 | 种子与执行边界 | 查 logs.search、execution.*、inspection.* 权限种子，并在 Agent Loop 申请执行类工具 | 种子不等于平台内置工具；Loop 拒绝执行类工具。通用转发不保证真实查询或 pending_confirm 任务，执行计划仍走既有 Execution 创建接口 |
| 9.7.14 | 审计语义 | 完成 Provider CRUD、直接 allowed=false 调用、对话工具拒绝，再查日志和 `/api/audits` | Provider CRUD 只有应用日志；直接 tool_invoke 不含 allowed/mode/reason且拒绝仍 result=success；对话 tool_invoke 才有完整状态。写入均按 best-effort 边界验收 |
| 9.7.15 | 类型 b 降级与真实模型 | 用 mock 构造 429/5xx/非法 JSON/空响应和正常结构化回答；另用真实模型做凭据/配额/模型能力验收 | 非流式按代码最多 3 次尝试并可返回 data.degraded=true；allowed=true 不代表真实分析成功。mock 通过不替代真实模型、视觉或配额验证 |
| 9.7.16 | 前端与自动化边界 | 检查 ProviderManager、ToolConsole、Identity 工具授权和现有 AI 脚本 | Provider 无筛选/健康/测试连接，ToolConsole 无对象字段且确认提示不是动态规则；固定工具目录非角色实授。三者无专项 Vitest，无直接 invoke/完整 Provider CRUD/角色工具绑定专项 E2E |
| 9.7.17 | 类型 c 编排参考服务（可选） | 手工构建运行 `cmd/ai-orchestrator`（env 见契约 §9.4），注册类型 c provider 后分别做直接调用与告警分析；再停掉其模型依赖 | `X-API-Key` 鉴权失败 401；非 `alarm.analyze` 400；模型失败仍返回 200 保守结果（无 `degraded` 字段，平台审计不记 partial）；长事务 `diagnosis` 仍由平台侧生成；对话不受影响（无类型 b 时 degraded/model_unavailable）。无专项 E2E，需人工验收 |

> 前置：按所验工具执行到对应迁移；0048 只建 Provider 表，不种子 Provider 记录，YAML `ai.providers` 仅在空表时写入。`e2e-ai-mock-provider.ps1` 是共享 fixture，只注册/删除保留 ID 的类型 b mock；其他 `e2e-ai-*.ps1` 主要使用 mock/fake，且部分脚本缺 active 告警会 SKIP 后正常退出。类型 c 编排服务无 compose/K8s 定义与 E2E，9.7.17 需手工构建运行。上述覆盖来自静态核对，执行结果须另记。

### 9.8 AI 运维助手（migration 0049）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.8.1 | 权限种子 | 执行 `go run ./cmd/migrate` | `app:ai:chat`、`app:ai:chat:feedback` 存在且绑定 `admin`；未授权用户调用返回 403 |
| 9.8.2 | 创建会话 | `POST /api/ai/chat/sessions`（带 `alert_id` 上下文） | 返回 `session_id`，`context.alert_id` 回显，审计含 `chat_session` |
| 9.8.3 | 发送消息 | `POST /api/ai/chat/sessions/{id}/messages` | `answer.summary/risk_level/actions` 完整；`status=ok/degraded/insufficient_context` 合法；审计含 `chat_message` 与 `tool_invoke` |
| 9.8.4 | 工具拒绝可见 | 移除某角色的 `alarm.analyze` 或 `metrics.query` 工具权限后提问 | 工具调用 `allowed=false` 且带 `reason`；回答仍返回并展示拒绝原因，不静默降级 |
| 9.8.5 | 降级回答 | 禁用/删除启用中的类型 `b` provider 后提问 | `status=degraded`、`degraded_reason=model_unavailable`；保留已获取的只读上下文和不确定性；action 权限仍单独计算，不把降级当模型成功 |
| 9.8.6 | 会话归属 | 用另一账号 `GET /api/ai/chat/sessions/{id}` | 403 `PERMISSION_DENIED`，不返回其他用户会话内容 |
| 9.8.7 | 反馈 | `POST .../messages/{message_id}/feedback` 提交 `valid` / `invalid` | 会话详情回显 `feedback`；审计含 `chat_feedback`；对用户消息提交反馈返回 400 |
| 9.8.8 | 生成执行计划 | 点击回答中的「生成执行计划」 | 调用 `POST /api/executions/tasks`（`source_type=ai_conversation`），任务为 `pending_confirm`；未确认前不可执行 |
| 9.8.9 | 确认边界 | 执行确认页输入 `CONFIRM` | 任务推进到 `pending_execute` 后才可执行；拒绝/大小写不符返回 400 |
| 9.8.10 | 审计可查 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | 可查 `chat_session` / `chat_message` / `tool_invoke` / `chat_feedback`，payload 无 prompt 与凭据 |
| 9.8.11 | 全局浮层 | 任意页面按 `Alt + A` 或点击顶栏「AI 助手」 | 浮层带入当前页面上下文；快捷问题可发送；只出现「生成执行计划」，无直接执行入口 |
| 9.8.12 | AI 配置归位 | 侧栏「AI 配置」 | Provider 管理与工具调用调试在系统设置配置页；AI 运维助手页只保留对话工作台 |
| 9.8.13 | 告警数据范围（AI） | 建角色仅绑 `team=A` 数据范围并绑定测试账号；用 `team=B` 标签的告警分别调 `POST /api/ai/analyze-alert` 和对话（`alert_id` 为该告警） | 分析返回 403 `PERMISSION_DENIED` 且审计 `result=denied`；对话 `tool_calls` 中 `alarm.analyze` 为 `allowed=false`、`status=denied`、`reason=data_scope_denied`，无告警引用与 `view.alert`，仍返回回答 |
| 9.8.14 | 数据范围放行 | 同一账号改用 `team=A` 标签告警重复 9.8.13 | 分析正常返回；对话 `alarm.analyze` 为 `allowed=true`、`status=ok`，引用含该告警 |
| 9.8.15 | 会话归档 / 恢复 | 会话列表点击「归档」→ 刷新 → 点击「恢复」 | 归档后列表显示「已归档」、历史仍可查看；归档态发消息被拦截（服务端 409，前端提前提示）；恢复后可继续提问；审计含 `chat_session_status`（含 `previous_status`） |
| 9.8.16 | 会话删除 | 会话列表点击「删除」并确认 | 会话与消息一并删除，列表即时移除；删除当前会话自动切到下一个；重复删除返回 404；审计含 `chat_session_delete` |
| 9.8.17 | 消息幂等 | 用同一 `client_message_id` 连续 `POST .../messages` 两次（内容相同） | 两次返回相同 `message_id` 与 `answer`；会话详情只有一组用户/助手消息；审计含 `chat_message` 且第二次 `replayed=true`；第二次不再调用模型（可用 provider 日志或耗时判断） |
| 9.8.18 | 幂等冲突 | 手工插入一条仅含用户消息（无回答）的 `client_message_id`（或中断一次请求后重试同一 id） | 返回 409 `FAILED_PRECONDITION`，提示上一次请求仍在处理或已中断；不新增消息、不调用模型 |
| 9.8.19 | 资源级上下文（资产） | 资产页选中应用并高亮某个资源后打开 AI 助手 | 上下文展示应用/资源 ID；`POST .../messages` 请求体 `context` 含 `application_id`、`resource_id`、`resource_type` |
| 9.8.20 | 资源级上下文（执行 / 观测） | 执行页打开任务详情、观测页执行一次指标查询后分别唤起 AI 助手 | 执行页上下文含 `execution_id`（告警来源另带 `alert_id`、资源目标另带 `resource_id`）；观测页上下文含 `resource_id` 与 `time_range`；关闭详情或查询后字段随页面切换清理 |
| 9.8.21 | 可选模型列表 | 普通账号（仅 `app:ai:chat`）调用 `GET /api/ai/chat/models` | 返回运行时启用中的类型 b 模型数组，含 `provider_id/name/model/vision_enabled`；不含 `base_url/api_key/headers/timeout_ms`；管理员 provider 管理接口权限不因此放开 |
| 9.8.22 | 会话内模型切换 | 新建会话前切换模型并发送首条消息；回答完成后再次切换并发送下一条消息 | 每次请求 `context.provider_id`/顶层 `provider_id` 为当前所选模型，审计实际 `provider_id` 一致；当前生成期间选择器实际未禁用，不能按旧预期验收 |
| 9.8.23 | 模型不可用回退 | 请求已删除/禁用 provider 的 `provider_id`，再测试无任何默认模型 | 有默认时请求不失败并回退，`answer.uncertainties` 首条含回退说明，审计 `provider_fallback=true`；无默认时 degraded/model_unavailable |
| 9.8.24 | 消息复制 | 悬停用户消息气泡；点击 AI 回答底部「复制全文」 | 用户消息复制原文；AI 回答复制 Markdown（摘要/依据/影响/建议/回滚/不确定性）；提示“已复制”；非安全上下文回退成功 |
| 9.8.25 | 历史窗口与事务 | 构造超过 200 条消息，另在模型/助手落库失败后读取会话 | 详情只返回最近 200 条升序，不代表全量；用户与助手消息为两个事务，失败后可保留 user，不能宣称整轮原子提交 |
| 9.8.26 | 幂等键内容边界 | 同一会话用相同 key 改 content/provider_id，再分别对 degraded/incomplete 结果重试 | 均返回首次 message_id/answer，不重新取数或调用模型；不同内容也比对 key 而非 hash。真正的新提问必须使用新 key |
| 9.8.27 | 模型持久化字段 | 已有会话模型的前提下只传顶层 provider_id 换模型，再在下轮省略；随后同时传 context.provider_id 重试 | 只传顶层时仅本轮生效，旧会话模型仍保留；同时传 context 才持久切换。当前页面会同时传两字段 |
| 9.8.28 | 归档与状态审计 | 重复归档/恢复，再在归档会话输入文字并点击发送；最后用相同 key 重放已有回答 | 同状态不推进 updated_at但仍写 chat_session_status；当前输入会被清空，属于已登记缺口；已生成回答在归档后仍可重放，新提问 409 |
| 9.8.29 | 前端重试与刷新 | 对已落库 degraded/incomplete 回答点击重新提问；刷新页面后再次尝试；检查列表刷新按钮 | 旧 key 只重放且本地可能重复消息；历史重试内容被清空，按钮可能不可用；刷新按钮只取列表，无通用当前详情刷新，需导航或重新加载页面 |
| 9.8.30 | 执行提议权限记录 | 检查一次正常回答的 tool_calls、actions、审计和 Execution 列表 | execution.tasks.propose 可作为权限校验出现在 tool_calls/tool_invoke，但不创建任务；只有点击 action 并调 `/api/executions/tasks` 后才产生 pending_confirm 任务 |
| 9.8.31 | 会话脚本边界 | 核对 `e2e-ai-assistant.ps1` 的输入、输出和清理 | 同步 API + mock 主路径可覆盖结构、执行、顺序幂等、反馈、归档/恢复、删除和部分审计；degraded 仍可结构通过，主会话/消息/任务/审计保留，不是 UI 或真实模型验收 |
| 9.8.32 | 浮层入口与上下文联动 | 顶栏「AI 助手」/`Alt + A` 默认打开；告警详情点「问 AI 助手」；在选择器中新建会话并发送 | 默认入口无选择器并自动选中最近会话；告警入口先显示会话选择器，新建会话的 `context` 含 `alert_id`；浮层顶部展示当前页面上下文摘要；跨页路由切换后返回，消息与生成状态保留 |
| 9.8.33 | 排队生命周期 | 会话生成中连续发送第二条并观察队列；分别尝试置顶/删除/清空；点击「停止并清空队列」；刷新页面 | 第二条进入排队列表且首条完成后自动发送；置顶/删除/清空即时生效且只影响当前会话；停止后队列清空且不自动发送；刷新后排队条目与乐观消息丢失，服务端回合按取消/完成落库 |

> 前置：已执行迁移 `0049`、`0050`（0050 为消息幂等键）；9.8.3 的模型回答需要已启用类型 `b` provider。E2E 脚本：`scripts/e2e-ai-assistant.ps1` 对 9.8.2/9.8.3/9.8.7/9.8.8/9.8.9/9.8.10/9.8.15/9.8.16/9.8.17 只有**部分自动断言**：未完整覆盖 chat_session、审计脱敏、replayed=true、不再调用模型、invalid/用户消息反馈、并发幂等、跨用户、模型切换或浏览器 UI；归档 probe 实为 active 会话。9.8.32/9.8.33 为纯前端行为：浮层选择器/上下文联动由 `AIAssistantOverlay.spec.ts` 与 `aiContext.spec.ts` 覆盖，跨会话并发与同会话排队由 `useChat.spec.ts` 覆盖，跨页共享控制器、跨页队列置顶/删除与停止清队由 `BasicLayout.spec.ts` 覆盖；归档排队失败、删除会话清理与快捷键无专项测试，UI 需人工验收。
>
> 数据范围（9.8.13/9.8.14）：告警快照读取按对象数据范围过滤，分析路径与对话路径共用同一入口，映射与拒绝语义见 `ops/ai-contract.md` §10.6.4、§10.11。注意告警列表/详情接口的自身数据范围口径不在 AI 模块范围内。

### 9.9 相似案例记忆（migration 0051，阶段 1）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.9.1 | 有效反馈沉淀 | 对一条 `status=ok` 的回答提交 `valid` 反馈 | `ai_memory_case` 新增一条：`source_type=chat_message`、`source_message_id` 为该回答、`quality_score=1`、`status=active`；重复提交不新增第二条。memory E2E 只间接验证引用，不直接查管理列表或分数，需按本项单独记录 |
| 9.9.2 | 降级回答不入记忆 | 在无可用 provider（`degraded=true`）时提交 `valid` | 不生成案例；反馈接口仍返回 200。memory E2E 在该场景 SKIP，不能自动覆盖 |
| 9.9.3 | 检索注入与引用 | 用同一告警再次提问（或重复调用 `POST /api/ai/analyze-alert`） | 对话 `references` 含 `type=memory_case`、`id=case_id`；provider payload 含 `memory_cases`（分析接口为 `similar_cases`） |
| 9.9.4 | 无效反馈降分与淘汰 | 对同一回答连续提交两次 `invalid` | 第一次 `quality_score=1 → -1` 仍 `active`；第二次 `≤ -2` 转 `retired`；后续检索不再命中 |
| 9.9.5 | 跨用户隔离 | 用另一账号对同一应用/资源提问 | 不返回 A 用户的案例（`memory_status=empty`），日志不含案例摘要 |
| 9.9.6 | 降级不阻断 | 停库或让检索失败后提问（可用错表/断连接复现） | 回答正常返回；审计 `memory_status=error`、`memory_used=0` |
| 9.9.7 | 审计字段 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | `chat_message` payload 含 `memory_used` / `memory_status`，不含案例原文；自动 valid/invalid 的记忆增减不写独立 memory 审计 |
| 9.9.8 | 记忆管理列表 | admin 调用 `GET /api/ai/memory/cases?status=active&keyword=...`；非 admin 调用 | 返回脱敏案例列表（摘要/建议/质量分/状态）；非 admin 返回 403。`session_id/alert_signature/keywords` 为空时省略；admin 可按 user_id 查他人案例，无对象级数据范围 |
| 9.9.9 | 手动停用/启用 | `POST /api/ai/memory/cases/{case_id}/status` 依次提交 `retired`、`active` | 停用后提问不再命中该案例且审计 `memory_review`（`previous_status`/`status`）；启用后重新参与检索，质量分保持；重复提交同状态幂等且不新增审计 |
| 9.9.10 | 前端管理边界 | 检查 MemoryManager 的筛选、分页、详情和二次确认 | 页面只有 status/keyword/user_id，limit=100 且无 application_id/resource_id/offset 分页；详情不展示 session_id；停用有确认、启用没有；无前端 Vitest |

> 前置：已执行迁移 `0051`、`0054`；9.9.3/9.9.4 需要已启用类型 `b` provider。契约见 `ops/ai-contract.md` §10.14、§10.19。E2E 脚本：`scripts/e2e-ai-memory.ps1` 只部分覆盖 9.9.3/9.9.4/9.9.7，首条回答 degraded 会 SKIP 后 exit 0；不调用管理列表、不校验 quality_score/幂等，未覆盖 9.9.1/9.9.2/9.9.5/9.9.6/9.9.8/9.9.9/9.9.10；`scripts/e2e-ai-knowledge.ps1` 额外部分覆盖记忆列表检索与 retire，也不覆盖 403、审计或完整 UI。

### 9.10 知识沉淀（migration 0052，阶段 2）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.10.1 | 权限种子 | `go run ./cmd/migrate` 后查 `iam_permission` | 存在 `app:ai:knowledge:manage` 且绑定 `admin`；非 admin 调用返回 403 |
| 9.10.2 | 新建为草稿 | `POST /api/ai/knowledge/items`（scope=global，title/content） | 返回 `status=draft`、`origin=manual`；审计 `knowledge_edit` |
| 9.10.3 | 草稿不注入 | 用命中该应用/资源类型的会话提问 | system prompt 无「知识沉淀」段落，`references` 无 `knowledge_item`，审计 `knowledge_used=0` |
| 9.10.4 | 审核激活后注入 | `POST .../items/{item_id}/review` `{"action":"activate"}` 后再次提问 | system prompt 末尾出现知识段落（固定安全约束仍在最前），`references` 含 `knowledge_item`；`hit_count` +1；审计 `knowledge_review` |
| 9.10.5 | 编辑回草稿 | `PUT` 修改 active 条目的 title/content | 条目回到 `draft`、`reviewed_by` 清空；再次提问不再注入 |
| 9.10.6 | 停用立即失效 | `review` `{"action":"disable"}` 后提问 | 立即不再注入（无缓存残留） |
| 9.10.7 | 有效反馈计入 useful | 对注入了知识的回答提交 `valid` | 条目 `useful_count` +1；不会自动停用 |
| 9.10.8 | 低效自动停用 | 让某条目被注入 ≥5 次且从未获得 `valid` | 条目转 `disabled`，审计 `knowledge_review` 且 `action=auto_disable`；后续不再注入 |
| 9.10.9 | 审计可查 | `GET /api/audits?resource_type=ai&resource_id={item_id}` | 可查 `knowledge_edit` / `knowledge_review`，payload 含 `action` 与 `status` |
| 9.10.10 | 反馈聚合生成草稿 | 对同一应用积累 ≥3 条 `valid` 反馈后，在「AI 配置 → 知识沉淀」填写应用 ID 点击「反馈聚合生成草稿」 | 返回 `action=created`，生成 `origin=feedback_aggregate`、`status=draft`、`item_id=agg-<app_id>` 的条目；内容含历史案例摘要与建议；审计 `knowledge_edit` 且 `action=aggregate`。UI 不暴露 min_cases，脚本也未断言固定 item_id，需单独核对 |
| 9.10.11 | 聚合幂等与保护 | 再次聚合同一应用；随后把该条目激活并再次聚合 | 未激活时 `action=updated` 且条目仍只有一条；激活后 `action=skipped_active`，线上内容不被覆盖 |
| 9.10.12 | 聚合案例不足 | 对有效案例 <3 条的应用点击聚合 | 返回 `action=insufficient_cases`，不生成草稿，前端提示案例不足；`min_cases<=0` 默认 3，聚合取数最多 20 条 |
| 9.10.13 | 前端与验证边界 | 检查 KnowledgeManager 的展示、确认、聚合和迁移权限 | 页面不展示审核人/审核时间/创建者/时间，无详情弹窗；激活/停用无二次确认；scope_ref 校验依赖后端。无前端 Vitest；knowledge E2E 需要 0052+0054 |

> 前置：已执行迁移 `0052`、`0054`（knowledge E2E 会调用 memory/cases）。契约见 `ops/ai-contract.md` §10.15。管理入口：「系统设置 → AI 配置 → 知识沉淀」页签（列表 / 新建 / 编辑 / 激活 / 停用 / 删除 / 反馈聚合）。E2E 脚本：`scripts/e2e-ai-knowledge.ps1` 部分覆盖 9.10.2/9.10.3/9.10.4/9.10.6/9.10.7/9.10.10 及记忆列表/retire；未覆盖 403、编辑回草稿 9.10.5、低效自动停用 9.10.8、聚合幂等 9.10.11 与案例不足 9.10.12 的完整断言或 UI 9.10.13。

### 9.11 受限 Agent Loop（阶段 3，无新迁移）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.11.1 | 追加只读工具 | 配置一个会在上下文不足时输出 `tool_requests`（`metrics.query`）的模型，提问 | 服务端执行追加取数后二次调用模型；`tool_calls` 含该轮调用；审计 `chat_message` 的 `loop_rounds ≥ 1` |
| 9.11.2 | 执行类工具不可达 | 让模型申请 `execution.command.run` / `execution.tasks.propose`，并核对 Execution 列表 | 该申请 `allowed=false`、`reason=tool is not allowed in agent loop`，不产生执行动作；finalize 阶段另可做执行提议权限校验，二者不得混淆 |
| 9.11.3 | 轮数预算与强制收敛 | 让模型按不同参数持续申请 `metrics.query`，分别让强制轮回答和继续申请工具 | 最多 4 个追加轮后先强制收敛；强制轮给出合法 answer 时可 ok/insufficient_context且 forced_convergence=true，仍失败才 degraded/agent_loop_exhausted，保留观测 |
| 9.11.4 | 无进展收敛 | 让模型持续申请被拒绝的工具 | 连续两轮无实际取数后进入强制收敛，通常为首轮+纠正轮+强制轮；不是“至多两次模型调用”，成功可正常回答，失败才 agent_loop_exhausted |
| 9.11.5 | 审计可回放 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | 每轮工具调用都有 `tool_invoke`；`chat_message` 含 loop_rounds、tokens，强制轮另有 forced_convergence；loop_rounds 不含强制轮 |
| 9.11.6 | 重复申请去重 | 跨轮重复已成功的同工具同参数，再对失败/非法参数修正重试 | 成功组合后续 denied/duplicate reason，不重复取数且纯重复不占轮；失败或非法参数可修正重试。连续两轮无进展后先强制收敛，不直接等同 degraded |
| 9.11.7 | token、时间与申请数预算 | 分别构造累计 usage 达到 80000、turn_budget_s=2、单轮超过 4 个不同参数申请 | token 达预算先强制收敛，时间耗尽直接 turn_budget_exceeded；单轮只受理前 4 个不同 tool_code+params，超出不执行 |
| 9.11.8 | 前端多轮展示 | 同工具不同参数、多轮调用后检查 AnswerCard/ContextPanel | tool_calls 保留记录，但 AnswerCard key 仅 tool_code，右侧只保留每个工具最后状态且目录固定，不是完整 Loop 时间线；execution.tasks.propose 不是 Loop 白名单 |
| 9.11.9 | Loop 脚本边界 | 核对 `e2e-ai-agent-loop.ps1` 的 SKIP、数据与断言 | 覆盖只读 Loop、执行拒绝、发现和 3 轮耗尽的部分断言；无 active 告警会 SKIP 后 exit 0。会话保留，未覆盖全部预算/去重/强制收敛/无执行任务断言，不是 UI 验收 |
| 9.11.10 | 证据覆盖缺口 | 分别使用资源分组型规则、云产品级规则（无资源绑定）、SQLServer 等单引擎规则、零数据规则、超过 288 点曲线、长窗口和大规则集，检查模型实际 observations | 规则资源优先通过 CES `ListAlarmRuleResources` 补全并用其 count 校正 resource_count，透传 `resource_level`/`dimension_name`；分组规则再展开实例；无绑定规则按 namespace + 维度名发现实例兜底（`scope_source=resource_discovery`，resource_count 为空、scope_note 给出未匹配原因），并按引擎线索匹配实例，只有实例发现也取不到才 no_resource_scope；预算内尽量覆盖全部实例（单规则上限 200、6 并发），覆盖不全标记 sampled；有数据规则附带 `recent_*`（近 24h/60s）分钟级复核；零数据规则返回 `probe_point_count` 区分“近期无数据/窗口无点”。288 点定稿保留 `peak_at` 与破限片段/时长；频率判断按近端 24h/60s 复核给出近似值，不再要求 1s 原始数据或将其列为数据缺失 |

> 前置：契约见 `ops/ai-contract.md` §10.16；无需新迁移。循环内不产生执行动作，`actions` 仍由服务端生成。`e2e-ai-agent-loop.ps1` 只自动覆盖 9.11.1/9.11.2/9.11.3/9.11.5 的部分场景，需要 active 告警；9.11.4/9.11.6 及预算/强制收敛其他分支由既有单测与人工验收分别记录。

### 9.12 流式对话与取消（SSE，契约 §10.17）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.12.1 | SSE 事件流 | `curl -N -X POST http://127.0.0.1:8080/api/ai/chat/sessions/{session_id}/messages/stream -H "Authorization: Bearer <token>" -H "Content-Type: application/json" -d '{"content":"分析这个告警","context":{"alert_id":"<alert_id>"}}'` | 响应头含 `Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`；依次收到 `stage:gathering`、`tool`（有告警上下文时）、`stage:thinking`、`delta(thinking)`、`delta(answer)`、`stage:answering`、`answer`；每条事件为 `data: <json>` + 空行，心跳为 `: ping` 注释 |
| 9.12.2 | 工具事件 | 用有 `alarm.analyze` 权限的账号发起 9.12.1 | 收到 `type=tool`、`tool_code=alarm.analyze`、`allowed=true`、`status=ok`；无权限账号收到 `allowed=false` 且带 `reason`，回答仍返回 |
| 9.12.3 | 业务错误转事件 | 对已归档会话调用流式接口 | HTTP 200，收到 `{"type":"error","code":"FAILED_PRECONDITION",...}`；无 `answer` 事件；不落 assistant 消息 |
| 9.12.4 | 缺 content | 请求体为空 | 写 SSE 之前返回 400（统一响应结构），无事件下发 |
| 9.12.5 | 客户端断开 / 停止 | 在用户消息已落库且尚未进入最终 answer 落库阶段时终止 curl（或前端点「停止生成」） | 命中取消检查点时保留 user、无 assistant，审计 `chat_canceled` 含 phase/result=canceled；首个 gathering 事件本身不保证 user 已落库，最终 answer 竞态另按 9.12.23 验收 |
| 9.12.6 | 幂等重放 | 同一 `client_message_id` 先调同步接口，再调流式接口 | 流式只收到 `gathering` + `answer`，`message_id` 与首次一致；不新增消息、不再次调用模型；审计 `chat_message` 带 `replayed=true` |
| 9.12.7 | 前端阶段提示与停止 | AI 助手页发送长问题；发送中点击「停止生成」 | 阶段提示按事件切换；thinking delta 可展示过程文本，answer delta 只显示“已生成 N 字”；停止后回读、无本地半成品并提示已停止，不能推广为所有断流都不落 assistant |
| 9.12.8 | SSE 心跳 | 让模型调用持续 >15s（或临时调小 `sseHeartbeatInterval`） | 事件之间出现 `: ping` 注释且连接不断；客户端解析器忽略注释行 |
| 9.12.9 | 回合时间预算 | 临时设置 `AIOPS_AI__TURN_BUDGET_S=2`，让模型调用阻塞超过预算 | 返回 `status=degraded`、`degraded_reason=turn_budget_exceeded`；保留已获取的只读观测；审计 `chat_message` 记录该原因 |
| 9.12.10 | 流式长回答超时语义 | provider `timeout_ms=30000`，mock 持续输出增量超过 30s | 不被 30s 总超时切断；首块 60s / 空闲默认 180s 内持续输出即继续，最终 `status=ok`；总时长受 `turn_budget_s` 约束；`ai.stream_idle_timeout_s` 可调且低于 30s 按 30s 处理 |
| 9.12.11 | 首块前重试 | mock 首次返回 429（带 `Retry-After`）或 5xx，随后成功 | 服务端退避后重试，最多 2 次；最终正常返回 `answer`；审计可见本轮成功 |
| 9.12.12 | 部分保留 | mock 输出部分 `delta` 后断开/静默超过空闲阈值 | 返回 `status=incomplete`、`answer.incomplete=true`、`incomplete_reason` 有值；观测保留，assistant 落库且重放保留标记；前端显示橙色中断原因，但旧 key 重试只重放，不等于重新生成 |
| 9.12.13 | 已输出不重试 | mock 输出部分内容后断开 | 不发起第二次模型请求（避免重复文本）；走 9.12.12 的部分保留路径 |
| 9.12.14 | 非重试错误 | provider 返回 400/401/403 | 不重试，直接失败；不产生 `incomplete` 回答 |
| 9.12.15 | 429 退避重试 | mock 首次 429（`Retry-After: 1`），随后成功 | 服务端退避后重试成功；最终 `status=ok`，耗时约 1s 以上 |
| 9.12.16 | 429 配额恢复提示 | mock 持续 429，响应体含 `quota resets in 2h` | `status=degraded`、`degraded_reason=provider_rate_limited`；`uncertainties` 含“预计约 2 小时后恢复”；前端失败提示直接展示该文案 |
| 9.12.17 | 慢流不被总超时切断 | mock 慢流持续输出约 45s | `status=ok` 且耗时 >30s，证明流式不再受 30s 总超时约束 |
| 9.12.18 | 首块等待提示 | 前端发送长问题，模型 1s 内无回答增量 | 阶段提示显示“已等待 Ns”；收到首个回答增量后切换为“已生成 N 字” |
| 9.12.19 | 跨会话并发与同会话排队 | 会话 A 生成中：新建会话 B 并发送；再在 B 连续发送第二条 | A/B 可同时生成，会话列表分别显示「生成中」；切回 A 不串消息、不丢后台回答；B 第二条进入队列，首条完成后自动发送；同一会话内不并发 |
| 9.12.20 | 空闲计时不被心跳误判 | mock 只发送 `: ping` 注释（间隔小于空闲阈值）后再发送正文 | 空闲计时被注释行重置，不返回 `incomplete`，最终正常 `answer` |
| 9.12.21 | 输出超限 | mock 输出部分正文后返回 `finish_reason=length`，再发 `[DONE]` | 返回 `status=incomplete`、`incomplete_reason=output_truncated`、`degraded_reason=output_truncated`；不重试；前端提示“达到模型输出长度上限”，不出现“模型不可用”文案 |
| 9.12.22 | 中断原因透出 | mock 触发 `stream_idle_timeout` / `stream_read_error` / `empty_model_response` | `chat_message` 审计含 `incomplete_reason`；assistant `answer.incomplete_reason` 保留具体值；前端橙色横幅显示中文原因，未知原因原样透出 |
| 9.12.23 | 最终 answer 竞态 | 在 assistant 已落库后模拟最终 answer 写失败/客户端断开，再获取详情 | 已落库 assistant 不回滚，也不补写 chat_canceled；只有取消检查点前才保证不写 assistant，必须重新获取详情判断实际状态 |
| 9.12.24 | 传输中断与重试 | 模拟无 answer/error 的连接中断后回读，再点击重试；与主动 stop 对比 | 当前工作台可能仍显示 failed，即使回读已有回答；旧 key 重试只重放并可能本地重复消息。主动 stop 与传输中断不是同一 UI 结果 |
| 9.12.25 | 流式脚本清理与覆盖 | 核对 `e2e-ai-stream.ps1` 的会话、临时文件、mock 和 `-SkipSlow` | 临时文件/mock fixture 清理，业务会话/消息/审计保留；慢流可跳过。无告警时不覆盖 tool 事件，缺参主要匹配 INVALID_ARGUMENT，重放未直接证明 replayed=true/不再调模型 |

> 前置：契约见 `ops/ai-contract.md` §10.17 / §10.17.7；`ai.turn_budget_s` 默认 600s，服务端 read/write timeout 必须 ≥ 预算+60s；`ai.stream_idle_timeout_s` 默认 180s（最低 30s，建议 180–300s）。E2E 脚本：`scripts/e2e-ai-stream.ps1` 通过脚本化 mock 自动覆盖 9.12.1/9.12.3/9.12.4/9.12.5/9.12.6 的部分断言及 delta；9.12.2 工具事件需另带告警，9.12.7/9.12.8 前端与心跳需人工抽检，9.12.9 由单测覆盖。9.12.10–9.12.17 由 toolgateway/application 测试覆盖，E2E 的 `MOCK_STREAM_429` / `MOCK_STREAM_429_ALWAYS` / `MOCK_STREAM_PARTIAL` / `MOCK_STREAM_SLOW` 分支另覆盖 9.12.11/9.12.12/9.12.15/9.12.16/9.12.17（`-SkipSlow` 可跳慢流）。9.12.18 需人工抽检；9.12.19 由 useChat.spec.ts 覆盖跨会话并发/同会话排队，UI 仍需人工；9.12.20/9.12.21 由 chat_stream_retry_test.go 覆盖，9.12.22 由 chat_service_test.go 与 toolReason.spec.ts 覆盖。9.12.23–9.12.25 当前只登记边界，不把脚本存在写成已通过。

### 9.13 会话只读分享（migration 0053，契约 §10.18）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.13.1 | 创建分享 | 会话列表点「分享」，选 7 天后点「生成链接」 | 返回 `share_id`、`token`、`share_url`；`expires_at` 约为 7 天后；弹窗展示可复制链接；审计 `chat_share_create` |
| 9.13.2 | 访问分享（另一账号） | 用另一具备 `app:ai:chat` 的账号打开 `share_url` | 只读渲染会话历史与结构化回答；顶部有只读提示；无输入框、反馈与「生成执行计划」按钮 |
| 9.13.3 | 撤销后失效 | owner 在弹窗点「撤销」，再用同一链接访问 | 撤销成功；链接访问返回 404，页面提示「链接不可用」；审计 `chat_share_revoke` |
| 9.13.4 | 过期失效 | 手工把 `ai_session_share.expires_at` 改到过去，再访问 | 返回 404（与撤销不可区分）；不泄露会话内容 |
| 9.13.5 | owner 边界 | 非 owner 调用创建/列表/撤销接口 | 返回 403 `PERMISSION_DENIED`；跨会话的 `share_id` 撤销返回 404 |
| 9.13.6 | 上限与参数 | 同一会话连续生成 11 条有效链接；再传 `expires_in_hours=0`/`721` 创建 | 第 11 条返回 409 `FAILED_PRECONDITION`；0 取默认 7 天；721 返回 400 |
| 9.13.7 | 会话删除联动 | 分享后删除源会话，再访问链接 | 返回 404；不返回任何消息 |
| 9.13.8 | 审计与日志脱敏 | `GET /api/audits?resource_type=ai&resource_id={session_id}`；再查看后端访问日志中的分享访问记录 | 可查 `chat_share_create` / `chat_share_revoke` / `chat_share_view`；审计 payload 无 token、无消息正文；访问日志路径显示为 `/api/ai/chat/shares/<redacted>` |
| 9.13.9 | 未装配降级 | 未注入分享仓储的环境调用分享接口 | 返回 503 `UNAVAILABLE`；普通对话、流式与归档不受影响 |

> 前置：已执行迁移 `0053`；分享为**对内只读**，不提供匿名/对外链接。契约见 `ops/ai-contract.md` §10.18；应用层用例见 `internal/ai/application/chat_share_test.go`。E2E 脚本 `scripts/e2e-ai-assistant.ps1` 自动覆盖 9.13.1/9.13.3/9.13.8 与 9.13.2 的“链接可访问”部分（同账号访问）；9.13.2 的跨账号语义、9.13.4 过期、9.13.5 owner 边界、9.13.6 上限与参数由单测覆盖（过期需改库后人工复查 UI），9.13.9 未装配降级由单测覆盖、标准装配下不可复现。
### 9.14 规则体检只读工具（migration 0055/0062，契约 §10.12.1）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.14.1 | 工具权限种子 | `go run ./cmd/migrate` 后查 `iam_ai_tool_permission` / `iam_role_ai_tool_permission` | 存在 `alert.rules.list` 与 `alert.rules.health`（均 read_only）且绑定 admin；非 admin 调用 `tool_requests` 返回 `allowed=false` |
| 9.14.2 | 规则列表参数与证据 | 让模型申请 `alert.rules.list`，params 带 `namespace=SYS.RDS`、`limit<=50` | 观测 `status=ok`，`data.rule_count>=1`，规则含 `metric/operator/threshold/unit/window_seconds/resource_count/resources`；回答 `references` 含 `observability_evidence`；`tool_invoke` 审计带 `evidence_id` |
| 9.14.3 | 自由指标参数 | 让模型申请 `metrics.query`，params 带 `namespace/metric/dimensions/from/to/period` | 观测 `status=ok`；缺省字段由告警推导；`period=3600` 时 30 天窗口通过，>30 天、>7 天且 `period<3600`、或点数>1440 返回 `status=invalid_argument` |
| 9.14.4 | 账号数据范围 | 用无权访问该 integration 账号的账号发起规则/指标工具 | 观测 `allowed=true`、`status=permission_denied`，不返回规则/指标数据；审计 `result=partial` |
| 9.14.5 | Loop 审计 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | `chat_message` 含 `loop_rounds` 与实际规则/指标工具码；每个 `tool_invoke` 含 `evidence_id`；无执行类工具调用 |
| 9.14.6 | huawei ak_sk 真实规则 | 用带 `alerts` 能力的真实 ak_sk 账号申请规则列表（参数含 account_id/namespace/region） | 观测 `status=ok`、`rule_count>=1`；规则含 metric/operator/threshold/window_seconds；`tool_invoke` 审计带 `evidence_id` |
| 9.14.7 | huawei agency 不支持 | 用 agency 账号申请规则列表 | 观测 `status=unsupported`、reason 稳定；回答仍正常返回 |
| 9.14.8 | 批量规则体检 | 让模型申请 `alert.rules.health`，params 带 `namespace=SYS.RDS`、`limit<=20`、30 天窗口与 `period=3600`，另覆盖资源分组型与云产品级规则 | 观测 `status=ok`，每条规则含统计字段与 `scope_source`（rule / resource_group / resource_discovery）；资源分组型展开实例、云产品级按 namespace 发现实例兜底，均最多 3 实例抽样并取越限最严重样本（`checked_resources`/`sampled` 标明覆盖）；只有实例发现也取不到才 `no_resource_scope`，审计 evidence_id 取首条成功证据 |
| 9.14.9 | 规则体检边界 | `limit=21`、超过 30 天、>7 天且 `period<3600`、只传 `from`；再检查 288 点定稿、全实例覆盖与分钟级复核 | 非法参数返回 `status=invalid_argument` 且不取数；单条失败不阻断其它规则，总预算（180s）耗尽剩余 `skipped/rule health query timeout`，主查询超过 400 次剩余 `skipped/rule health query budget exhausted`；预算按广度优先分配，单规则最多 200 实例、6 并发，覆盖不全标记 sampled；有数据规则附带 `recent_*`（24h/60s）。288 点定稿统计按全量并保留峰值时刻/越限时长，小时级 max 仍不能还原原始秒级误报 |
| 9.14.10 | 越限时段画像与根因证据 | 对一条 30 天窗口的 RDS 复制延迟规则申请 `alert.rules.health`，构造越限点集中在每日同一小时的数据 | 规则含 `breach_by_hour`（24 桶）、`breach_top_hours`、`breach_days`、`peak_at_local` / `breach_first_at_local`，观测 `data.timezone` 与统计一致；回答引用时段画像形成假设；如怀疑备份窗口，应申请 `rds.backup.query` 核对策略/最近备份记录；未取得证据时列入 `uncertainties`，不得只建议放宽阈值 |

> 前置：已执行迁移 `0055` 与 `0062`；契约见 `ops/ai-contract.md` §10.12.1 / §10.16。E2E 脚本：`scripts/e2e-ai-rules.ps1`（脚本化 mock 模型 + fake `auth_type=none` 账号，自动覆盖 9.14.1/9.14.2/9.14.3/9.14.5；9.14.4 数据范围拒绝、9.14.6 real ak_sk、9.14.7 agency 不支持、9.14.8/9.14.9/9.14.10 批量体检与时段画像由单测覆盖，真实账号需手工回归）。

### 9.15 AI 上下文发现工具（migration 0056，契约 §10.12.1）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.15.1 | 工具权限种子 | `go run ./cmd/migrate` 后查 `iam_ai_tool_permission` / `iam_role_ai_tool_permission` | 存在 `accounts.list` / `resource.analyze` / `resources.list`（read_only）且绑定 admin；非 admin 申请返回 `allowed=false` |
| 9.15.2 | accounts.list 自动注入 | 无告警上下文的新会话发送任意问题 | `tool_calls` 含 `accounts.list`（`status=ok`），观测 `data` 含 `accounts/total`；`tool_invoke` 审计可见；账号清单进入模型 prompt |
| 9.15.3 | resource.analyze 资产上下文 | 资产页选中云同步 RDS 资源后提问 | `tool_calls` 含 `resource.analyze`；观测 `data` 含 `account_id/region/namespace/dimensions`；随后 `metrics.query` 缺省字段按资源推导 |
| 9.15.4 | 观测页云维度资源 | 观测页执行一次指标查询后提问（上下文 `resource_id` 为 instance_id） | `resource.analyze` 按 `cloud_resource_id` 命中并带入账号/区域/维度；资源未同步时 `reason=resource_not_found`，回答降级不中断 |
| 9.15.5 | resources.list 纯对话发现 | 无告警/资源上下文提问「列出 RDS 实例」 | 模型申请 `resources.list`；观测 `resources` 含 `cloud_resource_id/namespace/dimension_name`；账号缺失时 `status=skipped`、`reason=no_account` |
| 9.15.6 | 数据范围 | 用无权访问该集成账号的账号申请 `resource.analyze` / `resources.list` | `status=denied` / `permission_denied`；不返回资源数据；审计 `result=partial` |
| 9.15.7 | 前端提示具体化 | 无上下文且模型返回 `insufficient_context` | ChatPanel 按 skipped reason 展示具体补全指引（缺账号 → 云账号接入；缺命名空间/维度 → 资产管理；缺指标名 → 提示补充指标名） |
| 9.15.8 | 资源维度推导 | 对 `SYS.RDS` 资源提问 CPU 指标 | 缺省 `dimensions` 为 `rds_cluster_id` 且值为资源主维度；阈值不因资源推导而臆造 |

> 前置：已执行迁移 `0056`；契约见 `ops/ai-contract.md` §10.12.1。E2E 脚本：`scripts/e2e-ai-agent-loop.ps1` 的 `MOCK_TOOL_DISCOVERY` 分支自动覆盖 9.15.2 / 9.15.5 的链路；9.15.3 / 9.15.4 / 9.15.6 / 9.15.8 由单测与手工验收覆盖（需要真实云同步资源）。
### 9.16 图片输入（migration 0059，契约 §10.20）

| # | 检查项 | 操作 | 预期 |
| --- | --- | --- | --- |
| 9.16.1 | 视觉开关与目录 | 在「AI 配置 → Provider 管理」为类型 b provider 开启「视觉能力」，调用 `GET /api/ai/chat/models` | 列表与 models 返回 `vision_enabled=true`；聊天页图片按钮变为可用 |
| 9.16.2 | 上传与粘贴发送 | 选择视觉模型，粘贴截图或上传图片（可附加文字）发送 | 图片随消息发送，用户气泡显示缩略图；回答包含图片理解内容；provider payload 为多模态 content（text + image_url） |
| 9.16.3 | 非视觉模型拦截 | 切换到未开启视觉的模型后带图发送 | 400 `INVALID_ARGUMENT`；用户消息未落库、未调用模型；前端提示开启视觉或更换模型 |
| 9.16.4 | 纯图片消息 | 不填文字只发图片 | 服务端补默认提问「请分析这张图片。」；回答正常返回 |
| 9.16.5 | 参数边界 | 分别构造 4 张图片、GIF、远程 URL、非法 base64、单张 >2MB 的请求 | 均 400 `INVALID_ARGUMENT`；空文本且无图仍在 SSE 之前返回 400 |
| 9.16.6 | 历史与分享回显 | 刷新会话详情；创建只读分享并打开链接 | 会话详情用户消息 `images` 正常回显缩略图；分享接口同样返回 `images`，但共享页当前只渲染文本、**不展示附图**（已登记前端缺口）；分享页无输入入口 |
| 9.16.7 | 审计与脱敏 | 查 `GET /api/audits?resource_type=ai&resource_id={session_id}` 与后端日志 | `chat_message` 含 `image_count`；审计 payload、日志与 Prompt 文本字段不含图片 base64 |

> 前置：已执行迁移 `0059`；图片理解效果需真实视觉模型验证，报文与拒绝语义由 `internal/ai/application/chat_images_test.go`、`internal/ai/toolgateway/chat_vision_test.go` 自动覆盖；前端工具函数由 `web/src/views/ai-assistant/composables/imageInput.spec.ts` 覆盖。图片链路当前没有 AI E2E，9.16 各项需真实视觉模型人工验收；ShareDialog/SharedSession/useChat 图片链路无专项前端测试，共享页附图渲染缺口见 9.16.6。

### 9.17 RDS for PostgreSQL 历史会话（migration 0061，契约 §10.21）

| # | 检查项 | 操作 | 预期 |
| --- | --- | --- | --- |
| 9.17.1 | 工具权限与真实只读采集 | 执行 `.\scripts\e2e-postgres-history.ps1 -AlertId <长事务告警ID>` | `postgres.activity.query.status=ok`；现有云账号返回历史会话；输出证据 ID 和 `diagnosis`，无 kill/自由 SQL/数据库直连 |
| 9.17.2 | IAM 与功能缺口 | 分别缺少 `rds:instance:getHistorySession/getWaitEvent/getTopSql` 或实例不支持历史会话时运行 | 主接口失败明确为 error/permission_denied；辅助接口失败进入 `missing_evidence/collection_errors`，不得伪装成无等待或无 SQL |
| 9.17.3 | 空闲事务分类 | 历史会话含 `idle in transaction` 且事务时长 >0 | `root_cause.code=idle_in_transaction`、`status=confirmed`，页面显示“分类已确认”而非“已确认根因”；应用原因仍未定并缺少 `application_logs`；事实含会话、Query ID 和匹配 Top SQL 的脱敏摘要；`Client/ClientRead` 不得解释为锁等待 |
| 9.17.4 | 活跃长 SQL 分类 | 历史会话含 `active` 且事务时长 >0 | `root_cause.code=active_long_query`、`status=suspected`；说明仍需执行计划和等待事件，不臆造慢因 |
| 9.17.5 | 锁等待分类 | 会话含 `wait_event_type=Lock` 或有效阻塞进程 | `root_cause.code=lock_wait`、`status=suspected`；只陈述锁等待和直接阻塞者，不自动 confirmed |
| 9.17.6 | 无现场/失败降级 | 采集失败或窗口内无可分类会话 | `root_cause.status=undetermined`，页面显示缺失证据和三类待验证假设；不得输出“未提交/持续阻塞”等确定根因 |
| 9.17.7 | 脱敏与截断 | 检查真实或 mock 会话、Top SQL、证据 payload 和 Prompt | 无原始 SQL、字符串/数字字面量、注释、用户名或完整客户端标识；只含 user_hash、客户端网段/摘要、query_id/sql_hash、sql_operation 和最长 300 字的脱敏 sql_summary；上游 total_count、original/retained 和 truncated 可见 |
| 9.17.8 | 双入口一致性 | 分别调用 Chat 与 `POST /api/ai/analyze-alert` | Chat observations 含 `postgres.activity.query`，分析 payload 含 `postgres_activity`；两端 `diagnosis` 语义一致 |
| 9.17.9 | 快照 fail-closed 与截断 | 停用快照存储或制造写库失败；构造超限 payload | 原 `ok` 结果转为 `error`（`postgres_snapshot_store_disabled`/`postgres_snapshot_persist_failed`）并清空 evidence_id，不进入模型；超限按先丢 Top SQL、再丢等待事件、逐条丢会话截断，计数与 `truncated` 保留 |

> 前置：已执行迁移 `0061`，账号具有 `rds:instance:getHistorySession/getWaitEvent/getTopSql`；历史窗口最长 7 天。当前仅支持查询时读取华为历史采样，尚未实现平台在告警触发后的主动快照。单测覆盖分页、脱敏、部分失败、四类判定和 fail-closed；真实账号权限与实例功能由 9.17.1 验证。E2E finally 只清理 mock provider，会话与证据保留；9.17.2 的 IAM 权限缺口组合需人工验证。告警页 AI 分析弹窗不展示 `diagnosis`，结构化诊断展示以 AI 助手回答卡为准。

### 9.18 RDS 备份策略/记录查询（migration 0063，契约 §10.12.1）

| # | 检查项 | 操作 | 预期 |
| --- | --- | --- | --- |
| 9.18.1 | 工具权限种子 | `go run ./cmd/migrate` 后查 `iam_ai_tool_permission` / `iam_role_ai_tool_permission` | 存在 `rds.backup.query`（read_only）且绑定 admin；非 admin 申请返回 `allowed=false` |
| 9.18.2 | 策略与记录 | 用 ak_sk 账号对标准 RDS MySQL 实例申请 `rds.backup.query` | `status=ok`；`policy` 含 `start_time_utc`、`window_local`、`period`、`keep_days`；`backups` 含本地时间、类型、状态、大小；时区与 `app.timezone` 一致 |
| 9.18.3 | 参数回退与边界 | 分别不传 instance_id、传 `days=31`、`limit=21`、非法 instance_id，覆盖 `rds_instance_id`/`rds_cluster_id`/`cloud_resource_id` 回退 | 缺实例返回 `skipped/no_instance_id`；越界与非法字符返回 `invalid_argument`；显式参数优先于标签/资源回退 |
| 9.18.4 | 权限与 agency | agency 账号申请；用无权访问该账号的用户申请 | agency 返回 `unsupported`；无数据范围返回 `permission_denied`，不泄露实例/备份数据 |
| 9.18.5 | 部分失败与脱敏 | 模拟策略成功、备份列表失败；检查观测、审计与日志 | 保留 `policy`，`collection_errors` 记录 `backups:*`；无 AK/SK、Token、原始凭据；`tool_invoke` 审计含 `instance_id` 与状态 |
| 9.18.6 | 与规则体检联动 | 对每日固定时段越限的规则申请 `rds.backup.query` | 回答同时引用时段画像与备份窗口/记录；策略缺失时列入 `uncertainties`，不得假设备份窗口或只建议放宽阈值 |

> 前置：已执行迁移 `0063`；账号需具备 RDS 实例备份策略/备份列表只读权限（具体 IAM action 以华为云 RDS 文档为准）。`rds_cluster_id` 在标准 RDS 实例上通常与实例 ID 一致，集群版可能需要 `rds_instance_id`；对不上时返回稳定错误，不做模糊猜测。单测覆盖参数回退、UTC→本地窗口、备份记录映射、agency/权限与部分失败；真实账号由 9.18.2/9.18.4 手工验证。当前没有专项 E2E 脚本。

### 9.19 Prompt 输入/输出预算（migration 0064/0065，契约 §10.22）

| # | 检查项 | 操作 | 预期 |
| --- | --- | --- | --- |
| 9.19.1 | 迁移与字段 | `go run ./cmd/migrate` 后查 `ai_provider.context_window_tokens` / `max_output_tokens`；通过 `POST /api/ai/providers` 写入 128000/30000、0/0、512/0、0/128、8000/8000 | 列存在且默认 0；合法值落库并透传运行时；过小窗口或输出上限、输出上限 ≥ 窗口均返回 `INVALID_ARGUMENT` |
| 9.19.2 | 自动裁剪顺序 | 设置 `AIOPS_AI__CHAT_PROMPT_TOKEN_BUDGET=4096`，构造 12 条长历史、含 points 的证据、多观测与记忆后提问 | 模型仍被调用；审计 `chat_message.prompt_trims` 可见 `dropped_history` / `stripped_metric_points` / `dropped_observations` / `dropped_memory`，`prompt_tokens_est <= prompt_token_budget` |
| 9.19.3 | 预算不足降级 | 设置 `AIOPS_AI__CHAT_PROMPT_TOKEN_BUDGET=1024` 并发送长问题或带图消息 | 返回 `degraded/prompt_budget_exceeded`；不调用模型；消息落库且保留已取观测 |
| 9.19.4 | 输入预算与窗口覆盖 | provider 配置 `context_window_tokens=8000`，全局输入预算 32768、全局输出 4096；再改配 `context_window_tokens=128000`；再配置 `max_output_tokens=2000` | 8000 时输出封顶 4000、输入 4000；128000 时输出 4096、输入 123904（不被全局 32768 截断）；显式输出 2000 时输入 6000；未配置窗口的 provider 才使用全局输入预算 |
| 9.19.5 | 输出上限下发 | provider 配置 `max_output_tokens=512`，用 mock 记录请求体并让模型输出超长文本 | 同步/流式/分析请求体均含 `max_tokens=512`；输出被截断时返回 `incomplete/output_truncated`（对话）或既有分析降级语义，不因重试绕过上限；审计 `chat_message.prompt_output_limit=512` |
| 9.19.6 | 前端配置 | 「系统设置 → AI 配置 → Provider 管理」编辑类型 b，填写上下文窗口与最大输出并保存 | 列表回显 `context_window_tokens` / `max_output_tokens`；0 表示使用全局预算/默认；保存后无需重启 |

> 前置：已执行迁移 `0064`、`0065`；输入裁剪只约束 AI 助手对话，输出 `max_tokens` 同时作用于对话与 `/api/ai/analyze-alert`。估算为保守口径（3 字节≈1 token，图片 1600 token/张），不是精确 tokenizer；裁剪只影响本轮 prompt，不删除已落库证据。单测：`internal/ai/application/chat_prompt_budget_test.go`、`provider_service_test.go`、`internal/ai/toolgateway/chat_client_test.go`、`chat_completion_test.go`、`pkg/config/config_test.go`。

## 附录 A：仅 UI 手工步骤（精简表）

> 适用场景：前端联调、演示、无脚本环境。
> 前置：API + 前端均已启动，使用 `admin` 登录；建议先跑一遍 E2E 脚本注入测试数据，或按下列步骤自行造数。

### 登录与导航

| 步骤 | 页面 | 操作 | 通过标准 |
|------|------|------|----------|
| A1 | 登录页 | 输入 `admin` / `admin123` 登录（**dev/演示**；生产须先用 `provision-prod-admin.ps1` 创建管理员） | 进入系统，侧栏可见各模块 |
| A2 | 侧栏 | 依次点击首页、告警、资产、Runbook、执行 | 各页面正常加载，无白屏 |

### Dashboard 首页

| 步骤 | 操作 | 通过标准 |
|------|------|----------|
| B1 | 打开首页 | 6 张指标卡显示数字（非全部 `—` 或加载失败） |
| B2 | 点击「活跃告警」类卡片 | 跳转到告警列表 |
| B3 | 查看「最近执行结果」表格 | 有任务行，状态 Tag 颜色正常 |
| B4 | 点击某执行行 | 跳转到执行详情或执行页 |
| B5 | 查看「Runbook 推荐与使用」 | 显示已启用/总数；下方有最近 Runbook 执行 |
| B6 | 点击「刷新」 | 数据重新加载，无报错 Toast |

### 告警

| 步骤 | 操作 | 通过标准 |
|------|------|----------|
| C1 | 打开 `/alerts` | 列表有数据，筛选/分页可用 |
| C2 | 点击一条活跃告警 | 详情抽屉打开，可见标签与时间线 |
| C3 | 对 `new` 告警点「认领」→「开始处理」 | 状态依次变更，时间线追加事件 |
| C4 | 查看已匹配资产的告警 | 应用名/资源名有值，资源为可点击链接 |
| C5 | 点击资源链接 | 跳转资产页，左侧选中应用，右侧资源行高亮 |

### 资产

| 步骤 | 操作 | 通过标准 |
|------|------|----------|
| D1 | 打开 `/assets` | 左侧应用列表、右侧资源区正常 |
| D2 | 新建应用 | 填写名称/环境/namespace，保存后在列表出现 |
| D3 | 选中应用 → 新建资源 | 填写 pod/namespace，保存后在右侧列表出现 |
| D4 | 从告警带 `resource_id` 跳转进入 | 对应资源行高亮并滚动到可见区域 |

### Runbook

| 步骤 | 操作 | 通过标准 |
|------|------|----------|
| E1 | 打开 `/runbooks` | 模板列表可见，含种子模板 |
| E2 | 打开某模板详情 | 可见多步骤、风险等级、匹配条件 |
| E3 | 回到告警详情（processing 状态） | 若有推荐 UI，可见匹配到的预案列表 |

### 执行

| 步骤 | 操作 | 通过标准 |
|------|------|----------|
| F1 | 打开 `/executions` | 任务列表可见，含状态/来源 |
| F2 | 从告警创建任务（页面入口或 API 已造数） | 列表出现 `pending_confirm` 任务 |
| F3 | 打开任务详情 | 可见步骤列表、Runbook 信息（如有）、dry-run 标记 |
| F4 | 确认并执行（页面按钮） | 状态变为 success，结果摘要可见 |
| F5 | 回到关联告警时间线 | 可见 execution_created / started / finished |

### 权限（UI 负向）

| 步骤 | 操作 | 通过标准 |
|------|------|----------|
| G1 | 退出，未登录访问 `/alerts` | 重定向登录页 |
| G2 | 使用无 Dashboard 权限账号登录 | 首页摘要请求 403 或有明确无权限提示 |
| G3 | 无权限账号直链 `/assets` | 页面不崩溃，创建/保存被拦截 |

### UI 验收签字（精简）

| 模块 | 步骤 | 结论 |
|------|------|------|
| Dashboard | B1–B6 | ☐ |
| 告警 | C1–C5 | ☐ |
| 资产 | D1–D4 | ☐ |
| Runbook | E1–E3 | ☐ |
| 执行 | F1–F5 | ☐ |
| 权限 | G1–G3 | ☐ |

**验收人** ______　**日期** ______
