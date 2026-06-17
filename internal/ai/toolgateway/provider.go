package toolgateway

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
)

// ProviderType 描述工具提供方协议。
type ProviderType string

const (
	ProviderTypeA ProviderType = "a"
	ProviderTypeB ProviderType = "b"
	ProviderTypeC ProviderType = "c"
)

// ProviderConfig 是内存中的 provider 配置，含敏感字段，禁止直接序列化对外返回。
type ProviderConfig struct {
	ID          string
	Name        string
	Type        ProviderType
	BaseURL     string
	APIKey      string
	TimeoutMS   int64
	Headers     map[string]string
	Enabled     bool
	Description string
}

// ProviderPublic 是对外暴露的 provider 摘要，不含明文 api_key。
type ProviderPublic struct {
	ID          string
	Name        string
	Type        ProviderType
	BaseURL     string
	HasAPIKey   bool
	TimeoutMS   int64
	Headers     map[string]string
	Enabled     bool
	Description string
}

// Public 将内部配置转换为对外安全摘要。
func (c ProviderConfig) Public() ProviderPublic {
	return ProviderPublic{
		ID:          c.ID,
		Name:        c.Name,
		Type:        c.Type,
		BaseURL:     c.BaseURL,
		HasAPIKey:   strings.TrimSpace(c.APIKey) != "",
		TimeoutMS:   c.TimeoutMS,
		Headers:     c.Headers,
		Enabled:     c.Enabled,
		Description: c.Description,
	}
}

// AuditFields 返回可安全写入审计日志的 provider 字段，不含 api_key 明文。
func (c ProviderConfig) AuditFields() map[string]any {
	pub := c.Public()
	return map[string]any{
		"id":          pub.ID,
		"name":        pub.Name,
		"type":        pub.Type,
		"base_url":    pub.BaseURL,
		"has_api_key": pub.HasAPIKey,
		"timeout_ms":  pub.TimeoutMS,
		"enabled":     pub.Enabled,
	}
}

// ToolExecutor 统一工具执行器接口。
type ToolExecutor interface {
	Type() ProviderType
	Validate(cfg ProviderConfig) error
	Invoke(ctx context.Context, cfg ProviderConfig, req ToolRequest, policy OutboundPolicy) (*ToolResponse, error)
}

// ProviderRegistry 统一管理多 provider 配置与执行器。
// HTTP 层会并发读写 providers，须通过 mu 保护底层 map。
type ProviderRegistry struct {
	mu        sync.RWMutex
	executors map[ProviderType]ToolExecutor
	providers map[string]ProviderConfig
	policy    OutboundPolicy
}

func NewProviderRegistry() *ProviderRegistry {
	return NewProviderRegistryWithPolicy(DefaultOutboundPolicy())
}

func NewProviderRegistryWithPolicy(policy OutboundPolicy) *ProviderRegistry {
	return &ProviderRegistry{
		executors: map[ProviderType]ToolExecutor{},
		providers: map[string]ProviderConfig{},
		policy:    policy,
	}
}

func (r *ProviderRegistry) RegisterExecutor(exec ToolExecutor) {
	if r == nil || exec == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[exec.Type()] = exec
}

func (r *ProviderRegistry) ListProviders() []ProviderPublic {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderPublic, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p.Public())
	}
	return out
}

func (r *ProviderRegistry) UpsertProvider(cfg ProviderConfig) error {
	if r == nil {
		return apperr.New(apperr.CodeUnavailable, "provider registry is not configured")
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "provider id is required")
	}
	if cfg.Type == "" {
		return apperr.New(apperr.CodeInvalidArgument, "provider type is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ValidateOutboundURL(context.Background(), cfg.BaseURL, r.policy); err != nil {
		return apperr.Wrap(err, apperr.CodeInvalidArgument, "invalid provider base_url")
	}
	if exec, ok := r.executors[cfg.Type]; ok {
		if err := exec.Validate(cfg); err != nil {
			return appErrorOrWrap(err, apperr.CodeInvalidArgument, "invalid provider config")
		}
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = int64((30 * time.Second) / time.Millisecond)
	}
	r.providers[cfg.ID] = cfg
	return nil
}

func (r *ProviderRegistry) DeleteProvider(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, id)
}

func (r *ProviderRegistry) GetProvider(id string) (ProviderConfig, bool) {
	if r == nil {
		return ProviderConfig{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.providers[id]
	return cfg, ok
}

func (r *ProviderRegistry) Invoke(ctx context.Context, providerID string, req ToolRequest) (*ToolResponse, error) {
	if r == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "provider registry is not configured")
	}
	r.mu.RLock()
	cfg, ok := r.providers[providerID]
	if !ok {
		r.mu.RUnlock()
		return nil, apperr.New(apperr.CodeNotFound, "provider not found")
	}
	if !cfg.Enabled {
		r.mu.RUnlock()
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider is disabled")
	}
	exec, ok := r.executors[cfg.Type]
	policy := r.policy
	r.mu.RUnlock()
	if !ok {
		return nil, apperr.New(apperr.CodeUnavailable, "provider executor is not configured")
	}
	if err := exec.Validate(cfg); err != nil {
		return nil, appErrorOrWrap(err, apperr.CodeFailedPrecondition, "provider config is invalid")
	}
	if err := ValidateOutboundURL(ctx, cfg.BaseURL, policy); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeFailedPrecondition, "provider base_url is invalid")
	}
	resp, err := exec.Invoke(ctx, cfg, req, policy)
	if err != nil {
		return nil, appErrorOrWrap(err, apperr.CodeUnavailable, "invoke tool provider failed")
	}
	return resp, nil
}

func appErrorOrWrap(err error, code apperr.Code, message string) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return err
	}
	return apperr.Wrap(err, code, message)
}
