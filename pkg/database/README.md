# pkg/database 模块说明（粤语版）

## 呢个模块做咩

`database` 系平台嘅「资料库连接员」。佢负责同 PostgreSQL、Redis 打交道，仲负责执行 migration，确保数据库表结构准备好。

## 调用关系

```text
bootstrap.Migrate / bootstrap.Init（auto_migrate=true）
  -> database.RunMigrations(ctx, db, options)

bootstrap.Init
  -> database.NewPostgres(ctx, cfg.Database, timezone)
  -> database.NewRedis(ctx, cfg.Redis)

identity/infrastructure
  -> 使用 *gorm.DB 读写 iam_user 表
```

## 入参

| 方法 | 入参 | 说明 |
| --- | --- | --- |
| `NewPostgres` | context、数据库配置、时区 | 创建 GORM PostgreSQL 连接 |
| `ClosePostgres` | `*gorm.DB` | 关闭数据库连接 |
| `RunMigrations` | context、DB、migration 目录 | 执行 SQL 迁移 |
| `NewRedis` | context、Redis 配置 | 创建 Redis client |

## 出参

| 输出 | 说明 |
| --- | --- |
| `*gorm.DB` | 业务仓储使用嘅数据库连接 |
| `*redis.Client` | 缓存、会话、限流等未来能力使用 |
| migration 结果 | 写入 `schema_migrations`，避免重复执行 |
| error | 连接失败、SQL 执行失败、目录唔存在等 |

## 通俗比喻

PostgreSQL 就似档案室，Redis 就似前台便签纸。重要资料放档案室，临时快速查嘅资料贴喺便签纸。`database` 就系负责帮你开门、摆好文件柜、检查便签纸仲用唔用得。