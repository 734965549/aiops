# migrations 模块说明（粤语版）

## 呢个目录做咩

`migrations` 系数据库结构嘅「装修图纸」。代码需要边啲表、边啲字段、边啲索引，都用 SQL 记录低。新环境启动时，可以按图纸自动装修数据库。

## 调用关系

```text
make migrate / go run ./cmd/migrate
  -> bootstrap.Migrate
  -> database.RunMigrations(ctx, db, MigrateOptions{Dir})
  -> 读取 migrations/*.up.sql
  -> 写入 schema_migrations
  -> PostgreSQL 建表/建索引
```

**仅当 `database.auto_migrate=true` 时**，`bootstrap.Init`（`cmd/api` 启动路径）才会在连接 PostgreSQL 后调用同一 `RunMigrations`。
默认值为 `false`：生产与常规本地联调应使用 `make migrate` / `make migrate-up` 显式建表，避免 API 启动时隐式迁移。

受控 dev/test 可临时开启 `auto_migrate`（例如叠加 `deployments/docker-compose.dev.yml`），与 `make migrate` **二选一**。

平台禁止与 golang-migrate 混用，详见 `ops/migration-contract.md`。

回滚文件：

```text
*.down.sql
  -> 描述点样撤销对应 up migration
```

## 入参

| 入参 | 说明 |
| --- | --- |
| migration 目录 | SQL 文件所在目录 |
| `*.up.sql` | 正向迁移，例如建表 |
| `*.down.sql` | 反向迁移，例如删表 |
| PostgreSQL 连接 | 执行 SQL 嘅目标数据库 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 数据库表结构 | 例如 `iam_user`、`iam_role`、`iam_permission`、`iam_user_role`、`iam_role_permission`、`iam_data_scope`、`iam_role_data_scope`、`iam_ai_tool_permission`、`iam_role_ai_tool_permission`、`schema_migrations` |
| migration 版本记录 | 防止同一条 SQL 重复执行 |
| error | SQL 错、权限不足、连接断开等 |

## 通俗比喻

数据库 migration 就似装修施工记录：今日装咗门，听日铺咗地板，每一步都有编号。以后换地方开分店，只要照住记录施工，就可以装修出同一间铺。