# ops 模块说明（粤语版）

## 呢个目录做咩

`ops` 系运维同接口契约嘅「办事规矩」。后端、前端、运维、测试要对齐接口点叫、参数点传、返回点睇，就可以参考呢度。

## 当前内容

- `auth-contract.md`：登录、JWT、刷新 token 契约。
- `identity-api-contract.md`：角色、权限、当前用户、统一授权校验、**管理员用户预置与 LDAP 导入**契约。
- `ai-contract.md`：AI provider 管理、工具调用与前端交互契约。
- `health-contract.md`：健康检查、就绪检查、版本接口契约。
- `migration-contract.md`：数据库迁移运行同约束说明。
- `execution-contract.md`：告警处置执行任务创建、确认、执行与时间线回写契约。
- `runbook-contract.md`：处置预案模板、告警推荐、多步骤执行任务生成契约。
- `cloud-observability-contract.md`：Integration、Observability、Inspection、建议转执行嘅接口契约。
- `execution-agent-contract.md`：Execution Agent、执行介体、Command Spec、租约同日志回传契约。

全项目串联关系见 `docs/AI运维平台整体流程与调用关系.md`；呢份图用粤语说明前端、HTTP、application、domain、infrastructure、Provider Adapter、Execution Agent 点样一层层调用。

## 调用关系

```text
开发 / 测试 / 运维
  -> 阅读 ops 契约
  -> 前端按契约传参
  -> 后端按契约实现接口
  -> 测试按契约验证结果
```

## 入参

| 契约 | 关注入参 |
| --- | --- |
| auth | username、password、refresh_token、Bearer token |
| identity-api | Bearer token、分页查询参数、授权校验请求体 |
| ai | Bearer token、provider 配置、工具调用请求体 |
| health | 无或简单 HTTP 请求 |
| migration | migration 文件、数据库连接、执行环境 |
| execution / runbook | Bearer token、告警 ID、执行任务参数、Runbook 模板与步骤 |
| cloud-observability | Bearer token、`account_id`、provider、观测查询条件、巡检策略 |
| execution-agent | 已确认任务、medium/agent 业务 ID、租约、Command Spec 参数 |

## 出参

| 契约 | 关注出参 |
| --- | --- |
| auth | token、认证错误 |
| identity-api | 角色/权限列表、当前用户、授权判断结果 |
| ai | provider 列表、工具调用结果 |
| health | 服务状态、版本、就绪情况 |
| migration | 执行成功、失败原因、版本记录 |
| execution / runbook | 执行任务、步骤状态、推荐预案、时间线回写结果 |
| cloud-observability | 账号摘要、能力声明、证据引用、Finding、Recommendation |
| execution-agent | agent 心跳、租约、日志片段、执行结果 |

## 云厂商只读接管契约

- `cloud-observability-contract.md`：P1+ 云账号只读接管、统一观测查询、巡检策略、AI 建议和通知发送契约草案。
- `execution-agent-contract.md`：P1+ 执行介体、执行代理、受控命令、租约、日志回传和确认后执行契约草案。

该契约用于指导后续实现，当前已落地 P0 闭环仍以 `alert-contract.md`、`ai-contract.md`、`execution-contract.md` 等现有契约为准。实现过程中若新增接口、状态机、权限码、迁移或审计字段，必须先更新该契约再改代码。

## 契约到代码嘅对应关系

```text
ops/cloud-observability-contract.md
  -> internal/integration        账号、凭据引用、连通性、能力声明
  -> internal/observability      查询 API、Provider Port、EvidenceRef
  -> internal/inspection         策略、运行、Finding、Recommendation
  -> web/src/api/README.md       前端调用路径同错误处理
  -> migrations/0018-0020        表结构同权限种子
```

真实执行相关契约继续落喺 `ops/execution-contract.md` 同 `ops/execution-agent-contract.md`。AI、Inspection 或 Recommendation 只可以创建待确认 Execution Task，唔可以跳过 Execution 状态机。

## 通俗比喻

`ops` 就似酒楼同客人、厨房、楼面之间嘅点菜单格式。大家都按同一张单做事，就唔会出现楼面写「牛河」厨房以为系「炒面」嘅情况。
