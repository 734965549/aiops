package persistence

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
)

type memoryUserGrantCache struct {
	mu    sync.Mutex
	items map[string][]byte
	ttl   time.Duration
}

func newMemoryUserGrantCache(ttl time.Duration) *memoryUserGrantCache {
	return &memoryUserGrantCache{items: map[string][]byte{}, ttl: ttl}
}

func (c *memoryUserGrantCache) get(_ context.Context, userID string) (*domain.UserGrantContext, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.items[grantCacheKey(userID)]
	if !ok {
		return nil, false, nil
	}
	var grant domain.UserGrantContext
	if err := json.Unmarshal(data, &grant); err != nil {
		delete(c.items, grantCacheKey(userID))
		return nil, false, nil
	}
	return &grant, true, nil
}

func (c *memoryUserGrantCache) set(_ context.Context, userID string, grant *domain.UserGrantContext) error {
	if grant == nil {
		return nil
	}
	data, err := json.Marshal(grant)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[grantCacheKey(userID)] = data
	return nil
}

func (c *memoryUserGrantCache) delete(_ context.Context, userID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, grantCacheKey(userID))
	return nil
}

func TestRedisUserGrantCacheRoundTrip(t *testing.T) {
	mem := newMemoryUserGrantCache(time.Minute)
	grant := &domain.UserGrantContext{
		Roles:       []domain.Role{{ID: "r1", Code: "admin"}},
		Permissions: []domain.Permission{{Code: "app:ai.tools:invoke"}},
		DataScopes: []domain.DataScope{{
			Code: "all-data", ScopeType: domain.DataScopeAll, ScopeConfig: map[string]any{"tags": []any{"prod"}},
		}},
		AIToolPermissions: []domain.AIToolPermission{{ToolCode: "alarm.analyze", PermissionMode: domain.AIToolPermissionReadOnly}},
	}
	ctx := context.Background()
	if err := mem.set(ctx, "u1", grant); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := mem.get(ctx, "u1")
	if err != nil || !ok || got == nil {
		t.Fatalf("get failed: ok=%v err=%v", ok, err)
	}
	if len(got.Roles) != 1 || got.Roles[0].Code != "admin" {
		t.Fatalf("unexpected roles: %+v", got.Roles)
	}
	if len(got.Permissions) != 1 || got.Permissions[0].Code != "app:ai.tools:invoke" {
		t.Fatalf("unexpected permissions: %+v", got.Permissions)
	}
}

func TestAccessControlRepositoryGrantCacheHit(t *testing.T) {
	repo := &AccessControlRepository{
		grantCache: newMemoryUserGrantCache(time.Minute),
		loadUserGrantContext: func(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
			t.Fatal("db loader should not be called on cache hit")
			return nil, nil
		},
	}
	cached := &domain.UserGrantContext{
		Roles:       []domain.Role{{ID: "r1", Code: "operator"}},
		Permissions: []domain.Permission{{Code: "app:user:read"}},
	}
	ctx := context.Background()
	if err := repo.grantCache.set(ctx, "u1", cached); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	got, err := repo.LoadUserGrantContext(ctx, "u1")
	if err != nil || got == nil || len(got.Roles) != 1 || got.Roles[0].Code != "operator" {
		t.Fatalf("unexpected grant: %+v err=%v", got, err)
	}
}

func TestAccessControlRepositoryGrantCacheMissAndPopulate(t *testing.T) {
	calls := 0
	repo := &AccessControlRepository{
		grantCache: newMemoryUserGrantCache(time.Minute),
		loadUserGrantContext: func(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
			calls++
			return &domain.UserGrantContext{
				Roles:       []domain.Role{{ID: "r1", Code: "admin"}},
				Permissions: []domain.Permission{{Code: "app:ai.tools:invoke"}},
			}, nil
		},
	}
	ctx := context.Background()
	got, err := repo.LoadUserGrantContext(ctx, "u1")
	if err != nil || got == nil || calls != 1 {
		t.Fatalf("first load failed: %+v calls=%d err=%v", got, calls, err)
	}
	got, err = repo.LoadUserGrantContext(ctx, "u1")
	if err != nil || got == nil || calls != 1 {
		t.Fatalf("second load should hit cache: %+v calls=%d err=%v", got, calls, err)
	}
}

func TestAccessControlRepositoryGrantCacheInvalidateOnUnbindUserRole(t *testing.T) {
	calls := 0
	repo := &AccessControlRepository{
		grantCache: newMemoryUserGrantCache(time.Minute),
		loadUserGrantContext: func(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
			calls++
			return &domain.UserGrantContext{Roles: []domain.Role{{ID: "r1", Code: "admin"}}}, nil
		},
	}
	ctx := context.Background()
	if _, err := repo.LoadUserGrantContext(ctx, "u1"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	repo.invalidateUserGrantCache(ctx, "u1")
	if _, err := repo.LoadUserGrantContext(ctx, "u1"); err != nil {
		t.Fatalf("reload after invalidate: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected db reload after invalidate, calls=%d", calls)
	}
}

func TestAccessControlRepositoryGrantCacheInvalidateOnBindUserRole(t *testing.T) {
	calls := 0
	repo := &AccessControlRepository{
		grantCache: newMemoryUserGrantCache(time.Minute),
		loadUserGrantContext: func(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
			calls++
			return &domain.UserGrantContext{Roles: []domain.Role{{ID: "r1", Code: "admin"}}}, nil
		},
	}
	ctx := context.Background()
	if _, err := repo.LoadUserGrantContext(ctx, "u1"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	repo.invalidateUserGrantCache(ctx, "u1")
	if _, err := repo.LoadUserGrantContext(ctx, "u1"); err != nil {
		t.Fatalf("reload after invalidate: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected db reload after invalidate, calls=%d", calls)
	}
}
