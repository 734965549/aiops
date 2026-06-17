# internal/bootstrap 模块说明（粤语版）

## 呢个模块做咩

`bootstrap` 负责「开机前检查」。就好似开茶餐厅之前，要先开灯、开炉、检查雪柜、准备收银机。未准备好之前，唔应该开始接单。

佢主要做：

- 读取同校验配置。
- 初始化日志。
- 设置系统时区。
- 连接 PostgreSQL。
- 默认不执行迁移；仅 `database.auto_migrate=true` 时调用自研 runner（dev/test 便利）。
- 连接 Redis。
- 提供 `Close()` 统一释放资源。

## 调用关系

```text
make migrate / go run ./cmd/migrate
  -> bootstrap.Migrate(ctx, configPath)
      -> database.RunMigrations(...)

cmd/api/main.go
  -> bootstrap.Init(ctx, configPath)
      -> config.Load(configPath)
      -> cfg.Validate()
      -> logger.Init(...)
      -> database.NewPostgres(...)
      -> database.RunMigrations(...)   # 仅 auto_migrate=true
      -> database.NewRedis(...)
  <- 返回 App{Cfg, DB, Redis}
```

关机时：

```text
cmd/api/main.go
  -> app.Close()
      -> Redis.Close()
      -> database.ClosePostgres(DB)
      -> logger.Sync()
```

## 入参

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `ctx` | `context.Context` | 控制初始化超时同取消 |
| `configPath` | `string` | 配置文件路径，可以为空 |
| 配置内容 | `config.Config` | 数据库、Redis、日志、时区等配置 |

## 出参

| 返回 | 说明 |
| --- | --- |
| `*App` | 包含 `Cfg`、`DB`、`Redis`，畀后面模块继续用 |
| `error` | 配置错、数据库连唔到、迁移失败、Redis 连唔到都会返回 |

## 常见失败情况

- 配置文件路径错。
- 必填配置缺失，例如非 dev 环境 JWT secret 过短、弱密钥或占位值。
- PostgreSQL 未启动或账号密码错。
- migration SQL 有问题。
- Redis 地址唔通。

## 通俗比喻

`bootstrap` 就似「开铺清单」。清单上每项都要打勾：电有冇、煤气有冇、食材有冇、收银机有冇。任何一项关键嘢失败，今日就唔好开门，免得客人入嚟先发现做唔到嘢。