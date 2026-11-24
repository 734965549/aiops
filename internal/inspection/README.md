# internal/inspection 模块说明

Inspection 提供巡检策略、同步手动运行、规则分析、Finding 和 Recommendation，并可从可执行建议创建 Agent 模式 Execution Task。它不直接连接云 SDK，也不直接执行生产动作；证据经 Observability 获取，真实动作仍受 Execution 状态机、权限、风险、确认和审计约束。契约见 `ops/cloud-observability-contract.md` §6–§7。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 策略 | 创建、列表、详情、部分更新、启停、软删除 | scope.account_id 和 checks 必填；application_ids 校验注册表存在性 |
| 手动运行 | 同一 HTTP 请求内同步采集、分析并保存结果 | 响应通常是 success/partial/failed 终态，不是仅返回 pending 的异步任务 |
| 分析 | 六种检查项的固定阈值规则 | 当前不是 AI Agent 分析；agent_profile 只是元数据 |
| Finding | 风险、摘要、证据引用、置信度、不确定性、嵌套建议 | 受影响资源仍为固定 payment-service 示例，不是完整 scope 映射 |
| Recommendation | open 和 execution_created 状态、can_create_execution | accepted/dismissed 只有枚举，无处理 API；无通知或完成回写 |
| 建议转执行 | 只创建 agent 模式 Execution Task | medium_id 必填，无自动选择；无前端入口、幂等返回或跨模块事务 |
| 定时与取消 | schedule/trigger=scheduled/cancelled 模型字段 | 无调度 worker、取消路由或取消调用点 |
| 权限 | inspections read/write，转执行使用 executions:create | 全部静态 RBAC，无 scope 对象级数据范围过滤 |

## 2. 路由与权限

当前 10 个路由注册、7 个唯一路径模式：

| 方法与路径 | 权限 |
| --- | --- |
| `GET /api/inspections/policies` | `app:inspections:read` |
| `POST /api/inspections/policies` | `app:inspections:write` |
| `GET /api/inspections/policies/:policy_id` | `app:inspections:read` |
| `PUT /api/inspections/policies/:policy_id` | `app:inspections:write` |
| `DELETE /api/inspections/policies/:policy_id` | `app:inspections:write` |
| `POST /api/inspections/policies/:policy_id/runs` | `app:inspections:write` |
| `GET /api/inspections/runs` | `app:inspections:read` |
| `GET /api/inspections/runs/:run_id` | `app:inspections:read` |
| `GET /api/inspections/findings` | `app:inspections:read` |
| `POST /api/inspections/recommendations/:recommendation_id/execution` | `app:executions:create` |

migration `0020` 默认将这些权限绑定 admin。所有路由使用 `AuthorizeStatic`，不按策略 scope、owner_team、application_id 或 region 做数据范围过滤。拥有读权限即可查询策略、运行和发现，拥有 executions:create 即可对满足条件的 Recommendation 请求创建任务，后端不会额外检查 inspections:write。

## 3. 策略语义

`InspectionPolicy.Validate` 要求：

- name 非空。
- `scope.account_id` 非空；账号是否存在/启用要到运行时的 Observability AccountAdapter 才校验。
- checks 至少一项，且只能使用六种已支持代码。

支持检查项：

```text
metrics.cpu
metrics.memory
metrics.disk
traces.latency
traces.error_rate
logs.error_burst
```

其他策略行为：

- `application_ids` 去空、去重；装配 ApplicationCatalog 时逐项检查资产注册表存在，未知 ID 返回 `INVALID_ARGUMENT`。
- provider 省略时由 Observability 按账号解析。
- enabled 默认 true；disabled 策略手动触发返回 412。
- agent_profile 默认 `sre_default`，当前没有 AI Agent 选择或模型调用。
- notification_policy_id 只保存，Notification 上下文和发送流程未落地。
- schedule 只是字符串配置，没有 worker 消费，不能凭填写 cron 期待自动运行。
- 更新是部分字段语义；checks 省略保留，传空数组会校验失败。包含 enabled 的更新审计动作记为 enable/disable，即使同时改了其他字段。
- 删除为软删除，并清空 scope.application_ids 以保持应用引用完整性；Run/Finding/Recommendation 历史不级联删除。

migration `0045` 同步回填历史软删除策略的 application_ids，并调整引用完整性视图。

## 4. 同步运行与状态

`POST /policies/:policy_id/runs` 在请求内执行完整流程：

```text
创建 pending Run + created 时间线
  -> running + started 时间线
  -> 按 checks 逐项收集证据
  -> 规则分析
  -> Findings + Recommendations + 终态 Run 同事务保存
  -> 返回 RunDTO
```

状态语义：

- `success`：所有计划检查均获得证据；可以没有 Finding。
- `partial`：至少一项证据成功，同时有采集失败；保留已采集证据和分析结果。
- `failed`：所有证据均失败，或分析/结果保存失败。
- `cancelled` 只在领域状态机中存在，当前没有取消 API 或调用点。
- `trigger_type=scheduled` 也只是枚举，没有后台调度。

多数成功持久化的运行会以 HTTP 200 返回最终 RunDTO，包括 `status=failed` 的“无证据”结果；前端必须检查 run.status，不能只看 HTTP code。当前页面无论终态如何都使用成功 Toast，failed 也会显示“巡检已触发：failed”。

Findings、Recommendations 和最终 Run 使用 ArtifactUnitOfWork 同事务保存；Run 的创建、running 更新和审计不在该事务内。分析或结果保存失败时会尽量再写 failed 终态，同时向调用方返回错误。

## 5. 证据采集的实际 scope 限制

当前 EvidenceAnalyzer 是固定规则实现，不是通用 scope 驱动的真实云巡检：

- 时间窗口固定为最近 1 小时。
- region 固定为 `cn-north-4`，不是从 Integration 账号 regions 选择。
- Metrics 固定 `namespace=SYS.ECS`、period=60，且没有 dimensions；真实华为 CES 要求 dimensions，因此该路径会失败。
- service 默认 `payment-service`；若配置 application_ids，只把第一个应用 ID 作为 service 字符串。
- environment 和 resource_types 当前不参与证据查询。
- 真实华为 AK/SK 的 logs/traces 也显式 unsupported，所以默认 checks 通常不能形成完整真实云巡检；现有可重复跑通主路径主要是 `huawei_cloud + none` fake。
- Finding 的 affected_resources 固定为 `service/payment-service`，并未按策略应用或资源真实展开。

因此，“Provider 有真实 Adapter”不等于“Inspection 已完成真实云巡检”。真实可用性必须按账号、region、dimensions 和 checks 单独验收，不能用 fake E2E 替代。

## 6. 规则、Finding 与 Recommendation

当前规则：

| 检查项 | 触发条件与风险 |
| --- | --- |
| metrics.cpu | 峰值 >=80 为 high；60–80 为 medium |
| metrics.memory | 峰值 >=75 为 high |
| metrics.disk | 峰值 >=70 为 medium |
| traces.error_rate | 有错误 Span 为 high |
| traces.latency | 有高延迟样本为 medium |
| logs.error_burst | error 摘要 >=2 条为 medium |

每个命中结果包含 evidence_refs、固定 confidence 和 uncertainty 文案；这些值是规则常量，不是模型评估分数。未命中不创建 Finding。

CanCreateExecution 为 true 且 Finding 风险为 high/critical 时，Recommendation 风险会降为 medium，避免直接生成高风险执行任务。当前可执行建议仍必须经 Execution 校验和人工确认，不能把 Finding 风险与任务风险混为一个字段。

Recommendation 当前只有：

- `open`：初始状态。
- `execution_created`：成功创建任务后更新。

accepted/dismissed 仅定义在领域枚举中，没有接受、忽略或通知接口；Execution 完成后也不会回写 Recommendation 状态。

## 7. 建议转 Execution

`POST /api/inspections/recommendations/:recommendation_id/execution` 当前约束：

- Recommendation 必须 `can_create_execution=true`；已为 execution_created 时返回 409，而不是幂等返回原任务。
- execution_mode 省略时默认 agent，其他模式返回 400。
- `medium_id` 在 HTTP 绑定层必填，没有自动介体选择。
- `confirm_intent` 省略或必须为 `create_task_only`，不会直接 dispatch/execute。
- command_spec_id 可省略：按标题中的 disk/磁盘、memory/内存、cpu 选择默认只读 Spec，未命中时兜底 `cmd_linux_disk_usage`；调用方必须复核该默认值是否适合建议。
- 磁盘 Spec 空 arguments 默认 `mount_point=/`；其他参数仍由 Execution 按 Command Spec schema 校验。
- target_type/target_id/target_name/environment 可传，Adapter 固定 source_type=inspection、source_id=recommendation_id、operation_type=command、execution_mode=agent。
- medium/high/critical 风险进入 pending_confirm；low 按 Execution 规则进入 pending_execute。后续确认、lease、日志和结果由 Execution 负责。

Recommendation 不保存 task_id，也没有独立时间线字段；task_id 只在本次响应和 `create_execution` 审计 Payload 中可见。任务创建与 Recommendation 状态更新不在同一事务：如果任务已创建而状态更新失败，接口返回错误但任务可能已存在，重试前必须核对 Execution/审计，避免重复创建。

## 8. 前端页面

`web/src/views/inspections/index.vue` 提供三个 Tab：

- 策略：列表、新建、立即巡检、删除确认；新建支持账号、环境、checks 和 schedule。
- 运行：列表、状态和摘要，可跳转对应 Finding。
- 发现：按 run_id/risk_level 查询，展示证据 ID、置信度和建议摘要。

当前没有：

- 策略编辑、启停按钮、keyword/enabled 筛选或详情页。
- Run 详情时间线、取消或定时 worker 状态。
- Recommendation 接受/忽略、通知、转 Execution 按钮或任务关联展示。
- 真实证据详情、受影响资源真实映射或前端 Vitest。

`inspection.ts` 已封装 `getPolicy`、`getRun` 和更新接口，但页面未使用；转执行 API 尚未封装。页面触发运行时统一显示成功 Toast，即使 Run 终态为 failed，需以实际 status 为准。

## 9. 审计、迁移与验证覆盖

审计均为 best-effort，失败不回滚主业务：

- `inspection_policy`：create/update/delete/enable/disable。
- `inspection_run`：create/start/finish；cancel 常量未使用。
- `inspection_recommendation`：create/create_execution；没有 notify 实现。

migration `0020` 还种子 `inspection.runs.create` 和 `inspection.findings.analyze` AI 工具权限，但当前没有对应的平台内置工具或对话 Agent Loop 注册。通用 `POST /api/ai/tools/invoke` 可在授权后把请求转发给外部 AI Provider，这不等于 Inspection 用例已被调用；权限种子也不代表平台业务工具能力已落地。

已有覆盖：

- domain：Run 状态机和 Policy 校验。
- application：application_id 存在性、Run 启动失败、Findings/Recommendations/Run 事务、空指标和未知检查项。
- HTTP：创建策略成功、触发成功和权限不足。
- `e2e-inspection.ps1`：创建真实 Asset 应用和 fake 账号，创建策略、同步运行、读取 Run 详情、Findings/Recommendations，再删除清理。

该 E2E 不覆盖策略更新/列表/详情、运行列表、partial/failed、取消/定时、权限负向、审计断言、真实云、Recommendation 转 Execution、非事务失败或完整 UI。RecommendationService 当前也没有独立单元测试，前端无 Vitest。

本轮仅静态复核，未执行测试或 E2E，不形成运行通过结论。
