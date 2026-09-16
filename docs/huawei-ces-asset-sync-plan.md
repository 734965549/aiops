# 华为云 CES 资源分组同步实现方案（索引页）

> 本文档已拆分为 4 个聚焦文档，便于维护。原始完整内容（1090 行，§1–§22）保留在 git 历史中。

## 拆分后的文档

| 文档 | 用途 | 路径 |
| --- | --- | --- |
| 架构决策记录 (ADR) | 记录核心架构决策的**动因（WHY）**：CES 权威 scope、`sync_mode=ces` 默认、hybrid 增强定位、lease+fencing token、反向 stale 标记、专用全量同步端口、账号配置快照冻结等 | [adr-huawei-ces-sync.md](adr-huawei-ces-sync.md) |
| 稳定契约 | 定义系统**保证什么（WHAT）**：同步模式、权限要求、CES API 路径、同步流程、namespace 映射表、错误处理与 stale 门控、审计与日志、前端对接契约。**本次 CES 同步相关 Go 代码注释中的 `§X` 引用只允许指向主契约 `ops/huawei-ces-sync-contract.md`，不要再引用本索引页；资产仓储等非 CES 同步注释请使用完整引用指明所属文档（如 `ops/cloud-observability-contract.md §5.5`），避免误跳。** | [../ops/huawei-ces-sync-contract.md](../ops/huawei-ces-sync-contract.md) |
| 发布 Runbook | 覆盖**验收、发布策略、回滚、升级**：单元/集成验收标准、灰度发布步骤、`sync_mode=native` 回退方式、迁移 `0032`–`0042` 云同步应用 ID 与名称变更升级说明（含 `0040`/`0041`/`0042` 守卫与补建） | [huawei-ces-sync-runbook.md](huawei-ces-sync-runbook.md) |
| 已知缺口与待办 | 记录**实现进度与后续待办**：P0/P1/P2/P3 阶段进度、EVS/OBS/ELB/CCE 等增强缺口、前端待办、P3 拓扑巡检后续 | [huawei-ces-sync-backlog.md](huawei-ces-sync-backlog.md) |

## 章节映射（历史原文 §1–§22 拆分去向）

> 以下编号为拆分前原始文档（§1–§22）的章节号，仅用于追溯历史内容去向，**不作为代码注释引用依据**。代码注释中的 `§X` 一律以 [`ops/huawei-ces-sync-contract.md`](../ops/huawei-ces-sync-contract.md) 本文档内的章节编号为准。

- §1–§4、§7、§10、§13 核心概念 → ADR
- §5、§6、§8、§9、§11、§13、§14、§15（含 §7.2、§18、§21.4 编号保留）→ 稳定契约
- §16、§17、§18、§19、§22 → 发布 Runbook
- §12、§20、§21 → 已知缺口与待办

> 注：历史 §13 被拆成两部分——决策背景（动因）归入 ADR，stale 门控等稳定保证归入稳定契约文档，故上方 ADR 与稳定契约两行均含 §13。

> 稳定 API/运维契约以 `ops/cloud-observability-contract.md` 和 `ops/huawei-ces-sync-contract.md` 为准；若与本文档拆分前的历史版本冲突，以拆分后的契约文档为准。
