// Package webhookidempotency 提供 Alert Webhook X-Request-ID 跨实例幂等存储（ops/alert-contract.md §3.2、§7.2）。
//
// 生产环境（redis.required=true）必须使用 RedisStore，多 Pod 共享同一 Redis。
// 单实例开发可在 redis.required=false 时降级为 MemoryStore。
package webhookidempotency

import (
	"context"
	"time"
)

// Store 在 key 非空时尽力保证 fn 全局只执行一次，正常路径下成功后将结果缓存至 ttl。
//
// 正常路径：fn 成功且结果写入缓存后，同 key 在 ttl 内复用缓存、不再执行 fn。
// 异常路径：fn 已成功但结果缓存写入失败时，Do 返回 pkg/errors.CodeUnavailable（可重试），
// 并保留缩短 TTL 的 processing marker，使并发同 key 请求在短窗口内等待而非长时间悬挂。
// marker 过期后若仍无缓存，后续请求可能再次执行 fn——属 Redis 不可用时的极端降级，调用方应依赖 5xx 重试。
//
// key 由 application 层组装为 source_id + "\x00" + request_id。
type Store interface {
	Do(ctx context.Context, key string, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error)
}
