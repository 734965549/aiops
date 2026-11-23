# AIOps 演示流程

本文档描述如何在本地或演示环境，按顺序走完 **告警 → 资产匹配 → Runbook 推荐 → 执行确认 → Dashboard 汇总 → 审计** 的完整闭环。适合产品演示、新人上手与发布前抽检。

**默认账号**：`admin` / `admin123`（仅开发/演示环境，且已启用 bootstrap）

**默认端口**：API `8080`，前端 `5173`

迁移 `0044` 会锁定仍使用已知默认密码的管理员；dev/test 可由 bootstrap 重新激活，生产不能使用本演示账号，须通过 `scripts/provision-prod-admin.ps1` 初始化安全管理员。

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

前置：使用方式 A 的 dev bootstrap，或已在本地 `configs/config.yaml` 启用 dev/test bootstrap。若只执行迁移而未启用 bootstrap，不要假设默认账号仍可登录。

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

脚本会创建应用/资源、配置匹配规则、检查筛选排序和 facets、接入告警验证绑定、验证删除保护，最后清理测试数据；不覆盖云同步或拓扑。

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

### 3.1 云同步扩展（fake 或真实账号）

非生产环境可先运行 fake 同步：

```powershell
.\scripts\e2e-asset-sync.ps1
```

脚本验证 running 批次、终态 success、云资源入库、批次历史和稳定清单不产生 stale。真实 CES/hybrid 对账必须使用独立只读账号并运行 `e2e-asset-sync-real.ps1`，不能用 fake 结果替代。默认候选名未命中时可进入全部资源组并集，显式指定组未命中仍失败。

当前同步数据范围、完整审计生命周期、周期后台 reaper 和云应用命名仍有实现差距，演示时不要讲成已经全部闭环。

### 3.2 CMDB 资源拓扑扩展

1. 打开 `/assets/topology?application_id=<application_id>`，确认应用与资源来自注册表，而不是 Observability fake 调用拓扑。
2. 在同一应用和跨应用资源间创建手工 `depends_on` 关系，再选择根资源并切换 1–3 层和上下游，检查完整路径、循环保护与列表降级。
3. 构造无法唯一解析的云派生端点，确认不出现幽灵节点，页面展示 `unresolved_relation_count`；超过 299 个资源时展示 `truncated`。
4. 使用 `/api/assets/resources/resolve` 分别验证唯一、未注册和多候选，页面不得默认选择多候选第一项。

拓扑目前没有独立 E2E 脚本，演示结果需结合后端测试和前端 Vitest 记录。

---

## 4. 创建告警源

**页面**：`/alerts` → 告警源管理（或通过 API）

| 步骤 | 操作 | 预期 |
|------|------|------|
| 4.1 | 创建 Alertmanager 类型告警源，`enabled=true` | 返回接入源业务字段 `id` |
| 4.2 | 创建前自行安全保存 Webhook secret | 用于下一步 ingest；响应只返回 `secret_masked`，不会返回明文 Token |

**API 示例**：

```powershell
# 需先登录获取 Bearer token；当前创建源还需要 app:integrations:read
POST /api/alerts/sources
{
  "id": "demo-am",
  "name": "演示 Alertmanager",
  "type": "prometheus_alertmanager",
  "enabled": true,
  "secret": "demo-webhook-secret"
}
```

创建成功后，Webhook 路径使用 `/api/alerts/ingest/alertmanager/demo-am`，请求头使用创建时保存的 secret。更新时省略 `secret` 会保留原哈希。

**CES 拉取扩展演示**：非生产环境可执行 `.\scripts\e2e-alert-sync.ps1`，脚本使用 fake 华为账号完成三轮同步、独立恢复和审计检查。该脚本不验证真实云账号、后台 worker 或多实例租约争抢。

---

## 5. 接入一条 Alertmanager 告警

通过 Webhook 模拟 firing 告警（labels 含 `service`、`env`、`pod` 等）：

```powershell
.\scripts\e2e-alert.ps1
```

该脚本会继续验证重复 firing、resolved、关闭后新 lifecycle、认领、处理、手动恢复和 AI 入口；转派、静默、备注与通用 Webhook 需另行演示或验收。

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

脚本会构造 processing 告警，验证推荐、多步任务、CONFIRM、模拟执行、首步 dry_run 标记和执行时间线；不覆盖模板 CRUD、非 processing 拒绝、low 风险、步骤失败或完整 UI。

当前 `/runbooks` 页面只提供列表、详情和启停。后端支持创建、全量更新和软停用，但页面没有新建、完整编辑或删除入口。推荐 API 本身不限制告警状态，只有创建执行任务时强制 processing。`command_spec_id`、`medium_selector` 和 Runbook→Agent 展开仍是规划能力，不在本演示中宣称已落地。

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

### 8.5 P1+ 执行介体演示扩展

后续演示“AI 分析后到指定介体上排查/处理”时，不把它讲成 AI 直接登录机器执行命令，而是讲成受控执行链路：

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8.5.1 | 管理员注册执行介体，例如 `prod-jumpbox-01` 或目标主机 Agent | 介体状态 `online`，能力、环境、网络区域、风险上限可见 |
| 8.5.2 | AI 根据 CES/APM/AOM/Signoz 证据生成诊断建议 | 建议包含证据、目标资源、推荐 Command Spec 和参数 |
| 8.5.3 | 运维人员从建议创建执行任务 | 任务状态 `pending_confirm`，展示 `medium_id`、`command_spec_id`、参数和风险 |
| 8.5.4 | 输入 `CONFIRM` 后确认 | 状态进入 `pending_execute`，Agent 才能领取租约 |
| 8.5.5 | Fake Agent 或真实 Agent 上报日志与结果 | 任务进入 `running -> success|failed`，日志脱敏，审计可追踪 |
| 8.5.6 | 回到告警或巡检详情页 | 时间线出现执行创建、开始、完成事件，并能跳转到执行详情 |

自动化脚本建议后续补充：

```powershell
.\scripts\e2e-execution-agent.ps1
.\scripts\e2e-execution-agent-permission.ps1
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

**文档版本**：与 migration `0017`（默认管理员权限全集修复）及 Dashboard 拆包同期维护。
