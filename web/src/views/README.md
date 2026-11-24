# web/src/views 模块说明

`views` 按路由组织前端页面：目录名与路由一一对应，页面内再按职责拆分 `components/` 与 `composables/`。页面只负责组合与展示；API 调用统一经 `web/src/api/`，跨页状态放 `web/src/stores/aiContext.ts`（页面上下文）与 `auth.ts`（登录态），AI 对话控制器由 `layouts/BasicLayout` 提供并跨页共享。

## 页面清单（与 router/index.ts 一致）

| 路径 | 页面目录 | 关键能力 |
| --- | --- | --- |
| `/login` | `login/` | 本地/LDAP/OAuth 登录；`?redirect=` 回跳；OAuth state 暂存 sessionStorage |
| `/dashboard` | `dashboard/` | 告警/执行/资产/Runbook 聚合摘要 |
| `/ai-assistant` | `ai-assistant/` | 独立对话工作台（三栏）；`SharedSession.vue` 渲染 `/ai-assistant/shared/:token` 只读分享，当前不展示附图 |
| `/alerts` | `alerts/` | 告警列表/详情/时间线/操作；深链 `?alert_id=`；「AI 分析」弹窗（无 diagnosis 展示）与「问 AI 助手」（带上下文打开浮层选择器） |
| `/assets` | `assets/` | 注册表/云同步/匹配规则 Tab；筛选排序与 `?application_id=`、`?resource_id=`、`?tab=match-rules` 入 URL |
| `/assets/topology` | `assets/topology/` | 手工与云派生资源拓扑，`?application_id=` 预选 |
| `/executions` | `executions/` | 任务列表/详情/确认/时间线；深链 `?task_id=`；执行提议跳转落点 |
| `/runbooks` | `views/runbooks/` | 模板列表/详情/启停；推荐区在告警页内 |
| `/integrations` | `integrations/` | 云账号 CRUD、CES 同步配置、字段级连接检查 |
| `/observability` | `observability/` | 指标/日志/链路/拓扑统一只读查询，查询成功写资源上下文 |
| `/inspections` | `inspections/` | 巡检策略/运行/Finding/Recommendation |
| `/audits` | `audits/` | 审计筛选/详情/聚合/CSV |
| `/system` | `system/` | 设置外壳（页面内切换三个子页）；`system/ai-config/` 含 ProviderManager、ToolConsole、KnowledgeManager、MemoryManager |
| `/identity/*` | `identity/access-control/`、`identity/ldap-import/` | 权限管理（角色/数据范围/AI 工具权限全量替换）、域账号导入（LDAP 浏览会话） |
| 404 | `error/404.vue` | 兜底页，public 路由独立渲染 |

## 约定与边界

- 页面级组合逻辑放当前 view 的 `composables/`（如 `useChat`、`useAssetRegistry`、`alertAIContext`），可复用组件放当前模块 `components/`；不建全局 `common/utils` 聚合层。
- 深链查询参数由页面读取与清理（`alert_id`/`application_id`/`resource_id`/`task_id`/`tab`），router 不统一处理；刷新后按 URL 还原是各页面自查项。
- 403 由全局拦截器提示，页面保留结构不白屏；未登录访问受保护页由路由守卫重定向 `/login`。
- 各页面已有能力的实现边界（无 E2E、无专项 Vitest、字段展示缺口等）登记在 `web/src/api/README.md` 对应模块小节与 `docs/documentation-review.md`，本清单不重复罗列。
