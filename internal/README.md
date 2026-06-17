# internal 模块说明（粤语版）

## 呢个目录做咩

`internal` 系项目嘅「后台办公室」。Go 语言规定 `internal` 入面嘅代码只畀本项目自己用，外面项目唔应该直接 import。就好似餐厅嘅后厨，客人只会见到出品，唔会直接入厨房指挥。

当前包含：

- `bootstrap`：启动前准备，配置、日志、数据库、Redis。
- `server`：HTTP 引擎、中间件、路由注册、优雅关机。
- `identity`：用户登录、刷新 token、当前用户。
- `alert`：告警接入（Webhook）、去重、列表/详情、状态流转（契约见 `ops/alert-contract.md`）。
- `version`：版本号、commit、构建时间。

## 总体调用关系

```text
cmd/api
  -> internal/bootstrap 初始化基础设施
  -> internal/identity / internal/alert 装配业务能力
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

## 主要入参

| 模块 | 入参 | 说明 |
| --- | --- | --- |
| `bootstrap` | `context.Context`、配置文件路径 | 控制启动超时，加载配置 |
| `server` | 配置、鉴权器、路由注册器 | 创建 Gin Engine 同 HTTP Server |
| `identity` | HTTP JSON、Bearer Token、用户 ID | 完成登录、刷新、查当前用户 |
| `version` | 构建变量 | 编译时注入或使用默认值 |

## 主要出参

| 模块 | 出参 | 说明 |
| --- | --- | --- |
| `bootstrap` | `App{Cfg, DB, Redis}` | 后续模块装配所需资源 |
| `server` | `*gin.Engine`、HTTP 响应 | 对外处理请求 |
| `identity` | token、用户信息、业务错误 | 身份认证结果 |
| `version` | 版本结构体 | `/version` 接口使用 |

## 通俗比喻

`internal` 就系公司内部运作部门：有行政负责开门准备，有保安负责验身份，有业务部门负责做事，有前台负责接待。外部客户只同前台沟通，但真正做事喺内部完成。