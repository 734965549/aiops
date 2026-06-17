package ldapsession

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultTTL     = 30 * time.Minute
	redisKeyPrefix = "aiops:ldap_browse:"
)

// Session 保存管理员临时 LDAP 浏览会话（含服务账号凭据，仅短期驻留 Redis/内存）。
type Session struct {
	ID         string                    `json:"id"`
	ProviderID string                    `json:"provider_id"`
	Type       string                    `json:"type"`
	LDAP       config.LDAPProviderConfig `json:"ldap"`
	UserID     string                    `json:"user_id"`
	CreatedAt  time.Time                 `json:"created_at"`
}

// Store 管理 LDAP 浏览会话。
type Store interface {
	Create(ctx context.Context, session Session, ttl time.Duration) error
	Get(ctx context.Context, sessionID, userID string) (*Session, error)
	Delete(ctx context.Context, sessionID, userID string) error
}

// MemoryStore 开发环境无 Redis 时的进程内会话存储。
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]memoryEntry
}

type memoryEntry struct {
	session   Session
	expiresAt time.Time
}

// NewMemoryStore 构造内存会话存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]memoryEntry)}
}

func (s *MemoryStore) Create(_ context.Context, session Session, ttl time.Duration) error {
	if s == nil {
		return errors.New("memory store is nil")
	}
	if session.ID == "" {
		session.ID = uuid.NewString()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[session.ID] = memoryEntry{session: session, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *MemoryStore) Get(_ context.Context, sessionID, userID string) (*Session, error) {
	if s == nil {
		return nil, errors.New("memory store is nil")
	}
	s.mu.RLock()
	entry, ok := s.data[sessionID]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, domain.ErrSessionNotFound
	}
	if userID != "" && entry.session.UserID != userID {
		return nil, domain.ErrSessionNotFound
	}
	session := entry.session
	return &session, nil
}

func (s *MemoryStore) Delete(_ context.Context, sessionID, userID string) error {
	if s == nil {
		return errors.New("memory store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[sessionID]
	if !ok {
		return nil
	}
	if userID != "" && entry.session.UserID != userID {
		return nil
	}
	delete(s.data, sessionID)
	return nil
}

// RedisStore 将会话存入 Redis，适合多实例与生产环境。
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore 构造 Redis 会话存储。
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Create(ctx context.Context, session Session, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return errors.New("redis store is nil")
	}
	if session.ID == "" {
		session.ID = uuid.NewString()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, redisKeyPrefix+session.ID, payload, ttl).Err()
}

func (s *RedisStore) Get(ctx context.Context, sessionID, userID string) (*Session, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("redis store is nil")
	}
	raw, err := s.client.Get(ctx, redisKeyPrefix+sessionID).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, err
	}
	if userID != "" && session.UserID != userID {
		return nil, domain.ErrSessionNotFound
	}
	return &session, nil
}

func (s *RedisStore) Delete(ctx context.Context, sessionID, userID string) error {
	if s == nil || s.client == nil {
		return errors.New("redis store is nil")
	}
	if userID == "" {
		return s.client.Del(ctx, redisKeyPrefix+sessionID).Err()
	}
	session, err := s.Get(ctx, sessionID, userID)
	if err != nil {
		return nil
	}
	if session.UserID != userID {
		return nil
	}
	return s.client.Del(ctx, redisKeyPrefix+sessionID).Err()
}
