// Package http 是 Integration 上下文的 HTTP 适配层。
//
// 路由前缀 /api/integrations/accounts；鉴权 Bearer + app:integrations:{read|create|update|delete|check}。
// 统一响应走 httpx.OK / httpx.Fail，契约见 ops/cloud-observability-contract.md §4。
package http
