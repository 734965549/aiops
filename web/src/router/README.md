# web/src/router 模块说明

路由是前端页面与 URL 的唯一映射来源：登录页、布局内 14 个业务页面和 404 兜底都在 `index.ts` 的 `routes` 中静态声明；守卫只做登录态判断，权限校验全部下沉到 API 拦截器与后端。

## 装配与调用关系

```text
main.ts
  -> createPinia() 先于 router 安装（守卫内使用 auth store）
  -> app.use(router)
  -> beforeEach 守卫检查登录态
  -> afterEach 设置 document.title
  -> 匹配路由渲染 views 页面

api/request.ts 401 登出时
  -> 动态 import('@/router') 取当前路由 meta.public
  -> router.push('/login?redirect=<当前路径>')
  （动态导入避免 request.ts 与 router 的循环依赖）
```

## 路由表（截至当前实现）

| 路径 | 页面 | meta | 说明 |
| --- | --- | --- | --- |
| `/login` | `views/login` | `public` | 登录/OAuth/域账号登录 |
| `/` | `layouts/BasicLayout` | — | 登录后统一布局，缺省重定向 `/dashboard`，全局浮层与 AI 控制器在此层 provide |
| `/dashboard` | `views/dashboard` | title/icon | 运维总览 |
| `/ai-assistant` | `views/ai-assistant` | title/icon | AI 助手独立工作台 |
| `/ai-assistant/shared/:token` | `views/ai-assistant/SharedSession.vue` | title | 只读分享，仍需登录 + `app:ai:chat`（契约 §10.18）；不设 public，未登录先去登录页 |
| `/alerts` | `views/alerts` | title/icon | 告警中心，深链 `?alert_id=` |
| `/assets` | `views/assets` | title/icon | CMDB 资产，深链 `?application_id=`、`?resource_id=`、`?tab=match-rules`，筛选/排序状态入 URL |
| `/assets/topology` | `views/assets/topology` | title/icon | 资源拓扑，深链 `?application_id=` |
| `/executions` | `views/executions` | title/icon | 自动化执行，深链 `?task_id=` |
| `/runbooks` | `views/runbooks` | title/icon | Runbook 预案 |
| `/integrations` | `views/integrations` | title/icon | 云账号接入 |
| `/observability` | `views/observability` | title/icon | 观测查询 |
| `/inspections` | `views/inspections` | title/icon | 智能巡检 |
| `/audits` | `views/audits` | title/icon | 审计中心 |
| `/system` | `views/system` | title/icon/`activeMenu=/system` | 系统设置外壳，缺省重定向 `/system/ai-config`；侧栏只有这一个入口，三个配置页在页面内切换 |
| `/system/ai-config` | `views/system/ai-config` | title/icon | Provider/工具调试/知识/记忆管理 |
| `/identity/access-control` | `views/identity/access-control` | title/icon | 权限管理；**绝对路径子路由**，URL 在 `/identity` 下但渲染在 system 外壳内 |
| `/identity/ldap-import` | `views/identity/ldap-import` | title/icon | 域账号导入，同上 |
| `/:pathMatch(.*)*` | `views/error/404.vue` | `public` | 404 兜底，独立渲染（不在 BasicLayout 内） |

## 守卫语义（`beforeEach`）

1. `auth.token` 为空时先 `loadFromStorage()` 从 localStorage 恢复（刷新页面场景）。
2. 已登录访问 `/login`：跳转 `?redirect` 指定路径（必须以 `/` 开头，否则回 `/dashboard`）。
3. `meta.public` 路由（登录、404）直接放行。
4. 有 token 放行；无 token 跳转 `/login?redirect=<to.fullPath>`。

`afterEach` 把 `meta.title` 拼为 `<页面> - AI 运维平台` 写入 `document.title`。

## 权限边界

- 守卫**只校验 token 存在**，不做 RBAC/数据范围/菜单权限过滤；受保护 API 的 403 由 `api/request.ts` 拦截器提示，页面保留结构。
- 侧栏 `layouts/navigation.ts` 是静态分组配置（工作台/运维处置/资源与观测/平台治理，共 12 项），不读取角色权限；动态菜单未实现，不能据菜单可见性推断权限。
- `BasicLayout` 高亮规则：`meta.activeMenu` 优先（`/identity/*` 借此高亮「系统设置」），否则按 `navigationItems` 最长前缀匹配，兜底当前 `route.path`。
- 深链查询参数（`alert_id`/`application_id`/`resource_id`/`task_id`/`tab`）由各页面自行读取与清理，router 不统一处理；语义见 `web/src/api/README.md` 各模块页面说明。

## 测试边界

路由表与守卫当前没有专项 Vitest；守卫行为由登录/登出相关页面测试与人工验收间接覆盖。修改路由或守卫后，需人工核对：未登录重定向、登录后回跳、`/identity/*` 高亮与 404 兜底。
