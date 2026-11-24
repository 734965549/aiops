# internal 模块说明

`internal` 是平台后端的内部实现目录。Go 会限制外部项目直接 import 这里的包，因此这里用于承载各限界上下文、启动装配、HTTP server 和版本信息。

## 当前上下文

- `bootstrap`：启动前准备，加载配置、初始化日志、PostgreSQL、Redis 和迁移状态。
- `server`：HTTP 引擎、中间件、路由注册、健康检查和优雅关机。
- `identity`：本地/LDAP/OAuth 登录、token 生命周期、当前用户、RBAC、数据范围、AI 工具权限和权限管理。
- `alert`：Webhook 去重接入、CES 手动/可选后台拉取、列表/详情、认领/转派/处理/恢复/关闭、静默、评论和时间线；详见 `internal/alert/README.md`。
- `asset`：应用/资源注册、资源解析、匹配规则、异步云同步与批次、CMDB 拓扑、手工及云派生关系；详见 `internal/asset/README.md`。
- `runbook`：处置预案模板、告警推荐、多步骤执行任务生成；详见 `internal/runbook/README.md`。
- `execution`：任务状态机、人工确认/拒绝、模拟执行、Runbook、时间线，以及 Medium/Command Spec/Agent 控制面；独立 Agent 与专门 UI 未提供，详见 `internal/execution/README.md`。
- `dashboard`：告警、资产、Runbook、执行任务聚合摘要；详见 `internal/dashboard/README.md`。
- `audit`：关键操作审计写入与精确分页查询；前端聚合仅基于当前页，CSV 实际最多 100 条，认证审计由 Identity 独立提供；详见 `internal/audit/README.md`。
- `ai`：Provider、工具网关、告警分析、多轮会话、SSE/取消、受限 Agent Loop、记忆/知识、分享、多模态、证据诊断和执行提议；Provider 无在线健康检查，直接转发与对话本地工具分开验收；会话详情最近 200 条，重试/归档/模型交互仍有前端缺口；记忆/知识管理筛选与展示有限，详见 `internal/ai/README.md`。
- `integration`：云账号/观测平台账号、AES-GCM 凭据引用、默认能力声明和字段级连接检查；当前 checker 不访问外部服务，capabilities 不是实测结果，详见 `internal/integration/README.md`。
- `observability`：指标/日志/链路/拓扑统一只读查询，以及资源发现、CES 告警与规则、批量规则体检、RDS 历史会话客户端和 AI 定稿快照；真实 Provider、fake、普通证据与模型 payload 边界详见 `internal/observability/README.md`。
- `inspection`：手动同步规则巡检策略、运行、Finding、Recommendation 和后端转执行；真实 scope 采集有限，定时 worker/取消/通知未落地，转执行无前端入口、自动介体或完成回写，详见 `internal/inspection/README.md`。
- `version`：版本号、commit、构建时间。

## 总体调用关系

```text
cmd/api
  -> internal/bootstrap 初始化基础设施
  -> internal/identity 装配鉴权与授权
  -> internal/audit 装配统一审计
  -> internal/alert / asset / runbook / execution / dashboard / ai 装配 P0 闭环
  -> internal/integration / observability / inspection 装配只读观测与巡检链路
  -> internal/server 注册路由并启动 HTTP
  -> internal/version 提供版本信息
```

HTTP 请求进入后的典型路径：

```text
客户端
  -> internal/server 中间件
  -> internal/<context>/interfaces/http Handler
  -> internal/<context>/application Service
  -> internal/<context>/domain Repository 接口
  -> internal/<context>/infrastructure Repository 实现
  -> PostgreSQL / Redis / Provider Adapter
```

只读观测链路：

```text
客户端
  -> internal/integration/interfaces/http 注册账号或做连通性测试
  -> internal/observability/application 通过 IntegrationAccountPort 解析账号摘要
  -> internal/observability/infrastructure/provider 路由 fake / 华为云 / SigNoz / Prometheus adapter
  -> internal/inspection/application 收集证据、生成 Finding 与 Recommendation
  -> internal/execution/application 只在用户确认后派发真实执行
```

完整串联图见 `docs/AI运维平台整体流程与调用关系.md`。改跨上下文代码前，应确认调用是通过 application Port 完成，而不是直接穿透到对方 infrastructure。

## 运行时进程与后台任务

| 运行单元 | 当前形态 | 边界 |
| --- | --- | --- |
| `cmd/api` | 主 HTTP 进程，装配全部业务上下文 | Asset Sync 在 API 进程内以后台 goroutine 执行，批次状态持久化 |
| CES 告警拉取 worker | 主 API 内可选后台 worker | `alert_sync.worker_enabled=false` 为默认；多实例只应在一个实例启用 |
| `cmd/ai-orchestrator` | 可选 type=c 内部 AI 编排参考服务 | 只处理 `alarm.analyze`，不落库、不执行动作；AI 助手主对话使用 type=b |
| Execution Agent | 外部执行端，通过 API 注册、心跳、领取租约和回传 | 仓库提供服务端控制面与 fake E2E，不提供独立 Agent 可执行程序 |

## 输入与输出

| 模块 | 主要输入 | 主要输出 |
| --- | --- | --- |
| `bootstrap` | `context.Context`、配置文件路径 | `App{Cfg, DB, Redis}` |
| `server` | 配置、鉴权器、路由注册器 | `*gin.Engine`、HTTP 响应 |
| `identity` | 登录凭据、Bearer Token、用户/角色/权限配置 | TokenPair、用户信息、授权判断、权限管理结果 |
| `alert` | Webhook payload、告警源与拉取配置、状态操作 | 告警、时间线、同步结果、审计事件 |
| `asset` | 应用/资源/规则、同步请求、拓扑与关系查询 | 资产分页、同步批次、资源解析、拓扑图 |
| `runbook` | 模板、告警上下文、推荐查询 | 模板与结构化步骤、推荐结果 |
| `execution` | task、confirm_text、Command Spec、agent lease/log/result | 任务状态、步骤结果、租约与日志片段 |
| `dashboard` | 聚合查询请求 | 告警/执行/资产/Runbook 摘要 |
| `audit` | 资源、用户、动作、Trace 与分页条件 | 审计分页结果 |
| `ai` | Provider、会话消息、图片、页面上下文、工具参数 | 结构化回答、工具结果、证据引用、待确认执行提议 |
| `integration` | 账号配置、凭据输入、provider 类型 | account DTO、capability、check result |
| `observability` | `account_id`、查询条件、时间窗口、采集请求 | series/log/trace/topology 摘要、资源/告警数据、证据引用与 AI 定稿快照 |
| `inspection` | 策略 scope、checks、手动触发与建议转执行请求 | run、finding、recommendation、待确认 Execution Task |
| `version` | 构建变量 | `/version` 使用的版本结构体 |
