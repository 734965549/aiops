# deployments 模块说明（粤语版）

## 呢个目录做咩

`deployments` 系部署相关文件嘅「搬屋清单」。代码写好之后，要点样打包、点样连数据库同 Redis、点样一键起服务，就靠呢度。

## 调用关系

```text
Dockerfile
  -> 编译 Go 后端（输出 /app/aiops-api 与 /app/aiops-migrate）
  -> 复制 migrations/（运行时供 /readyz 判读或 auto_migrate）
  -> 默认唔复制 config.yaml（运行时靠环境变量）

docker-compose.yml（主文件）
  -> 启动 aiops-api（纯 env，默认唔自动迁移、无 bootstrap）
  -> 启动 PostgreSQL + Redis（端口映射到宿主机，仅限本机 dev）
  -> 无 .env 时 JWT 默认 dev 占位值；生产须显式注入；健康检查走 /readyz

docker-compose.dev.yml（覆盖）
  -> AUTO_MIGRATE=true + bootstrap 管理员
  -> 挂载 configs/config.example.yaml 为 /app/configs/config.yaml
  -> 覆盖 JWT 为 dev 占位值
```

## 镜像标签

- `make docker` 产出 `aiops-api:$(VERSION)`（`VERSION` 默认 `git describe` 或 `dev`）。
- Compose 使用 `aiops-api:${AIOPS_VERSION:-dev}`，与 Makefile 对齐；构建前可设 `AIOPS_VERSION=$(git describe --tags --always --dirty)`。
- 生产 `docker-compose.prod.yml` 使用完整镜像引用 `${AIOPS_IMAGE:?...}`（不再拼接仓库名），优先 digest（`registry/repo@sha256:...`），禁止 latest；上线前运行 `scripts/verify-prod-version.ps1` 或 `scripts/deploy-prod.ps1` 校验。dev 仍用 `AIOPS_VERSION` tag。

## 入参

| 文件 | 入参 | 说明 |
| --- | --- | --- |
| `Dockerfile` | 源码、go.mod、go.sum | 构建后端二进制 |
| `docker-compose.yml` | 可选 `JWT_SECRET`、可选 `AIOPS_VERSION` | 编排服务；PG 默认 aiops/aiops；Web 登录 admin/admin123 由完整迁移种子 |
| `docker-compose.dev.yml` | 叠加于主文件 | dev 就绪：迁移 + admin + YAML |
| `.env` / 环境变量 | 数据库密码、JWT secret 等 | 覆盖部署配置 |

## 出参

| 输出 | 说明 |
| --- | --- |
| Docker 镜像 | `aiops-api:<version>` |
| 容器服务 | API（8080）、PostgreSQL（5432）、Redis（6379） |
| 暴露端口 | 主 compose 映射 PG/Redis 到宿主机，**仅限本机开发** |

## 安全提示

- PostgreSQL 默认 `aiops/aiops`、Web 登录 `admin/admin123`、dev JWT 占位值**只适用于本机联调或受控初始化**。
- 生产部署：勿将 PostgreSQL / Redis 端口发布到公网或宿主机；JWT、DB 密码、Integration 凭据加密密钥、`AIOPS_EXECUTION__AGENT_REGISTER_TOKEN` 须通过 secrets / CI 注入；关闭 bootstrap；`database.auto_migrate=false`；先执行 `/app/aiops-migrate`（或 `go run ./cmd/migrate`）再启 API。迁移 `0044` 已按 `username=admin` + 已知 admin123 哈希锁定默认管理员（status=locked、清空 password_hash），API 对外开放前运行 `scripts/provision-prod-admin.ps1` 创建安全管理员（bcrypt cost 12 + admin 角色），不得依赖默认账号（详见 `docs/release-checklist.md` §2.8 / §5.1）。
- `aiops-api:1.2` 起，非 dev 环境必须配置 `AIOPS_INTEGRATION__CREDENTIAL_ENCRYPTION_KEY`。该密钥用于加密云账号 AK/SK、Token 等接入凭据，必须是独立强随机值，不能为空、不能使用 dev 占位符、不能与 `AIOPS_AUTH__JWT_SECRET` 相同。

## 生产前端接入（CORS + VITE_API_BASE）

前后端分域或同源两种常见部署，须**同时**配置后端 CORS 与前端构建环境变量。

| 模式 | 架构 | `web/.env.production` | 后端 `cors.allow_origins` |
| --- | --- | --- | --- |
| **A. 同源反代（推荐）** | Nginx 将 `https://app.example.com/api/*` 反代到 API | `VITE_API_BASE=` 或 `/` | 可不配（浏览器同源无 CORS） |
| **B. 分域直连** | 前端 `https://app.example.com`，API `https://api.example.com` | `VITE_API_BASE=https://api.example.com` | 必须包含 `https://app.example.com` |

配置示例（分域）：

```bash
# 前端构建（web/.env.production）
VITE_API_BASE=https://api.example.com

# 后端（环境变量或 configs/config.yaml）
AIOPS_CORS__ALLOW_ORIGINS=https://app.example.com
AIOPS_CORS__ALLOW_CREDENTIALS=true
```

`docker-compose.yml` 中已预留注释项 `# AIOPS_CORS__ALLOW_ORIGINS=...`；生产勿沿用 `localhost:5173`。

本地 dev：`npm run dev` 走 Vite proxy（`/` + 代理 `/api`），CORS 仅在后端直连浏览器时生效；`configs/config.example.yaml` 已包含 `5173` 与 `127.0.0.1:5173`。

## 通俗比喻

`deployments` 就似搬新铺：Dockerfile 负责将厨房、桌椅、招牌打包装车；docker-compose 负责到新地址之后，边个放厨房、边个放收银台、边个接电。
