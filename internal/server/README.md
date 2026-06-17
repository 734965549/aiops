# internal/server 模块说明（粤语版）

## 呢个模块做咩

`server` 系 HTTP 服务嘅「前台大堂经理」。所有客人请求都会先经过大堂经理：登记 trace、处理异常、记录访问日志、处理跨域、检查身份，然后先分派去对应业务窗口。

主要负责：

- 创建 Gin Engine。
- 注册全局中间件。
- 注册健康检查、就绪检查、版本接口。
- 注册业务模块路由，例如 `identity`。
- 包装 `http.Server`，支持启动同优雅关机。

## 调用关系

```text
cmd/api/main.go
  -> server.NewEngine(Options)
      -> 注册 Trace / Recovery / RequestLog / CORS 中间件
      -> 注册 /healthz /readyz /version
      -> 调用各个 RouteRegistrar.RegisterRoutes(...)
  -> server.New(cfg.Server, engine)
  -> srv.Run()
```

请求处理链：

```text
客户端 HTTP 请求
  -> Gin Engine
  -> Trace 中间件：派 trace_id
  -> Recovery 中间件：防 panic 打爆服务
  -> RequestLog 中间件：记录请求日志
  -> CORS 中间件：处理跨域
  -> Auth 中间件：需要登录嘅路由检查 Bearer Token
  -> 业务 Handler
```

## 入参

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `Options.Cfg` | `*config.Config` | Server、CORS、App 等配置 |
| `Options.DB` | `*gorm.DB` | PostgreSQL 连接；`/readyz` 会检查 migration 同 db ping |
| `Options.Redis` | `*redis.Client` | Redis 连接；`/readyz` 会 ping |
| `Options.StartedAt` | `time.Time` | 进程启动时刻；用于 `uptime_ms`，零值时退化为路由注册时 |
| `Options.Authenticator` | 鉴权接口 | 用嚟验证 access token |
| `Options.Registrars` | `[]RouteRegistrar` | 各业务模块嘅路由注册器 |
| `ServerConfig` | 配置结构 | 地址、端口、读写超时、关机超时 |

## 出参

| 输出 | 说明 |
| --- | --- |
| `*gin.Engine` | 已经装好中间件同路由嘅 HTTP 引擎 |
| HTTP JSON | 健康检查、版本、业务接口响应 |
| `error` | 端口占用、启动失败、关机失败等 |

## 通俗比喻

`server` 就似商场入口：客人入嚟要过安检、问询台、指示牌。真正买嘢唔喺入口买，但入口决定咗客人应该去边层、边间铺。