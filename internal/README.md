# internal 模块说明

`internal` 是平台后端的内部实现目录。Go 会限制外部项目直接 import 这里的包，因此这里用于承载各限界上下文、启动装配、HTTP server 和版本信息。

## 当前上下文

- `bootstrap`：启动前准备，加载配置、初始化日志、PostgreSQL、Redis 和迁移状态。
- `server`：HTTP 引擎、中间件、路由注册、健康检查和优雅关机。
- `identity`：用户登录、刷新 token、当前用户、RBAC、LDAP/OAuth 和权限管理。
- `alert`：告警接入、Webhook 去重、列表/详情、状态流转和时间线。
- `asset`：应用/资源注册、告警匹配规则、云资源同步上下文。
- `runbook`：处置预案模板、告警推荐、多步骤执行任务生成。
- `execution`：执行任务状态机、人工确认、执行结果、时间线和 Execution Agent。
- `dashboard`：告警、资产、Runbook、执行任务聚合摘要。
- `audit`：关键操作审计查询和导出。
- `ai`：AI Provider、工具网关、告警分析和执行提议。
- `integration`：云账号/观测平台账号、凭据引用、能力声明和连通性测试。
- `observability`：指标、日志、链路、拓扑统一只读查询。
- `inspection`：巡检策略、巡检运行、Finding、Recommendation 和证据链。
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

## 输入与输出

| 模块 | 主要输入 | 主要输出 |
| --- | --- | --- |
| `bootstrap` | `context.Context`、配置文件路径 | `App{Cfg, DB, Redis}` |
| `server` | 配置、鉴权器、路由注册器 | `*gin.Engine`、HTTP 响应 |
| `identity` | HTTP JSON、Bearer Token、用户 ID | token、用户信息、授权判断 |
| `integration` | 账号配置、凭据输入、provider 类型 | account DTO、capability、check result |
| `observability` | `account_id`、查询条件、时间窗口 | series、log entries、trace spans、`evidence_id` |
| `inspection` | 策略 scope、checks、触发来源 | run、finding、recommendation |
| `execution` | task、confirm_text、agent lease | task 状态、步骤结果、日志片段 |
| `version` | 构建变量 | `/version` 使用的版本结构体 |
