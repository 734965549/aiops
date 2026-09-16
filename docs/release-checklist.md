# AIOps 上线前检查清单

发布到 staging / 生产环境前，请逐项确认。开发联调可跳过标有「生产」的条目。

---

## 1. 构建与测试

| # | 检查项 | 命令 / 操作 | 通过标准 |
|---|--------|-------------|----------|
| 1.1 | Go 单元测试 | `go test ./cmd/... ./internal/... ./pkg/...` | 全部 PASS，无 race |
| 1.2 | 前端类型检查与构建 | `cd web && npm run build` | 无 TS 错误，dist 产出正常 |
| 1.2a | 前端生产镜像 | `docker build -f web/Dockerfile -t aiops-web:<version> web` | `npm ci` 成功（以 `package-lock.json` 为准，勿依赖宿主机脏 `node_modules`） |
| 1.2b | 前端依赖审计 | `cd web && npm audit --omit=dev`；全量 `npm audit` | **生产依赖 0 漏洞**。全量报告中 Vite/esbuild 的 moderate/high 属 **devServer 构建工具链**（GHSA-67mh-4wv8-2f99），不影响运行时产物；升级到 Vite 7/8 属独立变更，勿用 `npm audit fix --force` 强推 |
| 1.3 | 代码格式 | `make lint`（可选） | gofmt / vet / eslint 通过 |
| 1.4 | 镜像构建 | `make docker-prod` 或 CI pipeline | `aiops-api:<version>` 构建成功；**拒绝** `*-dirty` / `latest` / 空版本。发布前须 clean commit + tag（如 `v1.x.y`） |

---

## 2. 数据库迁移

| # | 检查项 | 操作 | 通过标准 |
|---|--------|------|----------|
| 2.1 | 迁移脚本齐全 | 检查 `migrations/` 至最新版本 | 当前最高 `0045_inspection_policy_deleted_scope_cleanup`；当前仓库未包含 `0021` Notification 迁移，按实际文件顺序执行 |
| 2.2 | **生产**关闭 auto_migrate | `database.auto_migrate=false` | API 启动不自动改表 |
| 2.3 | 发布前执行迁移 | `go run ./cmd/migrate -config <prod-config>` | 无报错，`schema_migrations` 记录最新版本 |
| 2.4 | 回滚预案 | 阅读对应 `.down.sql` | 明确回滚步骤与数据影响 |
| 2.5 | **0032 破坏性迁移备份** | 升级前备份 `asset_application`、`asset_resource`、`asset_match_rule` 表 | 0032 是 DELETE 脚本，会删除旧格式 `cloud-<account_id>` 应用及其关联资源/匹配规则（不可自动恢复）；迁移保留 `integration_account`，升级后需重新触发云同步（无需重新录入账号）。0039 会清理 `alert_alert`/`inspection_policy` 中的孤儿引用，但不恢复已删除数据 |
| 2.6 | **迁移后引用完整性验证** | `SELECT * FROM v_asset_app_ref_integrity;` | 返回 0 行（0040 创建的视图，检查 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy` 中的 `application_id` 引用在 `asset_application` 中存在）。0043 会自动修复已有孤儿引用并通过 CHECK n=0 硬验收 |
| 2.7 | **重新触发云同步**（硬验收步骤） | 对已有接入账号执行 `POST /api/assets/sync` | 同步成功，资源写入新格式 `cloud-<前缀>-<hash>` 应用；同步后再次执行 2.6 验证视图返回 0 行 |
| 2.8 | **管理员安全态** | 确认迁移 `0044` 已应用；`verify-post-migration.ps1` 检查 (f) | `0044` 把 0016/0017 种入的 `admin/admin123` 锁定（status=locked、清空 password_hash）。检查 (f) 接受两种合法态：**首发** locked 空哈希 admin，或**二次发布/已 provision** 的 active + admin 角色 + 非空密码哈希。API 对外开放前，运行 `.\scripts\provision-prod-admin.ps1 -PgPassword '<db-password>'`（或 `-GeneratePassword`）激活/创建管理员；已 provision 时可重复执行（幂等跳过，`-Force` 才重置密码）；**staging 须演练一遍** |

迁移契约：`ops/migration-contract.md`

---

## 3. 配置与安全（生产必查）

| # | 检查项 | 配置项 | 通过标准 |
|---|--------|--------|----------|
| 3.1 | JWT 密钥 | `auth.jwt_secret` | 非占位值，足够熵，通过 `Config.Validate()` |
| 3.2 | 关闭 bootstrap | `auth.bootstrap_username/password` 留空 | 不自动创建默认管理员 |
| 3.3 | Redis 必填 | `redis.required=true` | `/readyz` 中 redis 为 ok |
| 3.4 | 数据库凭据 | 环境变量 / secrets 注入 | 非默认 `aiops/aiops` |
| 3.5 | CORS | `cors.allow_origins` | 仅包含正式前端域名，禁止 `*` + credentials |
| 3.6 | 前端 API 地址 | `web/.env.production` → `VITE_API_BASE` | 与部署架构一致（同源反代或分域） |
| 3.7 | 网络暴露 | Compose / K8s Service | **不**将 PostgreSQL、Redis 端口发布到公网 |
| 3.8 | Webhook 密钥 | 各告警源 `webhook_secret` | 生产使用强随机值，定期轮换 |
| 3.9 | **镜像引用不可变** | `AIOPS_IMAGE`（完整引用，如 `registry/repo@sha256:...`） | 运行 `.\scripts\verify-prod-version.ps1`（或 `.\scripts\deploy-prod.ps1 -SkipUp`）：digest PASS / 不可变 tag WARN / latest 与空 FAIL。禁止 latest，优先 digest；compose 不再拼接仓库名，勿传纯 tag 或纯 digest。`deploy-prod.ps1` 已扩展为完整发布链路（§9），`-SkipUp` 仅做镜像校验，CI dry-run 用 |
| 3.10 | **执行代理注册令牌** | `AIOPS_EXECUTION__AGENT_REGISTER_TOKEN` | 生产必填；`openssl rand -base64 32` 生成（勿用 hex-only）；通过 `Config.Validate()`；compose 未设置时直接报错 |
| 3.11 | **Token TTL** | `AIOPS_AUTH__ACCESS_TTL_M` / `AIOPS_AUTH__REFRESH_TTL_H` | 生产 compose 默认 60min / 72h；可按 **30–60min / 24–72h** 调整，避免 dev 默认 120min / 7d |
| 3.12 | **API 容器加固** | `docker-compose.prod.yml` api 服务 | 非 root + `read_only` + `cap_drop: ALL` + `no-new-privileges` |

配置说明：`deployments/README.md`、`configs/config.example.yaml`

---

## 4. 服务健康

| # | 检查项 | 端点 | 通过标准 |
|---|--------|------|----------|
| 4.1 | 存活探针 | `GET /healthz` | HTTP 200 |
| 4.2 | 就绪探针 | `GET /readyz` | `data.status=ready`，`migration`/`db`/`redis` 为 ok |
| 4.3 | 版本信息 | `GET /version` | 版本号、commit、构建时间与发布一致 |
| 4.4 | 日志 trace | 任意 API 响应头 | 含 `X-Trace-Id`，日志可关联 |

健康契约：`ops/health-contract.md`

---

## 5. 权限与身份

| # | 检查项 | 操作 | 通过标准 |
|---|--------|------|----------|
| 5.1 | 管理员账号 | `.\scripts\provision-prod-admin.ps1`（0044 后、API 对外前） | 目标用户 `status=active`、含 bcrypt cost 12 密码哈希，且 `iam_user_role` 已绑定 `admin` 角色；可用 `POST /api/identity/login` 验证 |
| 5.2 | 种子权限 | migration `0002` + 后续权限迁移 | admin 具备各模块读写权限 |
| 5.3 | 负向测试 | 无 token / 无权限用户访问 API | 返回 401 / 403 |
| 5.4 | 域账号策略 | 外部登录 | 仅预置绑定用户可登录（不自动开户） |

权限矩阵见：`docs/acceptance-checklist.md` §6

---

## 6. 业务链路 E2E（staging 推荐）

在目标环境（或连 staging API）执行：

```powershell
$env:API_BASE = "https://staging-api.example.com"   # 如需要
.\scripts\e2e-alert.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1
.\scripts\e2e-identity-access.ps1
```

| # | 脚本 | 覆盖范围 | 通过标准 |
|---|------|----------|----------|
| 6.1 | `e2e-alert.ps1` | 告警源、ingest、状态流转 | 输出 PASS |
| 6.2 | `e2e-asset.ps1` | 应用/资源 CRUD、匹配规则、绑定、云同步 | 输出 PASS |
| 6.3 | `e2e-runbook.ps1` | 推荐、多步执行、时间线回写 | 输出 PASS |
| 6.4 | `e2e-execution.ps1` | 简单任务确认执行 | 输出 PASS |
| 6.5 | `e2e-identity-access.ps1` | viewer 角色、授权写接口、403 边界、审计 | 输出 PASS |

---

## 7. 前端部署

| # | 检查项 | 操作 | 通过标准 |
|---|--------|------|----------|
| 7.1 | 生产构建 | `npm run build` | 主业务 chunk < 100KB（vendor 独立分包） |
| 7.2 | 静态资源托管 | Nginx / CDN | `index.html` + `assets/` 可访问 |
| 7.3 | API 反代（同源模式） | Nginx `location /api` | 浏览器无 CORS 错误 |
| 7.4 | 路由 history 模式 | `try_files` 回退 | 刷新子路由不 404 |
| 7.5 | 登录与会话 | 浏览器登录 | token 刷新、退出正常 |

---

## 8. 审计与可观测性

| # | 检查项 | 操作 | 通过标准 |
|---|--------|------|----------|
| 8.1 | 审计写入 | 执行一次资产创建 + 告警处理 + 云同步（含 hybrid 增强） | `GET /api/audits` 可查到 actor/action/result；`hybrid` 任一增强失败时批次应为 `partial`，且 summary 暴露 `enrichment_failed_count` / `enrichment_failed_types` |
| 8.2 | 审计不影响主流程 | 审计库短暂不可用（测试环境） | 主业务按设计降级或报错，行为符合预期 |
| 8.3 | 日志级别 | `log.level` | 生产建议 `info`，调试期避免 `debug` 泄露敏感信息 |
| 8.4 | AI Provider | `ai.providers` 配置 | 密钥来自 secrets，非明文提交仓库 |

---

## 9. 发布执行顺序

```text
1. 备份数据库
2. 执行 make migrate（或 go run ./cmd/migrate）
3. 运行 verify-post-migration.ps1
4. 运行 provision-prod-admin.ps1（0044 后创建首个安全管理员）
5. 部署 API 镜像（滚动更新，先新版本后切流量）
6. 验证 /readyz
7. 部署前端静态资源
8. 执行 E2E 脚本或手工抽检（demo-flow.md）
9. 观察日志与告警 15–30 分钟
```

### 一键发布脚本（封装步骤 1–6）

`scripts/deploy-prod.ps1` 把上面的步骤 1–6 串成一条命令，每步独立失败中止：

```powershell
# 默认：host 模式连 PG + container 模式跑 migrate + 交互输入管理员密码
.\scripts\deploy-prod.ps1 -PgPassword '<db-password>'

# PG 端口不映射到宿主机时，备份 / 验证走 docker compose exec；
# provision-admin 不支持 container 模式，需单独 host 模式执行
.\scripts\deploy-prod.ps1 -DbMode container -SkipProvisionAdmin
.\scripts\provision-prod-admin.ps1 -PgPassword '<db-password>' -GeneratePassword

# staging 演练：生成随机密码并完成全链路
.\scripts\deploy-prod.ps1 -PgPassword '<db-password>' -GenerateAdminPassword -ExpectedMigrationVersion 0045

# CI dry-run：仅解析 env + 校验 AIOPS_IMAGE
.\scripts\deploy-prod.ps1 -SkipUp
```

关键开关：`-DbMode host|container`、`-MigrateMode container|local`、`-SkipBackup`/`-SkipMigrate`/`-SkipVerify`/`-SkipProvisionAdmin`。脚本不改 schema / 业务代码；若需手工分步执行，每个子脚本仍可独立调用。剩余步骤 7（前端静态资源）、8（E2E）、9（观察）仍需人工处理。

---

## 10. 回滚预案

| 场景 | 操作 |
|------|------|
| API 新版本异常 | 回滚至上一镜像 tag，`/readyz` 恢复后切流量 |
| 迁移失败 | **不要**启动新版本 API；修复 SQL 或执行 down 迁移后重试 |
| 前端异常 | 回滚静态资源至上一版本目录 |
| 数据问题 | 从备份恢复，记录 incident 与审计日志 |

---

## 11. 云厂商只读接管发布前检查（P1+）

> **能力边界**：Notification 模块未交付（无 `0021` 迁移）；华为 `auth_type=agency` 委托未实现；OBS 等 hybrid 资源详情增强仍在 backlog。本节 11.8 等 Notification 相关项**当前跳过**，详见 [§12 版本能力边界](#12-版本能力边界发布说明)。

当启用 Integration、Observability、Inspection 任一已落地能力时，发布前必须额外确认：

| # | 检查项 | 配置 / 操作 | 通过标准 |
|---|--------|-------------|----------|
| 11.1 | 云账号最小权限 | 华为云 IAM / 其他云厂商只读策略 | 仅包含资源、指标、日志、链路、告警只读权限 |
| 11.2 | Integration 凭据加密密钥 | `AIOPS_INTEGRATION__CREDENTIAL_ENCRYPTION_KEY` | 已通过 Secret 注入独立强随机值，不为空、不使用 dev 占位符、不等于 JWT secret |
| 11.3 | 凭据注入 | Secret 管理或加密存储 | AK/SK、Token 不进入配置文件、日志、审计、前端响应 |
| 11.4 | 工具权限 | AI 工具权限种子和角色绑定 | 只有授权角色可调用 `cloud.*`、`inspection.*`、`notification.*` 工具 |
| 11.5 | Provider 限流 | Adapter 配置 | 按账号、region、工具设置限流和超时 |
| 11.6 | 巡检 Worker | 调度开关和并发数 | staging 验证无重复运行、无任务堆积 |
| 11.7 | 审计 | 触发一次指标查询和巡检 | 审计可查调用者、资源范围、参数摘要、结果摘要 |
| 11.8 | 通知通道 | 企业微信/飞书/邮件/Webhook | **未交付**（无 Notification 模块）；跳过 |
| 11.9 | 执行边界 | Recommendation 转 Execution | 中高风险建议进入 `pending_confirm`，不会自动执行 |
| 11.10 | 执行介体权限 | 介体注册和代理注册 | 介体绑定环境/网络域/能力，禁用介体不能执行 |
| 11.11 | Command Spec | 命令模板与参数 schema | 禁止自由 shell 字符串直接执行，参数不合法时拒绝 |
| 11.12 | 代理通信 | agent mTLS/token、心跳、租约 | 代理只能主动拉取任务，未确认任务不能领取 |
| 11.13 | 执行日志 | stdout/stderr 回传 | 服务端二次脱敏，敏感字段不进审计明文 |

相关文档：

- `docs/AI运维平台整体流程与调用关系.md`
- `docs/cloud-observability-agent-roadmap.md`
- `ops/cloud-observability-contract.md`
- `ops/execution-agent-contract.md`

## 12. 版本能力边界（发布说明）

以下能力**当前版本未完整交付**，发布说明与验收须明确边界，避免用户预期偏差：

| 能力 | 状态 | 说明 |
|------|------|------|
| **Notification 通知** | 未交付 | 仓库无 `0021_init_notification` 迁移与 `internal/notification` 模块；巡检策略中的 `notification_policy_id` 字段预留，无实际发送通道 |
| **华为 agency 委托** | 未实现 | `auth_type=agency` 的 CES 指标查询、资产同步、全量同步返回 `unsupported` / `FailedPrecondition`；仅 AK/SK 直连可用 |
| **OBS / EVS 等 hybrid 增强** | backlog | CES 基础资源入库已落地；ECS/RDS/VPC/DCS/DMS 详情补充已有，OBS、EVS、ELB、CCE 等详情增强待办（见 `docs/huawei-ces-sync-backlog.md`） |

## 13. 签字表

| 类别 | 检查人 | 日期 | 结论 |
|------|--------|------|------|
| 构建与测试（§1） | | | ☐ 通过 |
| 数据库迁移（§2） | | | ☐ 通过 |
| 配置与安全（§3） | | | ☐ 通过 |
| 服务健康（§4） | | | ☐ 通过 |
| 权限（§5） | | | ☐ 通过 |
| E2E 链路（§6） | | | ☐ 通过 |
| 前端部署（§7） | | | ☐ 通过 |
| 审计（§8） | | | ☐ 通过 |

**发布负责人** ______　**环境** ______（staging / prod）　**版本** ______

---

## 相关文档

- [演示流程](./demo-flow.md) — 逐步演示与话术
- [验收清单](./acceptance-checklist.md) — 模块级验收明细
- [Kubernetes 部署说明](../deployments/kubernetes.md) — 外挂 PostgreSQL/Redis 的 K8s 部署参考
- [README](../README.md) — 项目总览与快速启动
- `ops/` — 各模块 API 契约
