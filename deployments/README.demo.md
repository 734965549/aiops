# 专属演示环境（aiops-demo）

本文档说明如何用**完全隔离**的 Docker 环境运行本平台，不触碰宿主机上已有的其他 PostgreSQL 实例。

## 隔离设计

宿主机上已存在一个 PostgreSQL（容器 `lianghua-pg`，pgvector 镜像，映射宿主机 `5432`）。若直接按 README 执行
`docker compose -f deployments/docker-compose.yml ... up -d`，基线文件会把 postgres 映射到宿主机 `5432`，
**直接端口冲突**。因此本环境通过四层隔离解决：

| 隔离维度 | 手段 | 效果 |
|---|---|---|
| 网络 | compose project `aiops-demo` | 独立网络 `aiops-demo_aiops-net`，与 `lianghua_default` 无互通 |
| 数据卷 | project 前缀 + 显式命名 | 独立卷 `aiops-demo_aiops-demo-pg-data`，不触碰 `lianghua_lianghua_pg_data` |
| 容器名 | `aiops-demo-*` | 与默认 `aiops-postgres/aiops-redis/aiops-api/aiops-web` 不冲突 |
| 端口 | `!override` 强制替换 | PG→`5433`、Redis→`6380`、API→`8080`、前端→`5173` |

> **关键点**：Compose 对列表型字段（如 `ports`、`volumes`）的 merge 语义是**追加**而非**替换**。
> 直接在覆盖文件里写 `ports: ["5433:5432"]` 会保留基线的 `5432:5432`，导致端口冲突。
> 必须使用 `!override` 标签（Compose v2.24+ / v5.1.4 已支持）强制替换。

## 端口分配

| 服务 | 宿主机端口 | 容器内端口 | 说明 |
|---|---|---|---|
| 前端 (nginx) | 5173 | 8080 | 与前端 dev 默认端口一致 |
| API | 8080 | 8080 | `/api`、`/healthz`、`/readyz`、`/version` |
| PostgreSQL | **5433** | 5432 | 避开宿主机已有的 5432 |
| Redis | 6380 | 6379 | |

## 启动

```bash
docker compose -p aiops-demo \
  -f deployments/docker-compose.yml \
  -f deployments/docker-compose.dev.yml \
  -f deployments/docker-compose.web.yml \
  -f deployments/docker-compose.demo.yml \
  up -d --build
```

首次构建需拉取 `golang:1.26-alpine`、`node:20-alpine`、`nginxinc/nginx-unprivileged:1.27-alpine`。
若 Docker Hub 拉取出现 `failed to fetch anonymous token ... EOF`（网络抖动），手动重试即可：

```bash
for img in golang:1.26-alpine node:20-alpine nginxinc/nginx-unprivileged:1.27-alpine; do
  for i in 1 2 3 4 5; do docker pull "$img" && break; sleep 3; done
done
```

## 访问

- 前端：<http://127.0.0.1:5173>
- API：<http://127.0.0.1:8080>
- 就绪检查：<http://127.0.0.1:8080/readyz>
- 数据库：`postgresql://aiops:aiops@127.0.0.1:5433/aiops`

默认账号：`admin` / `admin123`（dev 覆盖注入，JWT 为 dev 占位密钥，仅限本机）。

## 演示数据

`deployments/demo-seed/demo_seed.sql` 与 `deployments/demo-seed/demo_seed2.sql` 构造了一套连贯的电商运维场景：

- 5 个应用（支付网关 / 订单中心 / 用户中心 / 商品搜索 / 接入网关）、6 个资源、4 条匹配规则
- 4 个 Runbook 模板 + 12 个步骤
- 5 个告警实例（含 p0/p1/p2/p3、processing/acknowledged/new/recovered/closed 各状态）+ 15 条事件时间线
- 3 个历史执行任务（含 succeeded 与 pending_confirm）及执行步骤

导入方式：

```bash
docker exec -i aiops-demo-postgres psql -U aiops -d aiops -v ON_ERROR_STOP=1 -f - < deployments/demo-seed/demo_seed.sql
docker exec -i aiops-demo-postgres psql -U aiops -d aiops -v ON_ERROR_STOP=1 -f - < deployments/demo-seed/demo_seed2.sql
```

脚本全部使用 `ON CONFLICT DO NOTHING`，可重复执行。**注意**：`docker cp` 在 Git Bash 下会被转换成本机路径
（`C:/Users/...`）而失败，请使用 `docker exec -i ... < file` 的 stdin 方式。

## 停止与清理

```bash
# 停止并保留数据（推荐先这样）
docker compose -p aiops-demo \
  -f deployments/docker-compose.yml \
  -f deployments/docker-compose.dev.yml \
  -f deployments/docker-compose.web.yml \
  -f deployments/docker-compose.demo.yml \
  down

# 停止并删除本 project 的网络与数据卷（不会影响其他 project）
docker compose -p aiops-demo \
  -f deployments/docker-compose.yml \
  -f deployments/docker-compose.dev.yml \
  -f deployments/docker-compose.web.yml \
  -f deployments/docker-compose.demo.yml \
  down -v
```

`-v` 只删除 `aiops-demo_*` 前缀的卷，`lianghua_lianghua_pg_data` 不受影响。

## 验证结果

主链路 e2e 全部通过（`scripts/e2e-alert.ps1`、`e2e-asset.ps1`、`e2e-runbook.ps1`、`e2e-execution.ps1`）：

```
告警接入 -> 资产匹配 -> Runbook 推荐 -> 执行确认 -> Dashboard 汇总 -> 审计追溯
```

迁移版本 `0045`，`/readyz` 的 config / migration / db / redis 子项均为 `ok`。
