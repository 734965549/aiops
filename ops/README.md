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

## 出参

| 契约 | 关注出参 |
| --- | --- |
| auth | token、认证错误 |
| identity-api | 角色/权限列表、当前用户、授权判断结果 |
| ai | provider 列表、工具调用结果 |
| health | 服务状态、版本、就绪情况 |
| migration | 执行成功、失败原因、版本记录 |
| execution / runbook | 执行任务、步骤状态、推荐预案、时间线回写结果 |

## 通俗比喻

`ops` 就似酒楼同客人、厨房、楼面之间嘅点菜单格式。大家都按同一张单做事，就唔会出现楼面写「牛河」厨房以为系「炒面」嘅情况。
