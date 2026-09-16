# ops 契约说明

`ops` 目录保存平台对外稳定契约。后端实现、前端联调、运维接入和测试验收都应优先对齐这里的接口、参数、响应、状态机、权限和迁移约束。

## 当前契约

- `auth-contract.md`：登录、JWT、refresh token、登出和认证审计契约。
- `identity-api-contract.md`：角色、权限、当前用户、统一授权校验、管理员用户预置与 LDAP 导入契约。
- `alert-contract.md`：告警源管理、Webhook ingest、状态流转、AI 分析入口和时间线契约。
- `runbook-contract.md`：处置预案模板、告警推荐、多步骤执行任务生成契约。
- `execution-contract.md`：告警处置执行任务创建、确认、执行与时间线回写契约。
- `ai-contract.md`：AI provider 管理、工具调用与前端交互契约。
- `health-contract.md`：`/healthz`、`/readyz`、`/version` 契约。
- `migration-contract.md`：数据库迁移 runner、版本文件和回滚责任边界。
- `cloud-observability-contract.md`：Integration、Observability、Inspection、建议转执行的接口契约。
- `execution-agent-contract.md`：Execution Agent、执行介体、Command Spec、租约和日志回传契约。

全项目串联关系见 `docs/AI运维平台整体流程与调用关系.md`。

## 契约使用方式

```text
开发 / 测试 / 运维
  -> 阅读 ops 契约
  -> 前端按契约传参
  -> 后端按契约实现接口
  -> 测试按契约验证结果
  -> 文档和验收脚本随接口变化同步更新
```

## 输入输出关注点

| 契约 | 关注输入 | 关注输出 |
| --- | --- | --- |
| auth | username、password、refresh_token、Bearer token | token、认证错误、审计结果 |
| identity-api | Bearer token、分页查询参数、授权校验请求体 | 角色/权限列表、当前用户、授权判断结果 |
| alert | Webhook payload、告警业务 ID、状态流转请求体 | 告警详情、事件时间线、接入结果 |
| runbook | alert_id、模板查询参数、模板更新请求 | 推荐预案、模板详情、多步骤执行计划 |
| execution | task_id、确认文本、任务参数 | 执行任务、步骤状态、时间线回写结果 |
| ai | provider 配置、工具调用请求体 | provider 列表、工具调用结果、`allowed` 判断 |
| health | 简单 HTTP 请求 | 服务状态、版本、就绪情况 |
| migration | migration 文件、数据库连接、执行环境 | 执行成功、失败原因、版本记录 |
| cloud-observability | `account_id`、provider、观测查询条件、巡检策略 | 账号摘要、能力声明、证据引用、Finding、Recommendation |
| execution-agent | 已确认任务、medium/agent 业务 ID、租约、Command Spec 参数 | agent 心跳、租约、日志片段、执行结果 |

## 契约到代码的对应关系

```text
ops/cloud-observability-contract.md
  -> internal/integration        账号、凭据引用、连通性、能力声明
  -> internal/observability      查询 API、Provider Port、EvidenceRef
  -> internal/inspection         策略、运行、Finding、Recommendation
  -> web/src/api/README.md       前端调用路径和错误处理
  -> migrations/0018-0020        表结构和权限种子

ops/execution-agent-contract.md
  -> internal/execution          medium、agent、command spec、lease、log stream
  -> scripts/e2e-execution-agent*.ps1
  -> migrations/0022
```

真实执行相关契约继续落在 `ops/execution-contract.md` 与 `ops/execution-agent-contract.md`。AI、Inspection 或 Recommendation 只允许创建待确认 Execution Task，不能跳过 Execution 状态机。
