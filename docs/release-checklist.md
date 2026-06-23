# AIOps 上线前检查清单

发布到 staging / 生产环境前，请逐项确认。开发联调可跳过标有「生产」的条目。

---

## 1. 构建与测试

| # | 检查项 | 命令 / 操作 | 通过标准 |
|---|--------|-------------|----------|
| 1.1 | Go 单元测试 | `go test ./cmd/... ./internal/... ./pkg/...` | 全部 PASS，无 race |
| 1.2 | 前端类型检查与构建 | `cd web && npm run build` | 无 TS 错误，dist 产出正常 |
| 1.3 | 代码格式 | `make lint`（可选） | gofmt / vet / eslint 通过 |
| 1.4 | 镜像构建 | `make docker` 或 CI pipeline | `aiops-api:<version>` 构建成功 |

---

## 2. 数据库迁移

| # | 检查项 | 操作 | 通过标准 |
|---|--------|------|----------|
| 2.1 | 迁移脚本齐全 | 检查 `migrations/` 至最新版本 | 当前最高 `0022_init_execution_agent`；当前仓库未包含 `0021` Notification 迁移，按实际文件顺序执行 |
| 2.2 | **生产**关闭 auto_migrate | `database.auto_migrate=false` | API 启动不自动改表 |
| 2.3 | 发布前执行迁移 | `go run ./cmd/migrate -config <prod-config>` | 无报错，`schema_migrations` 记录最新版本 |
| 2.4 | 回滚预案 | 阅读对应 `.down.sql` | 明确回滚步骤与数据影响 |

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
| 5.1 | 管理员账号 | 运维 SQL 或预置流程 | 存在且绑定 admin 角色 |
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
| 6.2 | `e2e-asset.ps1` | 应用/资源 CRUD、匹配规则、绑定 | 输出 PASS |
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
| 8.1 | 审计写入 | 执行一次资产创建 + 告警处理 | `GET /api/audits` 可查到 actor/action/result |
| 8.2 | 审计不影响主流程 | 审计库短暂不可用（测试环境） | 主业务按设计降级或报错，行为符合预期 |
| 8.3 | 日志级别 | `log.level` | 生产建议 `info`，调试期避免 `debug` 泄露敏感信息 |
| 8.4 | AI Provider | `ai.providers` 配置 | 密钥来自 secrets，非明文提交仓库 |

---

## 9. 发布执行顺序

```text
1. 备份数据库
2. 执行 make migrate（或 go run ./cmd/migrate）
3. 部署 API 镜像（滚动更新，先新版本后切流量）
4. 验证 /readyz
5. 部署前端静态资源
6. 执行 E2E 脚本或手工抽检（demo-flow.md）
7. 观察日志与告警 15–30 分钟
```

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

当启用 Integration、Observability、Inspection、Notification 任一能力时，发布前必须额外确认：

| # | 检查项 | 配置 / 操作 | 通过标准 |
|---|--------|-------------|----------|
| 11.1 | 云账号最小权限 | 华为云 IAM / 其他云厂商只读策略 | 仅包含资源、指标、日志、链路、告警只读权限 |
| 11.2 | 凭据注入 | Secret 管理或加密存储 | AK/SK、Token 不进入配置文件、日志、审计、前端响应 |
| 11.3 | 工具权限 | AI 工具权限种子和角色绑定 | 只有授权角色可调用 `cloud.*`、`inspection.*`、`notification.*` 工具 |
| 11.4 | Provider 限流 | Adapter 配置 | 按账号、region、工具设置限流和超时 |
| 11.5 | 巡检 Worker | 调度开关和并发数 | staging 验证无重复运行、无任务堆积 |
| 11.6 | 审计 | 触发一次指标查询和巡检 | 审计可查调用者、资源范围、参数摘要、结果摘要 |
| 11.7 | 通知通道 | 企业微信/飞书/邮件/Webhook | 测试发送成功，失败有重试和记录 |
| 11.8 | 执行边界 | Recommendation 转 Execution | 中高风险建议进入 `pending_confirm`，不会自动执行 |
| 11.9 | 执行介体权限 | 介体注册和代理注册 | 介体绑定环境/网络域/能力，禁用介体不能执行 |
| 11.10 | Command Spec | 命令模板与参数 schema | 禁止自由 shell 字符串直接执行，参数不合法时拒绝 |
| 11.11 | 代理通信 | agent mTLS/token、心跳、租约 | 代理只能主动拉取任务，未确认任务不能领取 |
| 11.12 | 执行日志 | stdout/stderr 回传 | 服务端二次脱敏，敏感字段不进审计明文 |

相关文档：

- `docs/AI运维平台整体流程与调用关系.md`
- `docs/cloud-observability-agent-roadmap.md`
- `ops/cloud-observability-contract.md`
- `ops/execution-agent-contract.md`

## 12. 签字表

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
