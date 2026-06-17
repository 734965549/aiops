package http

// ops/alert-contract.md §3.1 权限资源域；AuthorizeStatic(resource, action) 对应 app:{resource}:{action}。
const alertAuthResource = "alerts"

// §3.1 建议权限种子与路由 action 映射：
//
//	app:alerts:read         → GET  /api/alerts、GET /api/alerts/:id
//	app:alerts:acknowledge  → POST /api/alerts/:id/acknowledge
//	app:alerts:assign       → POST /api/alerts/:id/assign
//	app:alerts:update       → POST start-processing / recover / comments / ai-analysis
//	app:alerts:close        → POST /api/alerts/:id/close
//	app:alerts:silence      → POST silence / unsilence
//	app:alerts:ingest       → /api/alerts/sources CRUD（非 Webhook 调用）
//
// 种子数据见 migrations/0007_init_alert.up.sql。
// 管理端路由挂 Authed 组（Bearer 必需，无效 token → 401 UNAUTHENTICATED）；
// Webhook 挂 Public 组，不经 Bearer，见 §3.2 handler_ingest.go。
