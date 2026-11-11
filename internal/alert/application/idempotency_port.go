package application

import (
	"context"
	"time"
)

// WebhookIdempotencyStore 是 Webhook X-Request-ID 跨实例幂等端口
// （ops/alert-contract.md §3.2、§7.2）。Redis / 内存实现位于
// internal/alert/infrastructure/webhookidempotency，由 cmd/api 装配注入。
//
// 语义：key 非空时尽力保证 fn 全局只执行一次，成功后将结果缓存至 ttl。
// 结果缓存写入失败时返回 UNAVAILABLE（可重试），由调用方 5xx 重试。
type WebhookIdempotencyStore interface {
	Do(ctx context.Context, key string, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error)
}
