# AIOps 阶段验收清单

**范围**：告警接入 → 资产匹配 → Runbook 推荐 → 执行记录 → Dashboard 摘要 → 权限校验
**环境**：PostgreSQL + API（8080）+ 前端（UI 验收时）
**默认账号**：`admin` / `admin123`（admin 角色已绑定全部种子权限）

---

## 0. 前置准备

| # | 检查项 | 操作 | 预期 |
|---|--------|------|------|
| 0.1 | 依赖启动 | `docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d` | Postgres 健康 |
| 0.2 | 迁移 | `go run ./cmd/migrate` | 含 0001–0023，无报错；当前仓库未包含 `0021` Notification 迁移 |
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
| 2.4.7 | CES 全量口径 | `huawei_cloud` + `auth_type=ak_sk` + CES 只读权限 | 平台 `cloud_sync` active 资源数与 CES 控制台“全部资源”数量一致，或批次摘要能解释差异 |
| 2.4.8 | CES 类型覆盖 | CES 中存在 EVS/VPC/OBS/DCS/DMS 等非 ECS 资源 | 对应资源进入 `asset_resource`，`cloud_resource_type` 与 namespace 映射正确 |
| 2.4.9 | hybrid 增强 | `sync_mode=hybrid` 且授予部分云服务只读权限 | CES 基础资源数不下降，已授权类型补充 IP/VPC/规格/磁盘等详情；增强失败仅记录摘要 |
| 2.4.10 | native 兼容 | `sync_mode=native` | 沿用旧 ECS/CCE/RDS/ELB 同步路径，但页面和批次摘要不承诺与 CES 总览数量一致 |

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

# 3. 资产匹配（独立 RunId，可重复跑）
.\scripts\e2e-asset.ps1

# 3.1 云资源同步（fake provider，migration 0023）
.\scripts\e2e-asset-sync.ps1

# 4. Runbook 推荐 + 多步执行 + 时间线回写
.\scripts\e2e-runbook.ps1

# 5. 简单执行任务（可选）
.\scripts\e2e-execution.ps1 -AlertId <processing_alert_id>

# 6. 权限管理 P1：viewer 角色、授权写接口、403 边界、审计
.\scripts\e2e-identity-access.ps1

# 7. Dashboard API 抽检
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

## 附录 A：仅 UI 手工步骤（精简表）

> 适用场景：前端联调、演示、无脚本环境。
> 前置：API + 前端均已启动，使用 `admin` 登录；建议先跑一遍 E2E 脚本注入测试数据，或按下列步骤自行造数。

### 登录与导航

| 步骤 | 页面 | 操作 | 通过标准 |
|------|------|------|----------|
| A1 | 登录页 | 输入 `admin` / `admin123` 登录 | 进入系统，侧栏可见各模块 |
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
