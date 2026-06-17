# web 前端模块说明（粤语版）

## 呢个模块做咩

`web` 系 AI 运维平台嘅前端界面，即系用户见到同点击嘅「门面」。后端负责做菜，前端负责餐牌、点餐、显示结果。

当前技术栈大致系：

- Vite + Vue。
- TypeScript。
- Pinia 状态管理。
- Vue Router 路由。
- API 请求封装。

## 调用关系

```text
用户浏览器
  -> web/src/views 页面
  -> web/src/stores 状态
  -> web/src/api 请求封装
  -> 后端 /api/identity/* /healthz 等接口
```

启动开发服务时：

```text
npm install
npm run dev
```

生产构建：

```text
npm run build
```

若在 Windows 网盘映射目录（如 `D:\E盘\...`）或 junction 路径下构建失败，报错
`emitted chunks and assets must be strings ... received ".../index.html"`，
`vite.config.ts` 已通过 `root: realpathSync` + `preserveSymlinks` 规避。
仍失败时请将仓库放到纯 ASCII 本地路径（如 `C:\dev\aiops`）再构建。

## Alert 模块 E2E

后端与数据库就绪后，可在仓库根目录执行：

```powershell
.\scripts\e2e-alert.ps1
```

默认连 `http://127.0.0.1:8080`，账号 `admin` / `admin123`。覆盖接入源、Webhook 入库、去重、列表/详情、状态流转与 AI 分析入口；转派/静默在脚本外另行 API 验证。验收明细见 `ops/alert-contract.md` §12.1。

告警页面路由：`/alerts`（`web/src/views/alerts/index.vue`）。

执行任务 E2E（需先跑 `e2e-alert.ps1` 或提供 `-AlertId`）：

```powershell
.\scripts\e2e-execution.ps1
```

执行任务页面：`/executions`（`web/src/views/executions/index.vue`）；告警详情可「创建执行任务」，确认后跳转执行页，结果回写时间线。

Runbook 全链路 E2E（需迁移 `0012`，可独立运行或 `-AlertId`）：

```powershell
.\scripts\e2e-runbook.ps1
```

覆盖：告警 ingest → processing → 预案推荐 → 从 Runbook 创建任务（dry-run）→ 确认 → 执行 → 时间线回写。详见 `ops/runbook-contract.md`。

## 入参

| 来源 | 入参 | 说明 |
| --- | --- | --- |
| 用户操作 | 表单、点击、路由跳转 | 例如登录页输入用户名密码 |
| `.env.development` / `.env.production` | `VITE_API_BASE` 等 | dev 走 Vite proxy；生产见 `deployments/README.md` |
| 后端响应 | token、用户信息、错误码 | 前端保存状态同提示用户 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 页面 UI | 登录页、Dashboard、错误页、占位页 |
| HTTP 请求 | 调用后端接口 |
| 本地状态 | 保存 token、用户资料、登录状态 |

## 通俗比喻

`web` 就似餐厅大厅同餐牌。客人唔会入厨房同厨师讲 SQL，佢只会睇菜单、点餐、等上菜。前端就负责令呢个过程清晰、顺手、好睇。