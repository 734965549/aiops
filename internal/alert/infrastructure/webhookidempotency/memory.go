package webhookidempotency

import (
	"context"
	"sync"
	"time"
)

// MemoryStore 与 RedisStore 语义一致的进程内实现。
// 仅适用于单实例开发（redis.required=false）；生产多 Pod 须使用 RedisStore。
type MemoryStore struct {
	mu    sync.Mutex
	items map[string]memoryEntry
	cfg   Config
}

type memoryEntry struct {
	processing bool
	result     []byte
	expiresAt  time.Time
}

// NewMemoryStore 构造测试用内存幂等存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]memoryEntry)}
}

func (s *MemoryStore) Do(ctx context.Context, key string, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error) {
	if key == "" {
		return fn()
	}
	deadline := resolveWaitDeadline(ctx, s.cfg)
	ticker := time.NewTicker(s.cfg.pollInterval())
	defer ticker.Stop()

	for {
		if cached, ok, acquired := s.tryGetOrAcquire(key, ttl); ok {
			return cached, nil
		} else if acquired {
			result, err := fn()
			if err != nil {
				s.release(key)
				return nil, err
			}
			s.storeResult(key, result, ttl)
			return result, nil
		}
		if err := waitForPoll(ctx, deadline, ticker); err != nil {
			return nil, err
		}
	}
}

func (s *MemoryStore) tryGetOrAcquire(key string, ttl time.Duration) (result []byte, cached bool, acquired bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	e, ok := s.items[key]
	if ok && now.After(e.expiresAt) {
		delete(s.items, key)
		ok = false
	}
	if ok && !e.processing && e.result != nil {
		cp := make([]byte, len(e.result))
		copy(cp, e.result)
		return cp, true, false
	}
	if ok && e.processing {
		return nil, false, false
	}
	s.items[key] = memoryEntry{processing: true, expiresAt: now.Add(ttl)}
	return nil, false, true
}

func (s *MemoryStore) storeResult(key string, result []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(result))
	copy(cp, result)
	s.items[key] = memoryEntry{result: cp, expiresAt: time.Now().Add(ttl)}
}

func (s *MemoryStore) release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
}
