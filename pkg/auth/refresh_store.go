package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const refreshTokenKeyPrefix = "auth:refresh:"

// RefreshTokenStore 管理 refresh token 会话（轮换与吊销）。
type RefreshTokenStore interface {
	// Register 登记新的 refresh token jti，ttl 应与 token 过期一致。
	Register(ctx context.Context, userID, jti string, ttl time.Duration) error
	// Validate 校验 jti 是否仍有效。
	Validate(ctx context.Context, userID, jti string) (bool, error)
	// Revoke 吊销单个 refresh token。
	Revoke(ctx context.Context, userID, jti string) error
	// RevokeAll 吊销用户全部 refresh token（强制下线）。
	RevokeAll(ctx context.Context, userID string) error
}

// NewRefreshTokenJTI 生成 refresh token 唯一标识。
func NewRefreshTokenJTI() string {
	return uuid.NewString()
}

func refreshTokenKey(userID, jti string) string {
	return refreshTokenKeyPrefix + userID + ":" + jti
}

func refreshTokenUserPattern(userID string) string {
	return refreshTokenKeyPrefix + userID + ":*"
}

// RedisRefreshTokenStore 基于 Redis 的 refresh token 会话存储。
type RedisRefreshTokenStore struct {
	client *redis.Client
}

func NewRedisRefreshTokenStore(client *redis.Client) *RedisRefreshTokenStore {
	return &RedisRefreshTokenStore{client: client}
}

func (s *RedisRefreshTokenStore) Register(ctx context.Context, userID, jti string, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("redis refresh store is not configured")
	}
	if userID == "" || jti == "" {
		return fmt.Errorf("userID and jti are required")
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return s.client.Set(ctx, refreshTokenKey(userID, jti), "1", ttl).Err()
}

func (s *RedisRefreshTokenStore) Validate(ctx context.Context, userID, jti string) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("redis refresh store is not configured")
	}
	n, err := s.client.Exists(ctx, refreshTokenKey(userID, jti)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *RedisRefreshTokenStore) Revoke(ctx context.Context, userID, jti string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("redis refresh store is not configured")
	}
	return s.client.Del(ctx, refreshTokenKey(userID, jti)).Err()
}

func (s *RedisRefreshTokenStore) RevokeAll(ctx context.Context, userID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("redis refresh store is not configured")
	}
	iter := s.client.Scan(ctx, 0, refreshTokenUserPattern(userID), 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return s.client.Del(ctx, keys...).Err()
}

// NoopRefreshTokenStore 在 Redis 不可用时降级：不强制轮换，Validate 恒为 true。
type NoopRefreshTokenStore struct{}

func (NoopRefreshTokenStore) Register(context.Context, string, string, time.Duration) error {
	return nil
}
func (NoopRefreshTokenStore) Validate(context.Context, string, string) (bool, error) {
	return true, nil
}
func (NoopRefreshTokenStore) Revoke(context.Context, string, string) error { return nil }
func (NoopRefreshTokenStore) RevokeAll(context.Context, string) error      { return nil }
