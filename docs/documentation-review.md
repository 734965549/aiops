# 文档与注释体检记录

体检日期：2026-06-25

## 检查范围

- 根目录入口文档：`README.md`、`AGENTS.md`。
- 产品与架构文档：`docs/*.md`。
- 运维契约：`ops/*.md`。
- 前端 API 说明：`web/src/api/README.md`。
- 目录级说明：`internal/README.md`、`ops/README.md`。
- Go 代码注释中的明显表达问题。

## 自动检查结果

| 检查项 | 结果 |
| --- | --- |
| Markdown 文件数量 | 57 |
| Mermaid 文件数量 | 7 |
| Mermaid 代码块数量 | 52 |
| Markdown 代码围栏闭合 | 未发现未闭合围栏 |
| Markdown 相对链接 | 未发现缺失目标文件 |

## 主要问题

1. `web/src/api/README.md` 缺少 P0 主链路 API 说明，尤其是 Alert、Runbook、Execution、Dashboard。已有说明偏向新观测链路，容易让联调人员忽略当前验收主线。
2. `docs/AI运维平台整体流程与调用关系.md` 原本使用粤语表达，并作为 README、ops、roadmap 的关键入口，风格不适合作为全团队长期维护的权威调用图。
3. README 的契约索引没有在“运维契约索引”处显式列出 `cloud-observability-contract.md` 和 `execution-agent-contract.md`，而后文又引用它们，入口不够一致。
4. `internal/README.md`、`ops/README.md` 使用口语化表达，适合早期解释，但不适合作为工程协作入口。
5. 代码注释中存在少量粤语词汇，主要集中在 auth/config/identity/observability/integration/alert 的说明性注释里，降低跨团队可读性。
6. `docs/AI运维平台技术架构设计.md`、`docs/AI运维平台信息架构.md` 中仍有较多规划态模块（工单、知识库、审批、通知、消息队列、向量库等）。这些内容可以保留，但应持续标注“规划/未落地/已落地”，避免与当前代码能力混淆。

## 本轮已修复

- 将 `docs/AI运维平台整体流程与调用关系.md` 改为普通话版，并补充启动装配图、主调用链、Provider Port 对应关系和维护检查清单。
- 新增 `docs/AI运维平台调用关系图.md`，集中展示后端总图、P0 时序、AI 工具、观测巡检、Execution Agent 与模块装配关系。
- 补齐 `web/src/api/README.md` 中 Alert、Asset、Runbook、Execution、Dashboard 等主链路 API 说明，并保留 Integration、Observability、Inspection 的调用链。
- 更新 `README.md` 的调用关系入口文案和契约索引，补入 cloud observability 与 execution agent 契约。
- 重写 `internal/README.md` 与 `ops/README.md`，统一为工程协作风格。
- 修正 `docs/AI运维平台信息架构.md`、`docs/cloud-observability-agent-roadmap.md`、`ops/cloud-observability-contract.md`、`docs/acceptance-checklist.md` 中对调用关系图的旧表述。
- 收敛第一批 Go 注释中的明显口语化表达，不改变代码逻辑。

## 后续建议

1. 给缺少目录 README 的核心上下文补最小说明：`internal/alert`、`internal/asset`、`internal/runbook`、`internal/execution`、`internal/audit`、`internal/ai`、`internal/dashboard`。
2. 将 `docs/AI运维平台技术架构设计.md` 拆成“当前落地架构”和“规划架构”两部分，减少 P0/P1/P2 混读。
3. 继续收敛各契约中的口语化表达，优先处理 `ops/alert-contract.md`、`docs/future-direction/模块关系与调用说明-粤语版.md` 的引用关系。
4. 如果需要更严格的图校验，可在前端工具链中补 Mermaid 渲染检查；当前仅做 Markdown 围栏和链接检查。
