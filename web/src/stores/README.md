# web/src/stores 模块说明

`stores` 存放跨页面共享的 Pinia 状态。当前共两个 store：`auth`（登录态、JWT 与当前用户）与 `aiContext`（页面上下文与 AI 浮层开关）。AI 对话控制器不经过 Pinia，由 `layouts/BasicLayout` 以 `provide/inject` 提供（见 `internal/ai/README.md` §11）。

## auth store（`auth.ts`）

### 状态

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `token` | `ref<string>` | access token，写入 `Authorization: Bearer` |
| `refreshToken` | `ref<string>` | refresh token，仅用于续期与登出吊销 |
| `user` | `ref<CurrentUser \| null>` | 当前用户摘要 |

本地持久化键名：`aiops_token`、`aiops_refresh_token`、`aiops_user`。**当前实现将 access/refresh token 与用户摘要写入 localStorage，与项目“不将 Token 写入前端持久化状态”的安全约束冲突，需单独安全整改**（`docs/documentation-review.md` §5.3 已登记）。

### 方法

| 方法 | 说明 |
| --- | --- |
| `login(username, password, providerId?)` | 调用 `POST /api/identity/login`；传 `providerId` 时走 `login/external` 域账号登录 |
| `loginWithOAuthCallback(providerId, code, state?)` | OAuth 回调换发 `POST /api/identity/oauth/:provider_id/callback`，保存 TokenPair |
| `refresh()` | 调用 `POST /api/identity/refresh`，轮换 token 对；并发 401 时复用同一 in-flight Promise，避免重复轮换；无 refresh token 直接抛错 |
| `logout()` | **先清除本地状态**，再 best-effort 调 `POST /api/identity/logout` 吊销 refresh token；远端吊销失败不回滚本地清除 |
| `loadFromStorage()` | 从 localStorage 恢复 token / user（路由守卫、刷新页面时使用；user JSON 解析失败置 null） |
| `applyTokenPair(pair)` | 登录/刷新成功后统一写入 token、refreshToken、user |
| `setToken` / `setRefreshToken` / `setUser` | 细粒度更新（如 `GET /api/identity/me` 后刷新 user） |

### 调用关系

```text
views/login
  -> auth.login() / auth.loginWithOAuthCallback()
  -> api/identity.login() / completeOAuthCallback()
  -> auth.applyTokenPair()
  -> router -> /dashboard

api/request.ts（401 拦截）
  -> auth.refresh() 重试原请求（单次，_retry 防循环）
  -> 失败则 auth.logout() + 跳转 /login?redirect=<当前路径>

layouts/BasicLayout
  -> onMounted 无 token 直接跳过；有 token 时 fetchCurrentUser -> auth.setUser（失败不阻断布局）
  -> auth.logout() -> /login

router/index.ts 守卫
  -> token 为空时 auth.loadFromStorage() 恢复会话
```

契约见 `ops/auth-contract.md`；`refresh()` 通常由 `request.ts` 拦截器自动触发，页面无需直接调用。

## aiContext store（`aiContext.ts`）

跨页共享「当前页面上下文」与「全局 AI 浮层」两个状态；只存业务 ID 与标题，**不写 localStorage**，不保存凭据、告警正文或自由文本。

### 状态

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `current` | `ref<AIPageContext>` | `page`/`page_title` + 资源级字段（`alert_id`/`application_id`/`resource_id`/`resource_type`/`execution_id`/`time_range`），字段语义与长度上限见契约 §10.8 |
| `overlayVisible` | `ref<boolean>` | 全局 AI 浮层抽屉开关 |
| `overlaySessionPicker` | `ref<boolean>` | 浮层是否先显示会话选择器 |
| `hasResourceContext` | computed | 任一资源级 ID 非空；`execution_id` 与 `resource_id` 同等视为资源级 |
| `summary` | computed | 中文摘要（页面标题 + 告警/应用/资源/执行任务/时间范围），浮层顶栏与独立页展示 |

### 方法

| 方法 | 说明 |
| --- | --- |
| `setPageContext(context)` | 页面主动登记资源级上下文（告警详情、执行详情、观测查询等写入方）；**整体替换**资源级字段（未提供即清空），仅 `page` 缺省沿用当前值，trim 后空串转 undefined |
| `setPageBase(page, pageTitle)` | 路由切换时登记页面基础字段；**同页重入只更新标题、保留资源级字段**，页基变化则整体重置 |
| `clearResourceContext(page, pageTitle)` | 关闭详情/切页时清空资源级字段，仅保留页面基础字段 |
| `toRequestContext()` | 输出随消息发送的 `ChatSessionContext`（仅非空字段） |
| `openOverlay({ selectSession? })` | 打开浮层；`selectSession: true` 为显式入口（告警页「问 AI 助手」），先显示会话选择器 |
| `closeOverlay()` / `toggleOverlay()` | 关闭浮层 / 顶栏按钮与 `Alt + A` 切换（toggle 打开时不带选择器） |

### 调用关系与跨页语义

```text
路由切换（BasicLayout watch route.path）
  -> setPageBase(page, title)；/ai-assistant 特例：已有资源上下文时不覆盖，保留来源页资源

各页面写入方（契约 §10.8）
  告警详情 -> setPageContext({alert_id, application_id, resource_id, resource_type, time_range})
  执行详情 -> setPageContext({execution_id, ...来源/目标字段})
  观测查询 -> setPageContext({resource_id, resource_type, time_range})
  关闭详情/切页 -> clearResourceContext

useChat 发送消息/新建会话
  -> requestContext = toRequestContext()；排队条目按入队时刻快照冻结
```

- 资源级字段在同页路由重入时保留（如告警详情刷新），页基变化即清理；`execution_id` 不被路由切换覆盖。
- 浮层开关与会话选择器标志是纯 UI 状态，不持久化；刷新页面后浮层关闭。
- 覆盖：`aiContext.spec.ts` 6 个用例（资源 ID 保留与凭证类自由文本丢弃、`execution_id` 资源级、切页清理、同页重入保留、页基变化重置、显式选择器入口）。
