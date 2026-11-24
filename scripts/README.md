# scripts 目录说明

`scripts` 存放 E2E 验收、生产运维与工程检查脚本。E2E 均为 **API/PowerShell 验收脚本**（Invoke-WebRequest/curl 级别），不是浏览器 UI 验收；`PASS` 只表示脚本内断言通过，单项覆盖边界以 `docs/acceptance-checklist.md` 各节说明为准（单一事实源，本文不重复罗列）。

## 通用前置

```powershell
# 标准环境（与各脚本头部注释一致）
docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
go run ./cmd/migrate
go run ./cmd/api            # 或 compose 内 api 服务
.\scripts\e2e-alert.ps1     # 默认 API_BASE=http://127.0.0.1:8080，admin/admin123 登录
```

- 所有脚本支持 `-ApiBase`（或环境变量 `API_BASE`）指向远端/staging。
- 默认登录凭据 `admin/admin123` 仅限 dev（迁移 0044 后生产锁定）。
- 多数脚本**保留业务数据**（告警/会话/任务/审计），仅清理探针资源（fake 账号、mock provider、临时文件）；个别脚本有 SKIP 分支（见下表），SKIP 后 `exit 0`，不代表覆盖完成。

## E2E 验收脚本

| 脚本 | 迁移前置 | 外部依赖 | 写数据与清理 | 特殊语义 | 验收对照 |
| --- | --- | --- | --- | --- | --- |
| `e2e-alert.ps1` | 0001–0009 | — | 告警源/告警/时间线保留 | 主链路第一步，产出 alert_id 供 execution/runbook 复用（`-AlertId`） | 验收 §1 |
| `e2e-alert-sync.ps1` | 0046/0047 | fake CES 账号（`auth_type=none`，仅非 prod env） | finally 清理 integration account；告警与流水保留 | 三轮拉取、独立恢复、审计断言 | 验收 §1.4 |
| `e2e-asset.ps1` | 0008/0014 | — | 应用/资源/规则保留 | 注册表 CRUD、筛选排序、告警绑定 | 验收 §2.1/2.2/2.4 |
| `e2e-asset-sync.ps1` | 0023–0033 | fake 华为账号 | 清理同步资源/应用/账号 | running/partial 终态、批次列表、stale | 验收 §2.3 |
| `e2e-asset-sync-real.ps1` | 同上 | **真实华为 AK/SK**（不进 CI） | 人工比对 CES 总数 | 可选对账脚本，打印 region/type 摘要 | 验收 §2.3.12 |
| `e2e-runbook.ps1` | 0011/0012 | 0012 种子模板；可复用 processing 告警 | 任务/时间线保留 | `confirm_text=CONFIRM`、dry-run 标记、三类时间线 | 验收 §3 |
| `e2e-execution.ps1` | 0011 | 依赖 e2e-alert 产出的告警 | 任务保留 | 确认/拒绝/模拟执行 | 验收 §4 |
| `e2e-execution-agent.ps1` | 0022 | — | agent/租约数据保留 | 注册/心跳/租约/日志流 | 验收 §4 Agent 部分 |
| `e2e-execution-agent-permission.ps1` | 0022 | — | — | Agent 注册/操作权限负向检查 | 同上 |
| `e2e-identity-access.ps1` | 0015 | — | 角色/绑定保留 | RBAC/数据范围/权限管理 | 验收 Identity 部分 |
| `e2e-inspection.ps1` | 0020 | fake 观测账号 | **清理 policy/应用/账号** | 巡检策略/运行/Finding/建议 | 验收 Inspection 部分 |
| `e2e-integration.ps1` | 0018/0024 | — | **删除测试账号** | 账号 CRUD/能力/字段检查 | 验收 Integration 部分 |
| `e2e-observability.ps1` | 0019 | fake provider 账号 | **清理账号** | 四类查询 fake 主路径 | 验收 Observability 部分 |
| `e2e-ai-assistant.ps1` | 0049/0050/0053 | mock provider（共享 fixture） | 业务会话/任务/审计保留，仅删 probe 会话 | 分享步骤覆盖；degraded 仍可结构通过 | 验收 §9.8/9.13 |
| `e2e-ai-stream.ps1` | 0049 | mock；`-SkipSlow` 跳慢流 | 临时文件/mock 清理，业务数据保留 | SSE/429/部分输出/取消 | 验收 §9.12 |
| `e2e-ai-agent-loop.ps1` | 0049/0055 | mock + active 告警 | 会话保留 | 无 active 告警 **SKIP 后 exit 0** | 验收 §9.11 |
| `e2e-ai-rules.ps1` | 0055/0062 | mock + fake 账号 | finally 清理 fake 账号 | 规则列表/自由指标/批量体检 | 验收 §9.14 |
| `e2e-ai-evidence.ps1` | 0060 | mock + fake 账号 | 清理账号 | 快照 SHA-256 与 obs_evidence_ref 一致 | 验收 §9.2 |
| `e2e-ai-memory.ps1` | 0051 | **真实 type b provider** | 会话/案例保留（案例最终 retired） | 首条回答 degraded **SKIP 后 exit 0** | 验收 §9.9 |
| `e2e-ai-knowledge.ps1` | 0052/0054 | mock 即可 | finally 清残留条目、retire 案例、删 mock | 覆盖 draft/激活/聚合/retire | 验收 §9.10 |
| `e2e-postgres-history.ps1` | 0061 | **真实 ak_sk 账号 + 长事务告警**（`-AlertId` 必填） | 只清 mock；会话与证据保留 | 权限不足显式失败；RDS IAM 三授权项 | 验收 §9.17 |

共享 fixture：`e2e-ai-mock-provider.ps1` 提供 `Invoke-AiApi`/`Start-AiMockProvider`/`Register-AiMockProvider`，mock 模型用 `scripts/tools/ai-mock-provider`（Go 编译，`MOCK_TOOL_*`/`MOCK_STREAM_*` 环境分支决定行为）。

## 生产运维脚本

| 脚本 | 用途 | 关键边界 |
| --- | --- | --- |
| `deploy-prod.ps1` | 生产发布总链路：备份 → migrate → verify → 建管理员 → up API → /readyz | `-DbMode container/host`、`-SkipUp/-SkipVerify/-SkipProvisionAdmin`；container 模式不支持 provision（bcrypt 需宿主 Go） |
| `backup-pre-migration.ps1` | 迁移前数据库备份（pg_dump） | host 模式需宿主 psql；连接参数经 `-PgHost/-PgPort/-PgPassword` 传入，不硬编码 |
| `verify-post-migration.ps1` | 迁移后结构/种子校验 | 与 backup 成对使用 |
| `verify-prod-version.ps1` | 校验生产镜像引用（digest/不可变 tag，禁 latest） | 配套 `tests/verify-prod-version.tests.ps1` |
| `provision-prod-admin.ps1` | 创建安全管理员（bcrypt cost 12） | `-Force` 才重置；生产 bootstrap 关闭后的唯一入口 |
| `tools/hash-password` | Go bcrypt 工具（provision 依赖） | 供 `go run ./scripts/tools/hash-password` |

## 工程检查

- `check_architecture.py`：文件规模基线检查（`architecture_baseline.json`）；`--tighten` 在缩短存量后收紧基线，CI 比较变更前基线禁止增行。
- `tests/test_architecture_guard.py`：基线文件自身的结构校验。
- `tests/verify-prod-version.tests.ps1`：镜像版本校验脚本的 Pester 测试。

## 边界

- E2E 脚本对同一环境**可重复执行**（种子/资源用 `ON CONFLICT` 或先查后建），但重复执行会累积业务数据；对外演示建议用 `deployments/README.demo.md` 的隔离环境。
- 新增 E2E 时同步：本表、`docs/acceptance-checklist.md` 对应节的前置说明、（若涉及契约）对应 `ops/*.md`。
- 脚本不承担 UI 验收；浏览器侧步骤见验收清单附录 A 与各模块验收节。
