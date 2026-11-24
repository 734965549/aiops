# internal/bootstrap 模块说明

`bootstrap` 负责装配平台运行所需的基础组件（配置、日志、PostgreSQL、Redis），向 main 返回就绪的 `App`；同时提供独立迁移入口 `Migrate`，供 `cmd/migrate`、`cmd/api -migrate` 与发布流水线显式调用。这样 main 不承担基础设施细节，集成测试也可复用同一份初始化流程。

## 对外接口

| 接口 | 说明 |
| --- | --- |
| `Init(ctx, configPath)` | 加载并校验配置 -> 初始化日志 -> 连接 PG（`database.auto_migrate=true` 时执行迁移，仅限受控 dev/test）-> 连接 Redis；返回 `App{Cfg, DB, Redis, MigrationDir}` |
| `Migrate(ctx, configPath)` | 仅连接 PostgreSQL 并执行 `migrations/*.up.sql`（自研 runner）；生产 schema 变更走此路径，不在 API 启动时隐式迁移 |
| `App.Close()` | 释放 Redis/PG 连接并 `logger.Sync()`；幂等 |

## 启动顺序与失败语义

```text
config.Load -> cfg.Validate     配置错误先失败，避免基础组件起来后才发现致命问题
  -> logger.Init                后续日志均带 trace 字段
  -> database.NewPostgres       连接失败 -> error
  -> auto_migrate=true 时 RunMigrations（超时缺省 5 分钟，失败关闭 PG 并返回 error）
     auto_migrate=false 时 Warn "skipping migrations"
  -> Redis：required=true 且不可用 -> error；optional 且不可用 -> Warn + rdb=nil 继续启动
```

- 迁移目录由 `database.ResolveMigrationDir()` 解析，`App.MigrationDir` 供 `/readyz` 的 migration 子项使用。
- Redis 为 nil 时，上层装配按用途降级：refresh 轮换禁用、告警幂等退内存、LDAP 浏览/OAuth state 退内存——均为 dev-only，`redis.required=true` 时直接 Fatal。
- 常见失败：配置路径/必填项错误（非 dev JWT secret 过短或占位值）、PG 未启动或凭据错、迁移 SQL 错误、Redis 地址不可达（required 时）。

## 调用关系

```text
make migrate / go run ./cmd/migrate / cmd/api -migrate
  -> bootstrap.Migrate(ctx, configPath) -> database.RunMigrations

cmd/api/main.go
  -> bootstrap.Init(ctx, configPath) -> App
  -> defer app.Close()
```

迁移行为与契约见 `ops/migration-contract.md`；本模块自身无专项单元测试，迁移语义由 `pkg/database` 测试覆盖。
