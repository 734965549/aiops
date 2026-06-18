# AIOps 演示流程

本文档描述如何在本地或演示环境，按顺序走完 **告警 → 资产匹配 → Runbook 推荐 → 执行确认 → Dashboard 汇总 → 审计** 的完整闭环。适合产品演示、新人上手与发布前抽检。

**默认账号**：`admin` / `admin123`（仅开发/演示环境）
**默认端口**：API `8080`，前端 `5173`

---

## 0. 环境准备（约 2 分钟）

### 方式 A：Docker 一键就绪（推荐）

```powershell
# 仓库根目录
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
```

此模式自动迁移数据库并创建 bootstrap 管理员。确认就绪：

```powershell
curl http://127.0.0.1:8080/readyz
# data.status 应为 "ready"
```

### 方式 B：本地 go run 联调

```powershell
# 1. 仅启动中间件
docker compose -f deployments/docker-compose.yml up -d postgres redis

# 2. 迁移（必做）
make migrate

# 3. 启动 API
go run ./cmd/api -config configs/config.yaml

# 4. 启动前端（新终端）
cd web
npm install   # 首次
npm run dev
```

浏览器访问：`http://127.0.0.1:5173`

---

## 1. 管理员登录

| 步骤 | 操作 | 预期 |
|------|------|------|
| 1.1 | 打开登录页，输入 `admin` / `admin123` | 进入系统，侧栏显示各模块 |
| 1.2 | 打开首页驾驶舱 `/dashboard` | 指标卡、执行列表、就绪状态正常加载 |

也可通过 API 验证：

```powershell
curl -X POST http://127.0.0.1:8080/api/identity/login `
  -H "Content-Type: application/json" `
  -d '{"username":"admin","password":"admin123"}'
```

---

## 2. 创建资产应用与资源

**页面**：`/assets` → 「注册表」Tab

| 步骤 | 操作 | 预期 |
|------|------|------|
| 2.1 | 新建应用，例如名称 `payment-service`，环境 `prod`，namespace `prod` | 左侧应用列表出现新条目 |
| 2.2 | 选中该应用 → 新建资源，填写 `pod=payment-pod-01`、`namespace=prod` | 右侧资源列表出现新条目 |

**自动化替代**（无需手工填表）：

```powershell
.\scripts\e2e-asset.ps1
```

脚本会创建应用/资源、配置匹配规则、接入告警并验证绑定，最后清理测试数据。

---

## 3. 配置告警匹配规则（可选）

**页面**：`/assets` → 「匹配规则」Tab

当告警 `service` 标签与应用名不一致时，可通过规则绑定：

| 字段 | 示例值 |
|------|--------|
| 规则名 | payment 服务匹配 |
| Label Key | `service` |
| 匹配模式 | `payment-*`（glob） |
| 目标类型 | resource |
| 绑定应用/资源 | 上一步创建的应用与资源 |

保存后，新接入的告警将按规则优先级匹配，未命中时回退默认 §9.1 逻辑。

---

## 4. 创建告警源

**页面**：`/alerts` → 告警源管理（或通过 API）

| 步骤 | 操作 | 预期 |
|------|------|------|
| 4.1 | 创建 Alertmanager 类型告警源，`enabled=true` | 返回 `source_id` |
| 4.2 | 记录 Webhook Token | 用于下一步 ingest |

**API 示例**：

```powershell
# 需先登录获取 Bearer token
POST /api/alerts/sources
{
  "source_id": "demo-am",
  "name": "演示 Alertmanager",
  "type": "prometheus_alertmanager",
  "enabled": true,
  "webhook_secret": "demo-webhook-secret"
}
```

---

## 5. 接入一条 Alertmanager 告警

通过 Webhook 模拟 firing 告警（labels 含 `service`、`env`、`pod` 等）：

```powershell
.\scripts\e2e-alert.ps1
```

或手工调用：

```powershell
POST /api/alerts/ingest/alertmanager/{source_id}
Header: X-AIOPS-Webhook-Token: <webhook_secret>
Body: Alertmanager firing payload（含 labels）
```

**预期**：
- 返回 `created >= 1`
- 告警列表 `/alerts` 出现新条目，状态为 `new`

---

## 6. 查看告警自动绑定应用/资源

| 步骤 | 操作 | 预期 |
|------|------|------|
| 6.1 | 打开告警详情抽屉 | `application_id` / `resource_id` 已填充（匹配成功时） |
| 6.2 | 点击资源链接 | 跳转 `/assets?application_id=...&resource_id=...`，对应行高亮 |
| 6.3 | 认领 → 开始处理 | 状态依次变为 `acknowledged` → `processing` |

若未匹配到资产，告警仍会保存，资产字段为空——属于正常降级行为。

---

## 7. 查看 Runbook 推荐

告警进入 `processing` 状态后：

| 步骤 | 操作 | 预期 |
|------|------|------|
| 7.1 | 告警详情查看推荐区（或 Dashboard Runbook 区） | 显示匹配到的预案列表 |
| 7.2 | 打开 `/runbooks` | 可见种子模板与自定义模板 |

**API 验证**：

```powershell
GET /api/runbooks/recommendations?alert_id={id}
# items.length >= 1，含 template_id、steps_count、risk_level
```

**自动化**：

```powershell
.\scripts\e2e-runbook.ps1
```

---

## 8. 创建执行任务并确认

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8.1 | 从告警创建执行任务（Runbook 多步或简单 restart） | 任务状态 `pending_confirm` |
| 8.2 | 打开 `/executions`，确认任务 | 输入 `CONFIRM` 后状态变为 `pending_execute` |
| 8.3 | 点击执行 | 状态变为 `success`，步骤输出可见 |
| 8.4 | 回到告警时间线 | 出现 `execution_created` / `execution_started` / `execution_finished` 事件 |

**自动化**：

```powershell
.\scripts\e2e-execution.ps1 -AlertId <processing_alert_id>
# 或与 runbook 脚本一并覆盖：
.\scripts\e2e-runbook.ps1
```

---

## 9. 查看 Dashboard 汇总

**页面**：`/dashboard`

| 检查项 | 预期 |
|--------|------|
| 活跃告警 / P0 / P1 | 与告警中心计数一致 |
| 待确认执行 | 与执行模块 pending_confirm 数一致 |
| 注册资源 | 与应用/资源注册表 count 一致 |
| 最近执行结果 | 含刚执行的任务，点击可跳转 |
| Runbook 推荐与使用 | 显示已启用预案数、最近 Runbook 执行 |
| 平台就绪状态 | `/readyz` 检查结果，版本号可见 |
| 刷新按钮 | 点击后数据更新，无报错 |

**API 抽检**：

```powershell
GET /api/dashboard/summary
# code=OK，各聚合字段有值
```

---

## 10. 查看审计日志

关键操作（登录、资产变更、告警处理、Runbook 启停、执行确认/拒绝、AI 分析等）均会写入审计。

**前端查询**：进入 `/audits`，按资源类型、资源 ID、用户 ID、动作筛选，点击行查看 Payload 详情，必要时导出 CSV。

**API 查询**：

```powershell
GET /api/audits?page=1&page_size=20
```

筛选近期操作，确认包含：

| 模块 | 典型 action |
|------|-------------|
| 认证 | `login`、`refresh` |
| 资产 | `application_create/update/delete`、`resource_create/update/delete` |
| 告警 | `ingest`、`acknowledge`、`close` |
| Runbook | `runbook_create`、`runbook_enable` |
| 执行 | `execution_create`、`execution_confirm`、`execution_execute` |
| AI | `analyze_alert` |

---

## 一键自动化验收

按依赖顺序执行全部 E2E 脚本，全部输出 `PASS` 即主链路通过：

```powershell
# 环境
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
go run ./cmd/migrate

# 主链路
.\scripts\e2e-alert.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1

# 构建验收
go test ./cmd/... ./internal/... ./pkg/...
cd web && npm run build
```

更细的检查项见 [acceptance-checklist.md](./acceptance-checklist.md)；上线前见 [release-checklist.md](./release-checklist.md)。

---

## 演示话术参考（5 分钟版）

1. **开场**：「这是 AI 运维平台首页，汇总活跃告警、待确认执行和平台就绪状态。」
2. **资产**：「我们先注册 payment 应用和 Pod 资源，并配置 service=payment-* 匹配规则。」
3. **告警**：「模拟 Alertmanager 推送一条 CPU 告警，系统自动绑定到对应应用和资源。」
4. **处置**：「进入处理中后，平台推荐 Runbook 预案；我们创建执行任务，人工确认后自动执行。」
5. **闭环**：「执行结果回写告警时间线；Dashboard 实时汇总；所有操作可在审计日志中追溯。」

---

**文档版本**：与 migration `0016`（默认管理员种子）及 Dashboard 拆包同期维护。
