package webhookidempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/redis/go-redis/v9"
)

const (
	redisKeyPrefix   = "aiops:alert:webhook_idem:"
	processingMarker = "__processing__"
)

// RedisStore 使用 Redis SET NX + 结果缓存实现跨 Pod Webhook 幂等。
type RedisStore struct {
	client *redis.Client
	cfg    Config
}

// NewRedisStore 构造 Redis 幂等存储；client 不可为 nil。
func NewRedisStore(client *redis.Client, cfg Config) *RedisStore {
	return &RedisStore{client: client, cfg: cfg}
}

func (s *RedisStore) redisKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return redisKeyPrefix + hex.EncodeToString(sum[:])
}

// Do 跨实例幂等执行 fn：先占坑 processing，成功后写入 JSON 结果并设置 TTL。
// 重复请求等待窗口跟 ctx deadline 走；无 deadline 时用 cfg.DefaultMaxWait（应大于 HTTP 写超时）。
func (s *RedisStore) Do(ctx context.Context, key string, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("redis idempotency store is nil")
	}
	if key == "" {
		return fn()
	}
	rkey := s.redisKey(key)
	deadline := resolveWaitDeadline(ctx, s.cfg)
	ticker := time.NewTicker(s.cfg.pollInterval())
	defer ticker.Stop()

	for {
		if cached, ok, err := s.readCached(ctx, rkey); err != nil {
			return nil, err
		} else if ok {
			return cached, nil
		}
		acquired, err := s.client.SetNX(ctx, rkey, processingMarker, ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("redis idempotency setnx: %w", err)
		}
		if acquired {
			result, err := fn()
			if err != nil {
				s.releaseKey(rkey)
				return nil, err
			}
			if err := s.storeResult(rkey, result, ttl); err != nil {
				s.shortenProcessingMarker(rkey, ttl)
				return nil, apperr.Wrap(err, apperr.CodeUnavailable, "webhook idempotency result not cached")
			}
			return result, nil
		}
		if err := waitForPoll(ctx, deadline, ticker); err != nil {
			return nil, err
		}
	}
}

// storeResult 用独立于请求 ctx 的短超时写入最终结果，避免 fn 慢执行耗尽 deadline 后缓存写失败。
func (s *RedisStore) storeResult(rkey string, result []byte, ttl time.Duration) error {
	var lastErr error
	for attempt := 0; attempt < defaultStoreResultRetries; attempt++ {
		writeCtx, cancel := context.WithTimeout(context.Background(), s.cfg.storeResultTimeout())
		err := s.client.Set(writeCtx, rkey, result, ttl).Err()
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt+1 < defaultStoreResultRetries {
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
		}
	}
	return fmt.Errorf("redis idempotency store result: %w", lastErr)
}

func (s *RedisStore) shortenProcessingMarker(rkey string, ttl time.Duration) {
	markerTTL := s.cfg.failedStoreMarkerTTL(ttl)
	for attempt := 0; attempt < defaultStoreResultRetries; attempt++ {
		writeCtx, cancel := context.WithTimeout(context.Background(), s.cfg.storeResultTimeout())
		err := s.client.Set(writeCtx, rkey, processingMarker, markerTTL).Err()
		cancel()
		if err == nil {
			return
		}
		if attempt+1 < defaultStoreResultRetries {
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
		}
	}
}

func (s *RedisStore) releaseKey(rkey string) {
	for attempt := 0; attempt < defaultStoreResultRetries; attempt++ {
		writeCtx, cancel := context.WithTimeout(context.Background(), s.cfg.storeResultTimeout())
		err := s.client.Del(writeCtx, rkey).Err()
		cancel()
		if err == nil {
			return
		}
		if attempt+1 < defaultStoreResultRetries {
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
		}
	}
}

func (s *RedisStore) readCached(ctx context.Context, rkey string) ([]byte, bool, error) {
	val, err := s.client.Get(ctx, rkey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if string(val) == processingMarker {
		return nil, false, nil
	}
	return val, true, nil
}
