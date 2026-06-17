package oauthstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultTTL     = 10 * time.Minute
	redisKeyPrefix = "aiops:oauth_state:"
)

// Binding 记录 OAuth state 绑定嘅客户端上下文，用嚟降低 state 被拎走后跨客户端重放嘅风险。
type Binding struct {
	ClientIP  string
	UserAgent string
}

// Store 管理 OAuth/OIDC state，负责 CSRF 校验、provider 绑定同回调一次性消费。
type Store interface {
	Issue(ctx context.Context, providerID string, binding Binding, ttl time.Duration) (string, error)
	Consume(ctx context.Context, state, providerID string, binding Binding) error
}

// MemoryStore 係本地内存版 state store，适合单实例或测试环境。
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]memoryEntry
}

// memoryEntry 保存 state 对应嘅 provider、客户端指纹同过期时间。
type memoryEntry struct {
	providerID string
	binding    string
	expiresAt  time.Time
}

// NewMemoryStore 创建内存版 OAuth state store。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]memoryEntry)}
}

// Issue 签发一个 state，并同 provider 及客户端指纹一齐存起。
func (s *MemoryStore) Issue(_ context.Context, providerID string, binding Binding, ttl time.Duration) (string, error) {
	if s == nil {
		return "", errors.New("memory store is nil")
	}
	providerID = trim(providerID)
	if providerID == "" {
		return "", errors.New("provider_id is required")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	state, err := identityprovider.NewOAuthState()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[state] = memoryEntry{providerID: providerID, binding: binding.Fingerprint(), expiresAt: time.Now().Add(ttl)}
	return state, nil
}

// Consume 校验并消费 state；成功后立即删除，防止同一个回调被重复使用。
func (s *MemoryStore) Consume(_ context.Context, state, providerID string, binding Binding) error {
	if s == nil {
		return errors.New("memory store is nil")
	}
	state = trim(state)
	providerID = trim(providerID)
	if state == "" || providerID == "" {
		return domain.ErrOAuthStateInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[state]
	if !ok || time.Now().After(entry.expiresAt) {
		return domain.ErrOAuthStateNotFound
	}
	if entry.providerID != providerID || entry.binding != binding.Fingerprint() {
		return domain.ErrOAuthStateInvalid
	}
	delete(s.data, state)
	return nil
}

// RedisStore 係 Redis 版 state store，适合多 API 实例共享 OAuth 回调状态。
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore 创建 Redis 版 OAuth state store。
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// Issue 签发一个 state，并用 TTL 存到 Redis。
func (s *RedisStore) Issue(ctx context.Context, providerID string, binding Binding, ttl time.Duration) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("redis store is nil")
	}
	providerID = trim(providerID)
	if providerID == "" {
		return "", errors.New("provider_id is required")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	state, err := identityprovider.NewOAuthState()
	if err != nil {
		return "", err
	}
	if err := s.client.Set(ctx, redisKeyPrefix+state, encodeEntry(providerID, binding.Fingerprint()), ttl).Err(); err != nil {
		return "", err
	}
	return state, nil
}

var consumeStateScript = redis.NewScript(`
local val = redis.call("GET", KEYS[1])
if not val then
  return 0
end
if val ~= ARGV[1] then
  return -1
end
redis.call("DEL", KEYS[1])
return 1
`)

// Consume 用 Lua 脚本原子校验同删除 state，确保一次性消费。
func (s *RedisStore) Consume(ctx context.Context, state, providerID string, binding Binding) error {
	if s == nil || s.client == nil {
		return errors.New("redis store is nil")
	}
	state = trim(state)
	providerID = trim(providerID)
	if state == "" || providerID == "" {
		return domain.ErrOAuthStateInvalid
	}
	expected := encodeEntry(providerID, binding.Fingerprint())
	result, err := consumeStateScript.Run(ctx, s.client, []string{redisKeyPrefix + state}, expected).Int()
	if err != nil {
		return err
	}
	switch result {
	case 1:
		return nil
	case 0:
		return domain.ErrOAuthStateNotFound
	default:
		return domain.ErrOAuthStateInvalid
	}
}

// Fingerprint 将 IP 同 User-Agent 做摘要；store 只保存指纹，唔保存完整 UA。
func (b Binding) Fingerprint() string {
	ip := trim(b.ClientIP)
	ua := trim(b.UserAgent)
	if ip == "" && ua == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(ip + "\x00" + ua))
	return hex.EncodeToString(sum[:])
}

// encodeEntry 将 provider 同客户端指纹组合成可比较嘅存储值。
func encodeEntry(providerID, binding string) string {
	return providerID + "\n" + binding
}

// trim 统一处理配置同请求参数两边可能带住嘅空白。
func trim(v string) string {
	return strings.TrimSpace(v)
}
