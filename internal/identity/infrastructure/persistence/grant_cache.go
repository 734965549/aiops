package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/redis/go-redis/v9"
)

const userGrantCacheKeyPrefix = "iam:grant:"

// userGrantCache 为 LoadUserGrantContext 提供可选的短期缓存。
type userGrantCache interface {
	get(ctx context.Context, userID string) (*domain.UserGrantContext, bool, error)
	set(ctx context.Context, userID string, grant *domain.UserGrantContext) error
	delete(ctx context.Context, userID string) error
}

type redisUserGrantCache struct {
	client *redis.Client
	ttl    time.Duration
}

func newRedisUserGrantCache(client *redis.Client, ttl time.Duration) userGrantCache {
	if client == nil || ttl <= 0 {
		return nil
	}
	return &redisUserGrantCache{client: client, ttl: ttl}
}

func grantCacheKey(userID string) string {
	return userGrantCacheKeyPrefix + strings.TrimSpace(userID)
}

func (c *redisUserGrantCache) get(ctx context.Context, userID string) (*domain.UserGrantContext, bool, error) {
	if c == nil || c.client == nil {
		return nil, false, nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, false, nil
	}
	data, err := c.client.Get(ctx, grantCacheKey(userID)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var grant domain.UserGrantContext
	if err := json.Unmarshal(data, &grant); err != nil {
		_ = c.client.Del(ctx, grantCacheKey(userID)).Err()
		return nil, false, nil
	}
	return &grant, true, nil
}

func (c *redisUserGrantCache) set(ctx context.Context, userID string, grant *domain.UserGrantContext) error {
	if c == nil || c.client == nil || grant == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	data, err := json.Marshal(grant)
	if err != nil {
		return fmt.Errorf("marshal user grant context: %w", err)
	}
	return c.client.Set(ctx, grantCacheKey(userID), data, c.ttl).Err()
}

func (c *redisUserGrantCache) delete(ctx context.Context, userID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	return c.client.Del(ctx, grantCacheKey(userID)).Err()
}
