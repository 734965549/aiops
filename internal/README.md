# internal 模块说明（粤语版）

## 呢个目录做咩

`internal` 系项目嘅「后台办公室」。Go 语言规定 `internal` 入面嘅代码只畀本项目自己用，外面项目唔应该直接 import。就好似餐厅嘅后厨，客人只会见到出品，唔会直接入厨房指挥。

当前包含：

- `bootstrap`：启动前准备，配置、日志、数据库、Redis。
- `server`：HTTP 引擎、中间件、路由注册、优雅关机。
- `identity`：用户登录、刷新 token、当前用户。
- `alert`：告警接入（Webhook）、去重、列表/详情、状态流转（契约见 `ops/alert-contract.md`）。
- `asset`：应用/资源注册、告警匹配规则、资产上下文。
- `runbook`：处置预案模板、告警推荐、多步骤执行任务生成。
- `execution`：执行任务状态机、人工确认、执行结果同时间线。
- `dashboard`：告警、资产、Runbook、执行任务聚合摘要。
- `audit`：关键操作审计查询同导出。
- `ai`：AI Provider、工具网关、告警分析同执行提议。
- `integration`：云账号/观测平台账号、凭据引用、能力声明、连通性测试。
- `observability`：指标、日志、链路、拓扑统一只读查询。
- `inspection`：巡检策略、巡检运行、Finding、Recommendation 同证据链。
- `version`：版本号、commit、构建时间。

## 总体调用关系

```text
cmd/api
  -> internal/bootstrap 初始化基础设施
  -> internal/identity / alert / asset / runbook / execution 装配 P0 闭环
  -> internal/integration / observability / inspection 装配只读观测同巡检链路
  -> internal/server 注册路由同启动 HTTP
  -> internal/version 提供版本信息
```

HTTP 请求入嚟之后：

```text
客户端
  -> internal/server 中间件
  -> internal/identity/interfaces/http Handler
  -> internal/identity/application Service
  -> internal/identity/domain Repository 接口
  -> internal/identity/infrastructure Repository 实现
  -> PostgreSQL
```

新观测链路入嚟之后：

```text
客户端
  -> internal/integration/interfaces/http 注册账号同做连通性测试
  -> internal/observability/application 透过 IntegrationAccountPort 解析账号摘要
  -> internal/observability/infrastructure/provider 路由 fake / 华为云 / SigNoz / Prometheus adapter
  -> internal/inspection/application 收集证据、生成 Finding 同 Recommendation
  -> internal/execution/application 只喺用户确认后先派发真实执行
```

完整串联图见 `docs/AI运维平台整体流程与调用关系.md`。改跨上下文代码前，先确认自己系用 Port 调用，定系误穿到对方 infrastructure。

## 主要入参

| 模块 | 入参 | 说明 |
| --- | --- | --- |
| `bootstrap` | `context.Context`、配置文件路径 | 控制启动超时，加载配置 |
| `server` | 配置、鉴权器、路由注册器 | 创建 Gin Engine 同 HTTP Server |
| `identity` | HTTP JSON、Bearer Token、用户 ID | 完成登录、刷新、查当前用户 |
| `integration` | 账号配置、凭据输入、provider 类型 | 凭据加密保存引用，返回脱敏账号信息 |
| `observability` | `account_id`、查询条件、时间窗口 | 校验能力后查询指标/日志/链路/拓扑 |
| `inspection` | 策略 scope、checks、触发来源 | 收集证据并产出发现同建议 |
| `version` | 构建变量 | 编译时注入或使用默认值 |

## 主要出参

| 模块 | 出参 | 说明 |
| --- | --- | --- |
| `bootstrap` | `App{Cfg, DB, Redis}` | 后续模块装配所需资源 |
| `server` | `*gin.Engine`、HTTP 响应 | 对外处理请求 |
| `identity` | token、用户信息、业务错误 | 身份认证结果 |
| `integration` | account DTO、capability、check result | 只暴露业务 ID 同 `has_credential` |
| `observability` | series、log entries、trace spans、evidence_id | 只读观测结果同证据引用 |
| `inspection` | run、finding、recommendation | 可追溯巡检结果，后续可转 Execution |
| `version` | 版本结构体 | `/version` 接口使用 |

## 通俗比喻

`internal` 就系公司内部运作部门：有行政负责开门准备，有保安负责验身份，有业务部门负责做事，有前台负责接待。外部客户只同前台沟通，但真正做事喺内部完成。
