# AIOps 阶段验收清单

**范围**：告警接入 → 资产匹配 → Runbook 推荐 → 执行记录 → Dashboard 摘要 → 权限校验
**环境**：PostgreSQL + API（8080）+ 前端（UI 验收时）
**默认账号**（仅限 dev/演示）：`admin` / `admin123`（须叠加 `docker-compose.dev.yml` 或 bootstrap 启用）。迁移 `0044` 后生产默认锁定该账号，须 `scripts/provision-prod-admin.ps1` 创建安全管理员后再验收。

---

## 0. 前置准备

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 0.1 | 依赖启动 | `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d` | Postgres 健康 |
| 0.2 | 迁移 | `go run ./cmd/migrate` | 含 0001–0058，无报错；当前仓库未包含 `0021` Notification 迁移 |
| 0.3 | API 启动 | `go run ./cmd/api` 或 compose 内 api 服务 | 登录接口可用 |
| 0.4 | 前端（UI 验收） | `cd web && npm run dev` | 可登录并访问各页面 |
| 0.5 | 自动化冒烟 | 见 [§7 推荐验收顺序](#7-推荐验收顺序自动化) | 全部输出 `PASS` |

---

## 1. 告警接入

**权限**：`app:alerts:ingest`（管理源）、`app:alerts:read`（查询）
**脚本**：`scripts/e2e-alert.ps1`

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
| 1.2.4 | 详情完整性 | `GET /api/alerts/{id}` | 含 `labels`、`events[]`、`severity`、`environment` |

### 1.3 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 1.3.1 | 告警列表 `/alerts` | 可按源/状态筛选，活跃告警可见 |
| 1.3.2 | 告警详情抽屉 | 时间线、标签、状态操作按钮可用 |
| 1.3.3 | 资源链接 | 已匹配资产时显示应用/资源链接 |

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
| 1.4.9 | 审计与收尾 | 完成后查 `GET /api/audits`，另模拟摘要或审计写入失败 | 存在 `alert_source_sync` 记录，状态、计数与 trace 可关联。收尾失败返回 500，保留原最近同步状态 |

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
| 2.1.14 | 列设置持久化 | 勾选附加列（含创建/更新时间）后刷新页面 | 列保持勾选；创建时间列可按服务端字段排序（`updated_at` 待后端补白名单后再开放）；localStorage 脏值被白名单过滤 |
| 2.1.15 | 导出 CSV | 设置筛选后点击「导出 CSV」 | 导出的是当前筛选+排序下的全量结果（非仅当前页）；CSV 带 UTF-8 BOM，逗号/引号/Labels JSON 正确转义；超过 2000 条时提示上限 |

### 2.2 §9.1 标签匹配

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.2.1 | 应用名匹配 | 告警 labels 含 `service=<app_name>` + `env=prod` | `application_id` 命中 |
| 2.2.2 | Pod 匹配 | labels 含 `pod=<registered_pod>` | `resource_id` 命中 |
| 2.2.3 | 精确环境优先 | 同名应用：一条 `environment=''`，一条 `environment=prod` | prod 告警归 prod 应用 |
| 2.2.4 | 未匹配降级 | 无对应注册表 | 告警仍保存，资产 ID 为空 |

### 2.4 云资源同步（migration 0023，阶段 2）

**脚本**：`scripts/e2e-asset-sync.ps1`

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.4.1 | 触发同步 | `POST /api/assets/sync`（`app:assets:write`） | 返回 `batch_id`、`status`、计数摘要 |
| 2.4.2 | fake 账号同步 | `huawei_cloud` + `auth_type=none` | 资产表出现 `source=cloud_sync` 资源，含 `cloud_resource_id` |
| 2.4.3 | 批次查询 | `GET /api/assets/sync/batches?account_id=` | 分页返回历史批次 |
| 2.4.4 | stale 标记 | 二次同步且云端清单变化 | 未出现资源标记 `sync_status=stale`，不物理删除 |
| 2.4.5 | 前端展示 | `/assets` 资源列表 | 来源、云资源 ID、region、同步状态可见 |
| 2.4.6 | P0 不受影响 | 同步失败或禁用账号 | 告警 ingest / 匹配 / 执行闭环仍可跑 |
| 2.4.7 | CES 资源分组口径 | `huawei_cloud` + `auth_type=ak_sk` + CES 只读权限 | 平台 `cloud_sync` active 资源数与**指定 CES 资源分组**（默认候选名“全部资源”，需预先创建）数量一致，或批次摘要能解释差异 |
| 2.4.8 | CES 类型覆盖 | CES 中存在 EVS/VPC/OBS/DCS/DMS 等非 ECS 资源 | 对应资源进入 `asset_resource`，`cloud_resource_type` 与 namespace 映射正确 |
| 2.4.9 | hybrid 增强 | `sync_mode=hybrid` 且授予部分云服务只读权限 | CES 基础资源数不下降；当前已落地 ECS/RDS/VPC/DCS/DMS 详情补充（EVS/OBS 待办）；任一增强失败时批次应为 `partial`，且摘要必须暴露 `enrichment_failed_count` / `enrichment_failed_types` |
| 2.4.10 | native 兼容 | `sync_mode=native` | 沿用旧 ECS/CCE/RDS/ELB 同步路径，但页面和批次摘要不承诺与 CES 总览数量一致 |
| 2.4.11 | 多区域 project_id | `extra_config.region_projects` 配置多 region 映射 | 每个 region 使用对应 project_id 调 CES；未命中回落账号 `project_id` |
| 2.4.12 | 真实账号对账（可选，不进 CI） | 手工执行 `scripts/e2e-asset-sync-real.ps1` | 打印 region 摘要与 type 分组计数，人工比对 CES 控制台总数 |

### 2.3 可配置匹配规则（migration 0014）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.3.1 | 创建规则 | `POST /api/assets/match-rules`（`label_key=service`, `label_value_pattern=payment-*`） | 返回 `id`，`enabled=true` |
| 2.3.2 | 规则优先匹配 | 告警 `service=payment-xxx` 但应用名不同 | 按规则绑定 `application_id`/`resource_id` |
| 2.3.3 | 禁用规则 | `enabled=false` 后 ingest | 规则不生效，回退默认匹配 |
| 2.3.4 | 删除规则 | `DELETE /api/assets/match-rules/:id` | 规则移除，后续 ingest 走默认逻辑 |

### 2.4 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 2.4.1 | 资产页 `/assets` | 注册表 + 匹配规则 Tab，可选中/新建/编辑/删除 |
| 2.4.2 | 告警跳转 | 从告警详情点资源链 → `?application_id=&resource_id=` | 选中应用，资源行高亮并滚动到视口 |

### 2.5 CMDB 资源拓扑与手工关系（migration 0058）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 2.5.1 | 真实归属图 | 创建应用和两个资源后访问 `/assets/topology?application_id=...` | 应用与资源来自注册表，显示两条 `contains` 边，不出现 fake Provider 节点 |
| 2.5.2 | 手工关系 | 在同一应用两个资源间创建 `depends_on`，再刷新拓扑 | 返回唯一关系并显示手工边；重复创建返回 `ALREADY_EXISTS`，自环和跨应用返回 400 |
| 2.5.3 | 图形与列表 | 缩放、适配、切换横纵布局，再切到列表视图 | 画布可用，资源与关系列表一致，节点详情展示上下游 |
| 2.5.4 | 筛选与截断 | 使用 Region、云类型、关键字、关系类型、stale 筛选；构造超过 299 个资源 | 服务端过滤生效；默认不含 stale；超限时明确显示 `truncated` 提示 |
| 2.5.5 | 删除保护 | 删除仍有关联的资源，随后先删关系再删资源 | 首次返回 `FAILED_PRECONDITION`；关系删除写审计后可继续删除资源 |
| 2.5.6 | 权限 | 无 token、只读用户写关系、无资产权限用户查询 | 分别为 401、403、403，页面保留结构且不泄露资源 |

---

## 3. Runbook 推荐

**权限**：`app:runbooks:read`
**脚本**：`scripts/e2e-runbook.ps1`（依赖 migration 0012 种子模板）

### 3.1 推荐 API

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 3.1.1 | 前置状态 | 告警 `status=processing` | — |
| 3.1.2 | 拉取推荐 | `GET /api/runbooks/recommendations?alert_id={id}` | `items.length >= 1` |
| 3.1.3 | 匹配维度 | HighCPU + pod + prod 场景 | 命中种子或自定义模板 |
| 3.1.4 | 推荐内容 | 检查返回项 | 含 `template_id`、`steps_count >= 1`、`risk_level` |

### 3.2 模板管理（可选）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 3.2.1 | 列表 | `GET /api/runbooks/templates` | 含已启用模板 |
| 3.2.2 | 详情 | `GET /api/runbooks/templates/{id}` | 含多步骤 `steps[]` |

### 3.3 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 3.3.1 | Runbook 页 `/runbooks` | 模板列表、启用状态可见 |
| 3.3.2 | 告警页推荐区 | processing 告警可看到推荐预案 |
| 3.3.3 | Dashboard Runbook 区 | 显示已启用/总数，最近 Runbook 执行列表 |

---

## 4. 执行记录

**权限**：`app:executions:read/create/confirm/execute`
**脚本**：`scripts/e2e-execution.ps1`（简单任务）、`e2e-runbook.ps1`（Runbook 多步）

### 4.1 从告警创建任务

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 4.1.1 | 简单任务 | `POST /api/executions/tasks`（`operation_type=restart`） | `status=pending_confirm` |
| 4.1.2 | Runbook 任务 | 同上 + `runbook_template_id` + `dry_run=true` | 多步骤，`runbook_template_id` 非空 |
| 4.1.3 | 确认 | `POST .../confirm`（`confirm=true, confirm_text=CONFIRM`） | `status=pending_execute` |
| 4.1.4 | 执行 | `POST .../execute` | `status=success` |
| 4.1.5 | Dry-run 标记 | Runbook dry-run 任务 | 步骤 `output.dry_run=true` |

### 4.2 告警时间线回写

| # | 检查项 | 预期 |
|---|--------|------|
| 4.2.1 | `execution_created` | payload 含 `execution_id`、`runbook_template_id`（如有） |
| 4.2.2 | `execution_started` | 执行开始后写入 |
| 4.2.3 | `execution_finished` | `payload.status=success` |

### 4.3 执行列表与详情

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 4.3.1 | 列表 | `GET /api/executions/tasks` | 含刚创建的任务 |
| 4.3.2 | 详情 | `GET /api/executions/tasks/{id}` | 含 `task` + `steps[]` |

### 4.4 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 4.4.1 | 执行页 `/executions` | 列表、状态标签、详情可展开 |
| 4.4.2 | Dashboard「最近执行结果」 | 与 API 一致，点击可跳转 |

---

## 5. Dashboard 摘要

**权限**：`app:dashboard:read`（migration 0013）
**API**：`GET /api/dashboard/summary`

### 5.1 聚合字段

| # | 字段 | 预期 |
|---|------|------|
| 5.1.1 | `alerts.active_total / p0 / p1` | 与活跃告警计数一致 |
| 5.1.2 | `executions.pending_confirm` | 与待确认任务数一致 |
| 5.1.3 | `executions.recent` | 最近 ≤10 条任务 |
| 5.1.4 | `assets.applications / resources` | 与资产注册表 count 一致 |
| 5.1.5 | `runbooks.total / enabled` | 与模板总数/启用数一致 |
| 5.1.6 | `processing_alerts` | 最多 5 条活跃处理中告警 |

### 5.2 容错（best-effort）

| # | 检查项 | 预期 |
|---|--------|------|
| 5.2.1 | 部分统计失败 | API 仍返回 `code=OK`，失败块为 0/空 |

### 5.3 前端（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 5.3.1 | 首页 Dashboard | 指标卡有数（活跃告警/P0/P1/待确认/应用/资源等） |
| 5.3.2 | 卡片跳转 | 点击跳转对应模块 |
| 5.3.3 | 刷新 | 点击刷新后数据更新 |

---

## 6. 权限校验

### 6.1 权限矩阵（admin 应具备）

| 资源 | 权限 code | 典型 API |
|------|-----------|----------|
| alerts | `app:alerts:read/acknowledge/update/close/silence/ingest` | `/api/alerts/*` |
| assets | `app:assets:read/write` | `/api/assets/*` |
| runbooks | `app:runbooks:read/create/update/delete` | `/api/runbooks/*` |
| executions | `app:executions:read/create/confirm/execute` | `/api/executions/*` |
| dashboard | `app:dashboard:read` | `/api/dashboard/summary` |

### 6.2 负向用例

| # | 场景 | 操作 | 预期 |
|---|------|------|------|
| 6.2.1 | 未登录 | 无 Bearer 访问受保护 API | `401 UNAUTHENTICATED` |
| 6.2.2 | 无权限用户 | 登录后访问 `GET /api/dashboard/summary` | `403 PERMISSION_DENIED` |
| 6.2.3 | Webhook 无 Token | ingest 不带 token | 拒绝，不创建告警 |
| 6.2.4 | 越权写 | 无 `assets:write` 创建资源 | `403` |
| 6.2.5 | 越权执行 | 无 `executions:execute` 触发 execute | `403` |

> 建议：创建仅含 `app:alerts:read` 的测试用户，逐接口验证 403 边界。

### 6.3 前端路由（可选）

| # | 检查项 | 预期 |
|---|--------|------|
| 6.3.1 | 无权限菜单 | 侧栏不显示或无入口 |
| 6.3.2 | 直链访问 | 无权限时 API 403，页面有错误提示 |

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

# 6. 权限管理 P1：viewer 角色、授权写接口、403 边界、审计
.\scripts\e2e-identity-access.ps1

# 7. AI 运维助手（migration 0049 / 0050；建议先配置类型 b provider）
.\scripts\e2e-ai-assistant.ps1

# 7.1 AI 相似案例记忆（migration 0051；需要一个未降级的模型回答）
.\scripts\e2e-ai-memory.ps1

# 7.2 AI 知识管理（migration 0052 / 0054；脚本自带本地 mock 模型，需 dev 环境）
.\scripts\e2e-ai-knowledge.ps1

# 7.3 AI 流式对话与取消（migration 0049；脚本自带本地 mock 模型，需 dev 环境）
.\scripts\e2e-ai-stream.ps1

# 7.4 AI Agent Loop（migration 0049 / 0056；需要一条 active 告警，脚本自带本地 mock 模型；
#     同时覆盖 accounts.list + resources.list 上下文发现分支）
.\scripts\e2e-ai-agent-loop.ps1

# 7.5 AI 规则体检（migration 0055；脚本自带本地 mock 模型和 fake huawei 账号）
.\scripts\e2e-ai-rules.ps1

# 8. Dashboard API 抽检
# curl -H "Authorization: Bearer <token>" http://127.0.0.1:8080/api/dashboard/summary
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
| 权限管理 P1 | ☐ | `e2e-identity-access.ps1` ☐ | ☐ | ☐ | ☐ |
| AI 运维助手 | ☐ | `e2e-ai-assistant.ps1` / `e2e-ai-memory.ps1` / `e2e-ai-knowledge.ps1` / `e2e-ai-stream.ps1` / `e2e-ai-agent-loop.ps1` / `e2e-ai-rules.ps1` ☐ | ☐ | ☐ | ☐ |
| 审计中心 UI | ☐ | 手工/API ☐ | ☐ | ☐ | ☐ |
| 权限校验 | ☐ | 负向用例 ☐ | ☐ | ☐ | ☐ |

**签字**：验收人 ______　日期 ______　环境 ______（dev/staging/prod）

---

## 9. 云厂商只读接管与观测智能体验收（P1+）

本节用于 P1+ 收口验收，不影响 P0 闭环验收。验收前建议先读 `docs/AI运维平台整体流程与调用关系.md`，确认账号接入、观测查询、巡检证据、建议转执行与审计之间的调用关系。

### 9.1 Integration 接入账号

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.1.1 | 创建只读账号 | `POST /api/integrations/accounts` | 返回 `account_id`，`has_credential=true`，不返回明文密钥 |
| 9.1.2 | 连通性检查 | `POST /api/integrations/accounts/{account_id}/check` | 返回 `status=ok` 或脱敏失败原因 |
| 9.1.3 | 权限负向 | 无 `app:integrations:create` 创建账号 | `403 PERMISSION_DENIED` |
| 9.1.4 | 审计 | 创建/更新/检查账号 | `GET /api/audits` 可查对应动作 |

### 9.2 Observability 查询

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.2.1 | 指标查询 | `POST /api/observability/metrics/query` | 返回 `series[]` 和 `evidence_id` |
| 9.2.2 | 日志搜索 | `POST /api/observability/logs/search` | 返回脱敏摘要，不泄露敏感字段 |
| 9.2.3 | 链路查询 | `POST /api/observability/traces/query` | 返回 Trace 摘要、慢调用和错误调用 |
| 9.2.4 | 限流/降级 | fake provider 超时或限流 | 返回 `UNAVAILABLE` 或 `RESOURCE_EXHAUSTED`，写审计 |

### 9.3 Inspection 巡检

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.3.1 | 创建策略 | `POST /api/inspections/policies` | 返回 `policy_id`，状态 enabled |
| 9.3.2 | 手动触发 | `POST /api/inspections/policies/{policy_id}/runs` | 返回 `run_id`，状态进入 `pending/running` |
| 9.3.3 | 运行完成 | `GET /api/inspections/runs/{run_id}` | 状态为 `success` 或 `partial`，有时间线 |
| 9.3.4 | 发现与建议 | `GET /api/inspections/findings?run_id=...` | 每条发现含风险、证据、建议、置信度 |
| 9.3.5 | 建议转执行 | `POST /api/inspections/recommendations/{id}/execution` | 创建 Execution Task，不直接执行 |

### 9.4 推荐自动化脚本

```powershell
.\scripts\e2e-integration.ps1
.\scripts\e2e-observability.ps1
.\scripts\e2e-inspection.ps1
.\scripts\e2e-execution-agent.ps1
.\scripts\e2e-execution-agent-permission.ps1
```

> Notification 模块与 `scripts/e2e-notification.ps1` 暂未落地，不纳入当前 P1+ 自动化验收。

### 9.5 Execution Agent 执行介体验收

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.5.1 | 创建执行介体 | `POST /api/executions/media` | 返回 `medium_id`，不返回任何登录凭据 |
| 9.5.2 | 注册 fake agent | `POST /api/executions/agents/register` | agent 绑定 medium，状态可心跳为 online |
| 9.5.3 | 创建受控命令任务 | `POST /api/executions/tasks`，含 `execution_mode=agent`、`medium_id`、`command_spec_id` | 任务进入 `pending_confirm` 或 `pending_execute` |
| 9.5.4 | 未确认不可领取 | fake agent 领取 `pending_confirm` 任务 | 返回空任务或拒绝 |
| 9.5.5 | 确认后领取 | `CONFIRM` 后 fake agent lease | 返回单个 `lease_id` 和受控 argv |
| 9.5.6 | 日志回传 | fake agent 回传 stdout/stderr | 任务详情可见日志流，敏感内容被脱敏 |
| 9.5.7 | 结果回传 | fake agent 回传 exit_code/result | step 和 task 状态更新，审计可查 |
| 9.5.8 | 参数校验 | 提交不符合 schema 的 arguments | `INVALID_ARGUMENT`，不创建任务 |

推荐脚本：

```powershell
.\scripts\e2e-execution-agent.ps1
.\scripts\e2e-execution-agent-permission.ps1
```

### 9.6 AI 告警分析指标证据

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.6.1 | 证据采集成功 | AKSK CES 账号（含 `metrics` 能力）触发带结构化条件的告警，调用 `POST /api/ai/analyze-alert` 且 `include_metrics=true` | provider payload 含 `evidence.status=ok`、指标统计与 `evidence_id`；审计含 `evidence_status=ok` 和 `evidence_ids` |
| 9.6.2 | 无指标自然跳过 | 事件类告警或无维度/指标告警勾选包含指标 | `evidence.status=skipped`，`reason` 为 `no_metric`/`no_dimensions` 等，分析仍返回 |
| 9.6.3 | 数据权限降级 | 无绑定账号数据范围的用户发起分析 | `evidence.status=permission_denied`，分析不中断，审计记录该状态 |
| 9.6.4 | 上游/配置降级 | 停用 provider 或使用 agency 账号 | 分析仍返回本地摘要；`evidence.status=error/unsupported`，无凭据或底层原始错误泄露 |

> 9.6.1 需已配置并启用 AI provider；无 provider 时 `include_metrics` 不会发起取数，仅返回本地摘要。

### 9.7 AI Provider 页面配置（migration 0048）

| # | 检查项 | 操作 | 预期 |
| ---|--------|------|------|
| 9.7.1 | 配置持久化 | 在「系统设置 → AI 配置 → Provider 管理」新增类型 `b` provider（Ark 地址 + API Key + `model`/`ep-xxx`），保存后重启 API | 列表仍显示该 provider，密钥显示「已配置」；`ai_provider` 表中 `api_key_encrypted` 为密文，无明文 |
| 9.7.2 | 更新省略 api_key | 编辑该 provider，只改名称/模型、不填 `api_key`，保存后再次分析 | 保存成功且 `has_api_key=true`；分析仍可调用模型（原密钥未丢失） |
| 9.7.3 | 启停即时生效 | 禁用该 provider 后再次调用 `POST /api/ai/analyze-alert`，再启用 | 禁用时回落到本地摘要或下一可用 provider；启用后恢复模型分析，全程无需重启 API |
| 9.7.4 | 换模型即时生效 | 修改 `model`/`ep-xxx` 后保存，再次分析 | 下一次分析使用新模型（日志 `model` 字段可见），无需重启 API |

> 前置：已执行迁移 `0048`；类型 `b` 必须配置 `model`，否则保存返回 400。YAML `ai.providers` 仅在 `ai_provider` 表为空时作为种子写入。

### 9.8 AI 运维助手（migration 0049）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.8.1 | 权限种子 | 执行 `go run ./cmd/migrate` | `app:ai:chat`、`app:ai:chat:feedback` 存在且绑定 `admin`；未授权用户调用返回 403 |
| 9.8.2 | 创建会话 | `POST /api/ai/chat/sessions`（带 `alert_id` 上下文） | 返回 `session_id`，`context.alert_id` 回显，审计含 `chat_session` |
| 9.8.3 | 发送消息 | `POST /api/ai/chat/sessions/{id}/messages` | `answer.summary/risk_level/actions` 完整；`status=ok/degraded/insufficient_context` 合法；审计含 `chat_message` 与 `tool_invoke` |
| 9.8.4 | 工具拒绝可见 | 移除某角色的 `alarm.analyze` 或 `metrics.query` 工具权限后提问 | 工具调用 `allowed=false` 且带 `reason`；回答仍返回并展示拒绝原因，不静默降级 |
| 9.8.5 | 降级回答 | 禁用/删除启用中的类型 `b` provider 后提问 | `status=degraded`、`degraded_reason=model_unavailable`；保留已获取的只读上下文；无执行建议 |
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
| 9.8.21 | 可选模型列表 | 普通账号（仅 `app:ai:chat`）调用 `GET /api/ai/chat/models` | 返回启用中的类型 b 模型数组，含 `provider_id/name/model`；不含 `base_url/api_key/headers/timeout_ms`；管理员 provider 管理接口权限不因此放开 |
| 9.8.22 | 会话内模型切换 | 新建会话前切换模型并发送首条消息；回答完成后再次切换并发送下一条消息 | 每次请求 `context.provider_id`/`provider_id` 为当前所选模型，审计实际 `provider_id` 一致；已有消息时选择器仍可用，仅生成回答期间禁用 |
| 9.8.23 | 模型不可用回退 | 请求已删除/禁用 provider 的 `provider_id` | 请求不失败，回退默认模型；`answer.uncertainties` 首条含“所选模型 … 不可用，已回退 …”；审计 `provider_fallback=true` |
| 9.8.24 | 消息复制 | 悬停用户消息气泡；点击 AI 回答头部「复制全文」 | 用户消息复制原文；AI 回答复制 Markdown（摘要/依据/影响/建议/回滚/不确定性）；提示“已复制”；非安全上下文回退成功 |

> 前置：已执行迁移 `0049`、`0050`（0050 为消息幂等键）；9.8.3 的模型回答需要已启用类型 `b` provider。E2E 脚本：`scripts/e2e-ai-assistant.ps1`（自动覆盖 9.8.2/9.8.3/9.8.7/9.8.8/9.8.9/9.8.10/9.8.15/9.8.16/9.8.17）。
>
> 数据范围（9.8.13/9.8.14）：告警快照读取按对象数据范围过滤，分析路径与对话路径共用同一入口，映射与拒绝语义见 `ops/ai-contract.md` §10.6.4、§10.11。注意告警列表/详情接口的自身数据范围口径不在 AI 模块范围内。

### 9.9 相似案例记忆（migration 0051，阶段 1）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.9.1 | 有效反馈沉淀 | 对一条 `status=ok` 的回答提交 `valid` 反馈 | `ai_memory_case` 新增一条：`source_message_id` 为该回答、`quality_score=1`、`status=active`；重复提交不新增第二条 |
| 9.9.2 | 降级回答不入记忆 | 在无可用 provider（`degraded=true`）时提交 `valid` | 不生成案例；反馈接口仍返回 200 |
| 9.9.3 | 检索注入与引用 | 用同一告警再次提问（或重复调用 `POST /api/ai/analyze-alert`） | 对话 `references` 含 `type=memory_case`、`id=case_id`；provider payload 含 `memory_cases`（分析接口为 `similar_cases`） |
| 9.9.4 | 无效反馈降分与淘汰 | 对同一回答连续提交两次 `invalid` | 第一次 `quality_score=1 → -1` 仍 `active`；第二次 `≤ -2` 转 `retired`；后续检索不再命中 |
| 9.9.5 | 跨用户隔离 | 用另一账号对同一应用/资源提问 | 不返回 A 用户的案例（`memory_status=empty`），日志不含案例摘要 |
| 9.9.6 | 降级不阻断 | 停库或让检索失败后提问（可用错表/断连接复现） | 回答正常返回；审计 `memory_status=error`、`memory_used=0` |
| 9.9.7 | 审计字段 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | `chat_message` payload 含 `memory_used` / `memory_status`，不含案例原文 |
| 9.9.8 | 记忆管理列表 | admin 调用 `GET /api/ai/memory/cases?status=active&keyword=...`；非 admin 调用 | 返回脱敏案例列表（摘要/建议/质量分/状态）；非 admin 返回 403 |
| 9.9.9 | 手动停用/启用 | `POST /api/ai/memory/cases/{case_id}/status` 依次提交 `retired`、`active` | 停用后提问不再命中该案例且审计 `memory_review`（`previous_status`/`status`）；启用后重新参与检索；重复提交同状态幂等且不新增审计 |

> 前置：已执行迁移 `0051`、`0054`；9.9.3/9.9.4 需要已启用类型 `b` provider。契约见 `ops/ai-contract.md` §10.14、§10.19。E2E 脚本：`scripts/e2e-ai-memory.ps1`（自动覆盖 9.9.1/9.9.3/9.9.4/9.9.7；跨用户隔离 9.9.5 与降级 9.9.6 仍需人工验证）；`scripts/e2e-ai-knowledge.ps1` 额外覆盖 9.9.8 的列表检索与 9.9.9 的停用动作。

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
| 9.10.10 | 反馈聚合生成草稿 | 对同一应用积累 ≥3 条 `valid` 反馈后，在「AI 配置 → 知识沉淀」填写应用 ID 点击「反馈聚合生成草稿」 | 返回 `action=created`，生成 `origin=feedback_aggregate`、`status=draft`、`item_id=agg-<app_id>` 的条目；内容含历史案例摘要与建议；审计 `knowledge_edit` 且 `action=aggregate` |
| 9.10.11 | 聚合幂等与保护 | 再次聚合同一应用；随后把该条目激活并再次聚合 | 未激活时 `action=updated` 且条目仍只有一条；激活后 `action=skipped_active`，线上内容不被覆盖 |
| 9.10.12 | 聚合案例不足 | 对有效案例 <3 条的应用点击聚合 | 返回 `action=insufficient_cases`，不生成草稿，前端提示案例不足 |

> 前置：已执行迁移 `0052`。契约见 `ops/ai-contract.md` §10.15。管理入口：「系统设置 → AI 配置 → 知识沉淀」页签（列表 / 新建 / 编辑 / 激活 / 停用 / 删除 / 反馈聚合）。E2E 脚本：`scripts/e2e-ai-knowledge.ps1`（自动覆盖 9.10.2/9.10.3/9.10.4/9.10.6/9.10.7/9.10.10；编辑回草稿 9.10.5、低效自动停用 9.10.8、聚合幂等 9.10.11 与案例不足 9.10.12 仍需人工/单测补充）。

### 9.11 受限 Agent Loop（阶段 3，无新迁移）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.11.1 | 追加只读工具 | 配置一个会在上下文不足时输出 `tool_requests`（`metrics.query`）的模型，提问 | 服务端执行追加取数后二次调用模型；`tool_calls` 含该轮调用；审计 `chat_message` 的 `loop_rounds ≥ 1` |
| 9.11.2 | 执行类工具不可达 | 让模型申请 `execution.command.run` / `execution.tasks.propose` | 该工具 `allowed=false`、`reason=tool is not allowed in agent loop`；**不会产生任何执行动作**，回答仍返回 |
| 9.11.3 | 轮数预算 | 让模型按不同参数持续申请 `metrics.query` | 追加轮数最多 3 轮，随后返回 `status=degraded`、`degraded_reason=agent_loop_exhausted`，保留已取观测 |
| 9.11.4 | 无进展收敛 | 让模型持续申请被拒绝的工具 | 至多两次模型调用即收敛（一次纠正机会），不悬挂、不超时 |
| 9.11.5 | 审计可回放 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | 每轮工具调用都有 `tool_invoke`；`chat_message` payload 含 `loop_rounds`、`tokens` |
| 9.11.6 | 重复申请去重 | 让模型跨轮重复申请已成功的同工具同参数 | 后续申请记 `status=denied`、`reason=duplicate tool request: identical parameters already executed in this turn`，不重复取数、不占有效轮数；连续两轮无进展后返回 `agent_loop_exhausted` |

> 前置：契约见 `ops/ai-contract.md` §10.16；无需新迁移。循环内不产生任何执行动作，`actions` 仍由服务端生成。E2E 脚本：`scripts/e2e-ai-agent-loop.ps1`（通过脚本化 mock 模型自动覆盖 9.11.1/9.11.2/9.11.3/9.11.5，需要一条 active 告警；无进展收敛 9.11.4 与重复申请去重 9.11.6 由既有单测覆盖）。

### 9.12 流式对话与取消（SSE，契约 §10.17）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.12.1 | SSE 事件流 | `curl -N -X POST http://127.0.0.1:8080/api/ai/chat/sessions/{session_id}/messages/stream -H "Authorization: Bearer <token>" -H "Content-Type: application/json" -d '{"content":"分析这个告警","context":{"alert_id":"<alert_id>"}}'` | 响应头含 `Content-Type: text/event-stream`、`Cache-Control: no-cache`、`X-Accel-Buffering: no`；依次收到 `stage:gathering`、`tool`（有告警上下文时）、`stage:thinking`、`delta(thinking)`、`delta(answer)`、`stage:answering`、`answer`；每条事件为 `data: <json>` + 空行，心跳为 `: ping` 注释 |
| 9.12.2 | 工具事件 | 用有 `alarm.analyze` 权限的账号发起 9.12.1 | 收到 `type=tool`、`tool_code=alarm.analyze`、`allowed=true`、`status=ok`；无权限账号收到 `allowed=false` 且带 `reason`，回答仍返回 |
| 9.12.3 | 业务错误转事件 | 对已归档会话调用流式接口 | HTTP 200，收到 `{"type":"error","code":"FAILED_PRECONDITION",...}`；无 `answer` 事件；不落 assistant 消息 |
| 9.12.4 | 缺 content | 请求体为空 | 写 SSE 之前返回 400（统一响应结构），无事件下发 |
| 9.12.5 | 客户端断开 / 停止 | 收到首个事件后终止 curl（或前端点「停止生成」） | 会话只剩用户消息、无 assistant 消息；审计 `chat_canceled` 含 `phase` 与 `result=canceled`；同步接口不受影响 |
| 9.12.6 | 幂等重放 | 同一 `client_message_id` 先调同步接口，再调流式接口 | 流式只收到 `gathering` + `answer`，`message_id` 与首次一致；不新增消息、不再次调用模型；审计 `chat_message` 带 `replayed=true` |
| 9.12.7 | 前端阶段提示与停止 | AI 助手页发送长问题；发送中点击「停止生成」 | 阶段提示按服务端事件切换（思考 → 查询工具 → 生成回答），不再依赖固定定时器；`delta` 只展示“已生成 N 字”；停止后输入区恢复、无半成品回答、提示「已停止生成」 |
| 9.12.8 | SSE 心跳 | 让模型调用持续 >15s（或临时调小 `sseHeartbeatInterval`） | 事件之间出现 `: ping` 注释且连接不断；客户端解析器忽略注释行 |
| 9.12.9 | 回合时间预算 | 临时设置 `AIOPS_AI__TURN_BUDGET_S=2`，让模型调用阻塞超过预算 | 返回 `status=degraded`、`degraded_reason=turn_budget_exceeded`；保留已获取的只读观测；审计 `chat_message` 记录该原因 |
| 9.12.10 | 流式长回答超时语义 | provider `timeout_ms=30000`，mock 持续输出增量超过 30s | 不被 30s 总超时切断；首块 60s/空闲 180s 内持续输出即继续，最终 `status=ok`；总时长受 `turn_budget_s` 约束 |
| 9.12.11 | 首块前重试 | mock 首次返回 429（带 `Retry-After`）或 5xx，随后成功 | 服务端退避后重试，最多 2 次；最终正常返回 `answer`；审计可见本轮成功 |
| 9.12.12 | 部分保留 | mock 输出部分 `delta` 后断开/静默超过空闲阈值 | 返回 `status=incomplete`、`answer.incomplete=true`、`incomplete_reason` 有值；已获取观测保留；assistant 消息落库且重放保留标记；前端显示橙色“生成中断”与重新提问入口 |
| 9.12.13 | 已输出不重试 | mock 输出部分内容后断开 | 不发起第二次模型请求（避免重复文本）；走 9.12.12 的部分保留路径 |
| 9.12.14 | 非重试错误 | provider 返回 400/401/403 | 不重试，直接失败；不产生 `incomplete` 回答 |
| 9.12.15 | 429 退避重试 | mock 首次 429（`Retry-After: 1`），随后成功 | 服务端退避后重试成功；最终 `status=ok`，耗时约 1s 以上 |
| 9.12.16 | 429 配额恢复提示 | mock 持续 429，响应体含 `quota resets in 2h` | `status=degraded`、`degraded_reason=provider_rate_limited`；`uncertainties` 含“预计约 2 小时后恢复”；前端失败提示直接展示该文案 |
| 9.12.17 | 慢流不被总超时切断 | mock 慢流持续输出约 45s | `status=ok` 且耗时 >30s，证明流式不再受 30s 总超时约束 |
| 9.12.18 | 首块等待提示 | 前端发送长问题，模型 1s 内无回答增量 | 阶段提示显示“已等待 Ns”；收到首个回答增量后切换为“已生成 N 字” |

> 前置：契约见 `ops/ai-contract.md` §10.17 / §10.17.7；`ai.turn_budget_s` 默认 600s，服务端 read/write timeout 必须 ≥ 预算+60s。E2E 脚本：`scripts/e2e-ai-stream.ps1`（通过脚本化 mock 模型自动覆盖 9.12.1/9.12.3/9.12.4/9.12.5/9.12.6，含 `delta(thinking/answer)` 断言；9.12.2 工具事件需要告警上下文，9.12.7/9.12.8 前端与心跳需人工抽检，9.12.9 由单测覆盖；9.12.10–9.12.17 由 `internal/ai/toolgateway/chat_stream_retry_test.go` 与 `internal/ai/application/chat_service_test.go` 覆盖；E2E 脚本的 `MOCK_STREAM_429` / `MOCK_STREAM_429_ALWAYS` / `MOCK_STREAM_PARTIAL` / `MOCK_STREAM_SLOW` 分支覆盖 9.12.11/9.12.12/9.12.15/9.12.16/9.12.17（`-SkipSlow` 可跳过 45s 慢流）；9.12.18 需人工抽检前端）。

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

> 前置：已执行迁移 `0053`；分享为**对内只读**，不提供匿名/对外链接。契约见 `ops/ai-contract.md` §10.18；应用层用例见 `internal/ai/application/chat_share_test.go`；E2E 脚本 `scripts/e2e-ai-assistant.ps1` 自动覆盖 9.13.1/9.13.2/9.13.3/9.13.8（跨账号访问、过期与上限仍需人工验证）。
### 9.14 规则体检只读工具（migration 0055，契约 §10.12.1）

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 9.14.1 | 工具权限种子 | `go run ./cmd/migrate` 后查 `iam_ai_tool_permission` / `iam_role_ai_tool_permission` | 存在 `alert.rules.list`（read_only）且绑定 admin；非 admin 调用 `tool_requests` 返回 `allowed=false` |
| 9.14.2 | 规则列表参数与证据 | 让模型申请 `alert.rules.list`，params 带 `namespace=SYS.RDS`、`limit<=50` | 观测 `status=ok`，`data.rule_count>=1`，规则含 `metric/operator/threshold/window_seconds`；回答 `references` 含 `observability_evidence`；`tool_invoke` 审计带 `evidence_id` |
| 9.14.3 | 自由指标参数 | 让模型申请 `metrics.query`，params 带 `namespace/metric/dimensions/from/to/period` | 观测 `status=ok`；缺省字段由告警推导；`period=86400` 时 30 天窗口通过，>30 天或点数>1440 返回 `status=invalid_argument` |
| 9.14.4 | 账号数据范围 | 用无权访问该 integration 账号的账号发起规则/指标工具 | 观测 `allowed=true`、`status=permission_denied`，不返回规则/指标数据；审计 `result=partial` |
| 9.14.5 | Loop 审计 | `GET /api/audits?resource_type=ai&resource_id={session_id}` | `chat_message` 含 `loop_rounds` 与两个工具码；每个 `tool_invoke` 含 `evidence_id`；无执行类工具调用 |
| 9.14.6 | huawei ak_sk 真实规则 | 用带 `alerts` 能力的真实 ak_sk 账号申请规则列表（参数含 account_id/namespace/region） | 观测 `status=ok`、`rule_count>=1`；规则含 metric/operator/threshold/window_seconds；`tool_invoke` 审计带 `evidence_id` |
| 9.14.7 | huawei agency 不支持 | 用 agency 账号申请规则列表 | 观测 `status=unsupported`、reason 稳定；回答仍正常返回 |

> 前置：已执行迁移 `0055`；契约见 `ops/ai-contract.md` §10.12.1 / §10.16。E2E 脚本：`scripts/e2e-ai-rules.ps1`（脚本化 mock 模型 + fake `auth_type=none` 账号，自动覆盖 9.14.1/9.14.2/9.14.3/9.14.5；9.14.4 数据范围拒绝、9.14.6 real ak_sk 与 9.14.7 agency 不支持由单测覆盖，真实账号需手工回归）。

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
