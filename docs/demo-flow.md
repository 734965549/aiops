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

### 8.5 Execution Agent 服务端控制面扩展

本扩展演示的是 **API 控制面 + PowerShell fake Agent**，不是仓库内独立 Agent 在目标机器真实执行命令。AI/前端仍不能直接 SSH 或执行任意 shell。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8.5.1 | 使用 `app:executions:media:create` 创建 jumpbox Medium | 返回 medium_id，能力、环境和风险上限可查询 |
| 8.5.2 | 使用静态 X-Register-Token 注册 fake Agent | 返回随机 Agent Token，数据库只存 hash；注册令牌不是一次性，当前无 mTLS |
| 8.5.3 | Agent Token 心跳 online，创建 manual Agent 任务 | 顶层传 execution_mode=agent、medium_id、command_spec_id、arguments；中高风险 pending_confirm |
| 8.5.4 | 确认前 lease，再输入 CONFIRM 后 lease | 确认前为空，确认后返回 lease_id 和受控 argv；Agent 任务不能调同步 `/execute` |
| 8.5.5 | 回传一条 stdout 和 success result | 服务端保存日志并更新 step/task；日志查询需另调 `/tasks/:id/logs?step_id=`，当前 E2E 不检查读取/脱敏 |
| 8.5.6 | 检查当前边界 | 无自动介体选择、Runbook→Agent、专门前端或真实 Agent；Alert execution_created/started/finished 当前也未闭环 |

自动化脚本已存在：

```powershell
.\scripts\e2e-execution-agent.ps1
.\scripts\e2e-execution-agent-permission.ps1
```

第一个脚本覆盖创建 Medium、注册/心跳、确认前后 lease、日志写入、结果和非法参数；第二个实际只验证错误注册令牌。它们不覆盖真实命令、日志读取/脱敏、审计、幂等、禁用、Alert 时间线或 Runbook→Agent。

---

## 8A. 配置 Integration 接入账号

**页面**：`/integrations`。该页面管理账号与凭据引用，不是实际观测数据查询页面。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8A.1 | 在 dev 创建 `huawei_cloud + none` 账号 | 返回 `acc-...`，无凭据时 `has_credential=false`；华为新账号 extra_config 默认 `sync_mode=ces`，历史空配置账号经 0029 保留 native |
| 8A.2 | 通过安全输入创建 AK/SK 或 api_token 账号 | 响应只有 `has_credential=true`，不回显 credential、密文或 ref；凭据在服务端 AES-GCM 加密，不写文档、日志或前端持久化 |
| 8A.3 | 点击“连通性” | 当前仅做字段校验，不访问云端/SigNoz/Prometheus；ok 不代表云端权限或 capabilities 实测通过 |
| 8A.4 | 构造缺 region/token/base_url 再检查 | 结果可为 HTTP 200 + `data.status=failed`，message 脱敏；页面仍可能显示成功 Toast，必须看业务 status，不能只看提示颜色 |
| 8A.5 | 编辑账号但凭据留空，随后禁用、删除 | 留空保留原凭据，切 none 也不会清除；禁用保留凭据且检查 412，删除软删除账号并硬删除密文，无恢复接口 |
| 8A.6 | 核对页面边界 | 页面无关键字/agency 凭据字段/检查历史/凭据清空，provider 编辑禁用；agency 需通过 API 配置。capabilities 是默认声明，真实查询另按 Observability 验收 |

开发环境可运行：

```powershell
.\scripts\e2e-integration.ps1
```

脚本只覆盖 `huawei_cloud + none` 的创建、列表/详情、检查 ok、更新、viewer 创建 403 和删除，不验证真实凭据或网络、其他 auth_type、检查 failed、审计、密钥轮换或完整 UI。

---

## 8B. 执行 Observability 查询与核对证据

**页面**：`/observability`。先明确账号数据性质，再解释结果，不能把 fake 样本讲成真实云端观测。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8B.1 | 使用 dev 的 `huawei_cloud + none` 账号分别查询指标、日志、链路和拓扑 | 返回确定性 fake 结果和 `ev-<uuid>`；只证明统一 API、权限和证据链路，不代表真实云平台 |
| 8B.2 | 使用真实只读 AK/SK 账号查询一条合法 CES 指标 | 填写账号已配置 region、namespace、1–4 个真实 dimensions 和 CES 支持的 period/filter；公共请求不接收 project_id，由账号或 region_projects 解析。真实日志/链路/拓扑仍 unsupported |
| 8B.3 | 对比 agency、SigNoz 和 Prometheus | 华为 agency 不降级 fake；SigNoz/Prometheus 当前仍为 fake，即使配置 token 也不是在线查询 |
| 8B.4 | 检查普通证据记录 | Metrics 可有 query_params、业务窗口和 collected_at，但 payload 为空；Logs 以脱敏 summary 为主。普通 evidence_id 不等于存在模型定稿快照 |
| 8B.5 | 运行 AI 证据快照专项 | fake 指标先 collecting，定稿 payload/hash 落库后才进入 mock 模型；比较模型输入和数据库规范化 SHA-256，失败证据不作为支持引用 |
| 8B.6 | 检查展示边界 | 页面指标只显示首条序列，证据 ID 仅文本；无公共证据详情 API/抽屉。运行时 `/observability/topology` 与 CMDB `/assets/topology` 不混用 |

开发环境脚本：

```powershell
.\scripts\e2e-observability.ps1
# 另需 migration 0060、本地 mock 模型和 Docker PostgreSQL：
.\scripts\e2e-ai-evidence.ps1
```

两个脚本都使用 fake/mock，不是实际 CES 验收。真实指标需要独立只读账号按 9.2/9.6 专项核对，当前没有专用真实指标 E2E。

---

## 8C. 执行一次手动 Inspection 巡检

**页面**：`/inspections`。当前演示的是同步规则巡检，不是定时或 AI Agent 巡检。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8C.1 | 准备真实 Asset 应用和 dev fake 华为账号，创建含 metrics.cpu、traces.error_rate、logs.error_burst 的策略 | scope.account_id 和 checks 必填，application_ids 必须是已注册应用；agent_profile/schedule/notification_policy_id 只是配置 |
| 8C.2 | 点击“立即巡检” | 请求内同步完成采集和分析，直接返回 success/partial/failed；不是异步 pending。页面即使 failed 也显示成功 Toast，必须看 run.status |
| 8C.3 | 查看 Run 与 Finding | success 表示所有检查有证据，partial 表示部分失败，全部失败为 failed；Finding 带 evidence_refs、固定 confidence/uncertainty 和嵌套建议 |
| 8C.4 | 核对 scope 边界 | 当前固定最近 1h、cn-north-4、SYS.ECS、无 dimensions，真实华为指标会失败；affected_resources 仍为 payment-service 示例，fake 主路径不代表真实云巡检 |
| 8C.5 | 核对自动化边界 | schedule 无 worker、Run 无取消、Recommendation 无接受/忽略/通知或执行完成回写，前端也没有转执行按钮 |
| 8C.6 | 如需 API 演示转执行 | 仅对 can_create_execution=true 的建议调用 execution 接口，必须显式 medium_id，使用 agent + Command Spec；只创建待确认任务，不直接执行。Recommendation 不保存 task_id，失败重试先查 Execution/审计 |

开发环境可运行：

```powershell
.\scripts\e2e-inspection.ps1
```

脚本覆盖 fake 账号下创建应用/账号/策略、同步运行、Run 详情、Finding/Recommendation 和删除清理，不覆盖策略更新/列表、partial/failed、审计、真实云、转执行或 UI。`e2e-execution-agent.ps1` 直接创建 manual Agent 任务，不经过 Inspection Recommendation，不能用来证明转执行已验收。

---

## 8D. 配置 AI Provider 并核对工具网关

**页面**：`/system/ai-config`，以及 `/identity/access-control` 的角色 AI 工具权限区域。演示优先使用本地 mock Provider，不把 mock 结果讲成真实模型验收。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8D.1 | 在 Provider 管理新增类型 b，填写 mock 可达 base_url、model、api_key 和 timeout | 保存到 ai_provider 并同步当前 API 进程；api_key 只保存密文，列表只显示“已配置”，没有掩码值或健康状态 |
| 8D.2 | 留空 key 编辑名称/模型，再禁用和启用 | 原密钥保留；当前实例运行时立即变化。enabled 只是配置状态，保存不做测试连接，多副本不会自动同步本次内存变更 |
| 8D.3 | 通过 API 配置 headers 后在页面普通保存 | headers 被清空，因为页面没有该字段；headers 本身原样保存和返回，不能放 Token/Authorization |
| 8D.4 | 在工具调用调试页选择 Provider 并调用一个已授权 tool_code，再撤销角色工具权限重试 | 前者转发给指定 Provider；后者 HTTP 200 + OK 但 allowed=false，页面展示 reason且不调用 Provider。该页不是对话本地工具调试器 |
| 8D.5 | 核对 confirmed 与数据范围 | require_confirm 默认需确认，但 permits_unconfirmed_invoke=true 可豁免；现有部分 admin 种子带豁免。非 all 数据范围用户直接调 /tools/invoke 会在路由入口 403 |
| 8D.6 | 在 AI 助手触发告警、指标、规则或资源工具 | 8 个内置工具由本地 Port 执行并做对象/账号数据范围校验，不经 AI Provider 转发；只有类型 b 负责模型补全，执行类工具不在 Loop 白名单 |
| 8D.7 | 查看操作审计 | Provider CRUD 没有操作审计；直接 tool_invoke 不含 allowed/reason且拒绝也可能 result=success。对话 tool_invoke 才展示完整 allowed/status/reason，不能混用结果口径 |

`e2e-ai-mock-provider.ps1` 只是其他 AI 脚本的共享 fixture，负责注册和删除保留 ID 的 mock。现有助手、规则、证据和 Loop 脚本主要验证 mock/fake 编排，不覆盖完整 Provider CRUD、直接 `/tools/invoke`、角色工具权限绑定或真实外部模型；部分脚本缺 active 告警会 SKIP 后正常退出，必须看实际输出而不是只看退出码。

---

## 8E. 演示 AI 会话、SSE 与受限 Agent Loop

**页面**：`/ai-assistant`。使用本地 mock 模型和一条 active 告警，先区分 API 能力与当前页面交互缺口。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8E.1 | 新建带 alert_id 的会话并发送问题 | 返回结构化回答、工具观测和执行提议 action；execution.tasks.propose 在 tool_calls 中只是权限校验，真正任务仍要点击 action 调 Execution 创建接口 |
| 8E.2 | 切换模型后再发一条，随后请求已删除/禁用的 provider_id | 页面同时发送顶层与 context.provider_id；有默认模型时回退并在 uncertainties 说明，无默认时 degraded/model_unavailable。当前生成期间选择器实际未禁用 |
| 8E.3 | 同一 client_message_id 重发相同和不同内容，再对新问题换新 key | 相同 key 都重放首次回答，不比较内容 hash，degraded/incomplete 也重放；新 key 才产生新消息。当前重试按钮复用旧 key，不是重新生成 |
| 8E.4 | 归档、重复归档、恢复、删除会话 | 历史保留，同状态不推进 updated_at但仍写状态审计；归档后新提问 409，已有回答仍可重放。当前归档发送会先清空输入，属于已登记缺口 |
| 8E.5 | 观察 SSE 后点击停止，并对比最终 answer 竞态 | stage/tool/delta/answer/error 分事件，thinking delta 可展示文本，answer delta 只计数；取消检查点保留 user/不写 assistant，最终 answer 已落库后的断开不回滚 |
| 8E.6 | 用脚本化 mock 让模型连续申请只读工具和执行类工具 | 8 个内置工具逐轮权限校验，执行申请 denied；最多 3 个追加轮，轮数/token 或连续无进展后先强制收敛，强制成功可正常回答，仍失败才 agent_loop_exhausted |
| 8E.7 | 检查长历史、证据覆盖与当前页面边界 | 详情只取最近 200 条，用户/助手消息为两个事务；同步封装 90s 与后端 600s 预算不同。资源分组规则会先展开实例再按每规则最多 3 个实例抽样，288 点定稿保留峰值时刻/越限时长；小时级聚合与预算仍会限制覆盖，必要时缩窗复查；当前无通用详情刷新，历史重试可能不可用，多轮右侧不是完整时间线 |

可分别运行：

```powershell
.\scripts\e2e-ai-assistant.ps1
.\scripts\e2e-ai-stream.ps1
.\scripts\e2e-ai-agent-loop.ps1
```

三者都是 API/curl + mock/fake，不是浏览器或真实模型验收。assistant 脚本 degraded 仍可结构通过并保留主会话/任务；stream 保留业务会话、`-SkipSlow` 可跳慢流；Agent Loop 无 active 告警会 SKIP 后 exit 0。覆盖与清理边界以验收清单 9.8/9.11/9.12 为准，不要仅看 PASS 或退出码。

---

## 8F. 演示记忆沉淀与知识审核

**页面**：`/ai-assistant` 提问并反馈；`/system/ai-config` 查看「记忆管理」和「知识沉淀」。memory 脚本需真实 provider，knowledge 脚本可用 mock。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8F.1 | 对同一告警得到 `status=ok` 回答后提交「有效」 | `ai_memory_case` 按 user+message 幂等沉淀一条 chat_message 案例；`memory_used/memory_status=ok` 进入 chat_message 审计。当前 UI 对历史 degraded 回答重试可能只重放旧结果，不能当作新案例 |
| 8F.2 | 用相近问题再次提问 | references 出现 memory_case（label 摘要最多 80 字），Prompt 注入最多 3 条；检索只按当前用户 active 案例。管理端自动 valid/invalid 的记忆变化不写独立 memory 审计 |
| 8F.3 | 提交「无效」两次 | quality_score 降到 -2 或更低后转 retired，后续检索不命中；不存在案例时 invalid 是 no-op |
| 8F.4 | 在记忆管理手动停用/启用 | 停用有确认并写 memory_review；启用无确认、质量分不变；同状态幂等不写审计。页面只有 status/keyword/user_id，limit=100，无 application/resource/offset 筛选或分页 |
| 8F.5 | 在知识沉淀新建 scope=application 条目并激活 | manual 固定 draft，激活后 references 含 knowledge_item，回答落库 hit_count+1；invalid 不减 useful_count，只评估低效停用 |
| 8F.6 | 编辑 active 条目 | 回 draft 并清空审核人，但保留计数与 origin；需要重新激活。页面激活/停用无二次确认，审核人/时间/创建者未展示 |
| 8F.7 | 对某应用积累 ≥3 条 valid 案例后聚合 | 生成 `agg-<application_id>` 的 draft；再次聚合 updated，激活后 skipped_active。UI 固定 min_cases=3，未暴露入参；聚合读取当前操作者自己的案例 |

可分别运行：

```powershell
.\scripts\e2e-ai-memory.ps1
.\scripts\e2e-ai-knowledge.ps1
```

memory 脚本不调用管理列表、不校验分数/幂等，首条回答 degraded 会 SKIP 后 exit 0，业务数据保留。knowledge 脚本需要 0052+0054 和 dev/mock，finally 只删除残留条目、retire 残留案例并清理 mock，会话保留；不覆盖 403、编辑回草稿、自动停用或 UI。

---

## 8G. 演示会话只读分享与图片输入

**页面**：`/ai-assistant`。分享需已执行迁移 `0053`，图片需已执行迁移 `0059` 并为类型 b provider 开启「视觉能力」。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8G.1 | 会话列表点「分享」，选 7 天生成链接并复制 | 弹窗展示一次性完整链接；审计 `chat_share_create`；token 只显示一次，关闭弹窗后无法再取 |
| 8G.2 | 用另一具备 `app:ai:chat` 的账号打开链接 | 只读渲染历史与结构化回答，顶部提示不可提问/反馈/执行；无输入框。访问者需已登录，无匿名访问 |
| 8G.3 | owner 撤销后再访问原链接 | 撤销有确认；链接返回 404，页面提示「链接不可用」；审计 `chat_share_revoke`，访问路径日志显示 `/api/ai/chat/shares/<redacted>` |
| 8G.4 | 删除源会话后访问有效分享链接 | 404；分享记录不级联删除，失效由会话不存在保证。同一会话最多 10 条有效链接，第 11 条 409 |
| 8G.5 | 视觉模型下粘贴截图（可附加文字）发送 | 用户气泡显示可预览缩略图；provider payload 为 text + image_url 多模态 content；`chat_message` 审计含 `image_count`，base64 不进日志/审计/Prompt 文本 |
| 8G.6 | 切到未开启视觉的模型带图发送，并构造 4 张/GIF/远程 URL/超 2MB 边界 | 均 400 且不落用户消息、不调用模型；纯图片消息补默认提问「请分析这张图片。」 |
| 8G.7 | 创建带图会话的分享并打开 | 接口返回 `images`，但共享页当前只渲染文本、不展示附图（已登记前端缺口），不能按“分享页可见图”演示 |

分享与图片均无专项浏览器 E2E：分享主路径由 `e2e-ai-assistant.ps1` 自动覆盖（列表/过期/上限需人工或单测），图片报文与拒绝语义依赖 Go/前端单测，理解效果需真实视觉模型。

---

## 8H. 演示全局浮层、跨页连续与消息排队

**页面**：任意登录后页面 + `/alerts`。全部为前端行为（`useChat` 控制器在布局层提供），后端无排队接口。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8H.1 | 任意页面按 `Alt + A` 或点顶栏「AI 助手」 | 右侧抽屉打开并展示当前页面上下文摘要；无激活会话时自动选中最近会话 |
| 8H.2 | 在告警详情点「问 AI 助手」 | 先进入会话选择器；新建会话携带 `alert_id` 上下文；顶栏默认入口不出现选择器 |
| 8H.3 | 提问后未等回答结束切换到其它页面再返回 | 生成过程跨页不中断，返回后消息与阶段提示保留；顶栏按钮和抽屉共用同一控制器 |
| 8H.4 | 生成中连续发送第二、三条 | 进入「排队中」列表；首条完成后自动逐条发送；置顶/删除/清空即时生效 |
| 8H.5 | 排队未清空时点击「停止并清空队列」 | 当前流中止（服务端按取消检查点保留 user、不写 assistant），队列清空且不再自动发送 |
| 8H.6 | 切到另一会话发送，再切回原会话 | 两个会话可同时生成，列表分别显示「生成中」；切回不串消息、不覆盖乐观消息 |
| 8H.7 | 排队期间刷新页面 | 排队条目与乐观消息丢失（不持久化）；正在生成的回合由服务端按取消或完成落库，不能把浮层当作可恢复工作区 |

覆盖说明：选择器/上下文联动由 `AIAssistantOverlay.spec.ts`、`aiContext.spec.ts` 覆盖；并发与排队由 `useChat.spec.ts` 覆盖；停止清队、队列管理、快捷键无专项测试，需人工演示。排队为纯前端行为，同一会话的服务端并发仍由幂等语义兜底。

---

## 8I. 演示 RDS 长事务历史会话诊断

**前置**：一条 `SYS.RDS + oldest_transaction_duration` 长事务告警、带 `alerts` 能力的华为 `ak_sk` 账号、支持历史会话的 RDS 实例（需 IAM `rds:instance:getHistorySession/getWaitEvent/getTopSql`）；mock 模型即可。

| 步骤 | 操作 | 预期 |
|------|------|------|
| 8I.1 | 打开长事务告警详情，点「问 AI 助手」新建会话提问 | gather 自动取数（工具权限 + 账号数据范围校验）；回答出现「诊断结论」区块：状态 Tag + 根因陈述 + 证据 ID；事实含会话、Query ID 与匹配 Top SQL 的脱敏摘要 |
| 8I.2 | 核对 `idle in transaction` 场景文案 | 页面显示“分类已确认”而非“已确认根因”；应用原因仍未定并列出缺失 `application_logs`；`Client/ClientRead` 附注“不是锁等待” |
| 8I.3 | 核对失败/无现场降级 | 采集失败或无可分类会话时显示“根因未定” + 缺失证据 + 三类待验证假设；不出现确定的根因结论 |
| 8I.4 | 用 `scripts/e2e-postgres-history.ps1 -AlertId <告警ID>` 复跑 | 输出 `root_status/root_code/evidence_ids/missing_evidence` 与前 6 条事实；权限不足显式失败，不伪造成功；脚本只清理 mock，会话与证据保留 |
| 8I.5 | 对同一条告警走告警页 AI 分析弹窗 | 弹窗只有风险/摘要/建议/引用，**没有诊断区块**；完整结构化诊断以 AI 助手回答卡为准（已登记前端差距） |

注意：诊断读取的是华为已采样的历史窗口（最长 7 天），不是平台在告警时刻的主动快照；证据快照写库失败时按 fail-closed 转为 error，不会把未持久化现场当事实。

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

补充边界：

- `processing_alerts` 实际包含 new/acknowledged/processing，前端再取前 3 条分别获取推荐，不是仅 processing。
- 后端单项统计失败会降级为 0/空数组，响应没有 degraded 标记，真实 0 与降级 0 不可区分。
- 前端 summary 整体失败时指标显示 `—`，没有专门错误 Alert；`/version` 与 `/readyz` 独立展示。
- 当前没有专用 Dashboard E2E 或前端 Vitest；`e2e-identity-access.ps1` 只以 viewer 调一次摘要，不能替代完整 UI 和降级验收。

---

## 10. 查看审计日志

资产变更、告警处理、Runbook 启停、执行确认/拒绝、AI 分析等已接入路径会尝试写操作审计。当前多数写入为 best-effort，业务成功不等于审计必定落库；登录、refresh、logout 则写入 Identity 独立认证审计，不出现在 `/api/audits`。

**前端查询**：进入 `/audits`，按资源类型、资源 ID、用户 ID、动作或 Trace 精确筛选，点击“详情”查看 Payload。Resource/User/Trace 聚合只统计当前页；CSV 实际最多导出当前筛选第一页 100 条，不是全量导出。

资源类型下拉尚有差距：`asset/identity/integration/observability/inspection` 与实际写入类型不一致，多个真实类型也没有快捷选项。演示这些模块时直接用 API 传真实类型，不要把下拉无结果解释为没有审计。

**API 查询**：

```powershell
curl.exe -H "Authorization: Bearer <token>" "http://127.0.0.1:8080/api/audits?page=1&page_size=100&resource_type=execution"
curl.exe -H "Authorization: Bearer <token>" "http://127.0.0.1:8080/api/identity/admin/auth-audits?page=1&page_size=20"
```

第一个接口要求 `app:audits:read`，第二个要求 `app:identity.auth_audits:read`。操作审计筛选近期操作时，可按真实目录确认：

| 模块 | `resource_type` | 典型 action |
|------|-----------------|-------------|
| Identity 权限管理 | `identity_user`、`identity_role` | `set_user_roles`、`set_role_permissions` |
| 认证审计（独立接口） | 不适用 | `login`、`refresh`、`logout` |
| 资产 | `application`、`resource`、`asset_sync_batch` | `create/update/delete`、`sync` |
| 告警 | `alert`、`alert_source` | `ingest`、`acknowledge`、`close`、`alert_source_sync` |
| Runbook | `runbook`、`alert` | `runbook_create`、`runbook_enable`、`runbook_recommend` |
| 执行 | `execution`、`alert` | `create`、`confirm`、`reject`、`execute`、`execution_create` |
| AI | `ai` | `analyze_alert`、`chat_message`、`tool_invoke` |
| Integration | `integration_account` | `create`、`update`、`check` |
| Observability | `observability_query` | `metrics_query`、`logs_search` |
| Inspection | `inspection_policy`、`inspection_run` | `create`、`start`、`finish` |

完整目录和边界见 `ops/audit-contract.md`。标准操作审计 recorder 当前不传 IP/User-Agent；页面掩码只影响详情和 CSV，不能据此宣称数据库或原始 API 已完成统一脱敏。

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
5. **闭环**：「普通模拟执行结果回写告警时间线；Dashboard 实时汇总；已接入动作可在操作审计中追溯，认证事件走独立认证审计。审计写入多为 best-effort，验收时需单独核对落库。」

---

**文档版本**：与 migration `0017`（默认管理员权限全集修复）及 Dashboard 拆包同期维护。
