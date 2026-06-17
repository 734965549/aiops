# cmd/api 模块说明（粤语版）

## 呢个模块做咩

`cmd/api` 系成个平台嘅「开机掣」。你可以将佢想象成一间餐厅嘅店长：开门之前，店长要检查水电煤、叫齐厨师同服务员、摆好餐牌，最后先正式开门接客。

呢度主要负责：

- 读取启动参数，例如 `-config`（可选；唔传则 env + 默认值）。
- 调用 `internal/bootstrap.Init` 初始化配置、日志、数据库、Redis。
- 装配身份认证模块 `internal/identity` 同 AI 模块 `internal/ai`。
- 启动期 bootstrap 默认管理员、绑定 admin 角色、从配置载入 AI providers。
- 创建 JWT 工具同鉴权器。
- 创建 HTTP Server，监听系统信号，支持优雅退出。

## 调用关系

```text
go run ./cmd/api [-config configs/config.yaml]
  -> bootstrap.Init
      -> pkg/config.Load
      -> pkg/logger.Init
      -> pkg/database.NewPostgres
      -> [仅当 database.auto_migrate=true] database.RunMigrations
      -> pkg/database.NewRedis
  -> pkg/auth.NewJWTManager
  -> internal/identity 装配 repository/service/handler
  -> authSvc.EnsureBootstrapUser + ensureBootstrapAdminRole
  -> internal/ai/toolgateway 注册 executor + seedProvidersFromConfig(cfg.AI.Providers)
  -> internal/server.NewEngine（注入 Cfg、DB、Redis、StartedAt、Authenticator、Registrars）
  -> internal/server.New(...).Run()
```

迁移默认**唔**喺 API 启动时执行；请用 `make migrate` 或 dev compose 覆盖。详见 `ops/migration-contract.md`。

## 入参

| 来源 | 参数 | 说明 |
| --- | --- | --- |
| 命令行 | `-config` | 配置文件路径；唔传则由配置模块搵默认位置或纯 env |
| 配置文件 / env | `app/server/database/redis/auth/logger/cors/ai` | 启动时需要嘅基础配置 |
| 系统信号 | `SIGINT` / `SIGTERM` | 收到之后开始优雅关机 |

## 出参 / 输出

| 输出 | 说明 |
| --- | --- |
| HTTP 服务 | `/healthz`、`/readyz`、`/version`、`/api/identity/*`、`/api/ai/*` |
| 日志 | 打印启动、错误、关机信息 |
| 进程退出码 | 初始化失败会非 0 退出 |

## 通俗比喻

`cmd/api/main.go` 就似「总导演」。佢唔亲自拍戏，但会安排摄影、灯光、演员、场地全部到位；如果任何一个关键角色未准备好，成套戏就唔开拍。
