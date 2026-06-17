package webhookidempotency

import "time"

const (
	defaultPollInterval = 50 * time.Millisecond
	// defaultMaxWait 默认 90s，大于 server.write_timeout_s 默认值 30s，供无 ctx deadline 场景兜底。
	defaultMaxWait = 90 * time.Second
	// defaultStoreResultTimeout 写入幂等结果缓存的独立超时，与请求 ctx 脱钩。
	defaultStoreResultTimeout   = 5 * time.Second
	defaultStoreResultRetries   = 3
	defaultFailedStoreMarkerTTL = 30 * time.Second
)

// Config 控制幂等等待行为；生产建议 DefaultMaxWait 明显大于 HTTP 写超时。
type Config struct {
	// PollInterval 轮询 Redis 是否已有结果的间隔。
	PollInterval time.Duration
	// DefaultMaxWait ctx 无 deadline 时的最长等待；有 deadline 时以 ctx 为准。
	DefaultMaxWait time.Duration
	// StoreResultTimeout fn 成功后写入结果缓存的独立超时；0 使用 defaultStoreResultTimeout。
	StoreResultTimeout time.Duration
	// FailedStoreMarkerTTL 结果缓存写入失败时 processing marker 的缩短 TTL；0 使用 defaultFailedStoreMarkerTTL。
	FailedStoreMarkerTTL time.Duration
}

func (c Config) pollInterval() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return defaultPollInterval
}

func (c Config) maxWaitDuration() time.Duration {
	if c.DefaultMaxWait > 0 {
		return c.DefaultMaxWait
	}
	return defaultMaxWait
}

func (c Config) storeResultTimeout() time.Duration {
	if c.StoreResultTimeout > 0 {
		return c.StoreResultTimeout
	}
	return defaultStoreResultTimeout
}

func (c Config) failedStoreMarkerTTL(ttl time.Duration) time.Duration {
	markerTTL := defaultFailedStoreMarkerTTL
	if c.FailedStoreMarkerTTL > 0 {
		markerTTL = c.FailedStoreMarkerTTL
	}
	if ttl > 0 && ttl < markerTTL {
		return ttl
	}
	return markerTTL
}
