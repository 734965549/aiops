package application

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// ProviderSecretCipher 是 provider api_key 的加解密端口，由装配层注入。
type ProviderSecretCipher interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
}

// ProviderRuntime 是 provider 运行时注册表端口。
type ProviderRuntime interface {
	ValidateProvider(cfg toolgateway.ProviderConfig) error
	UpsertProvider(cfg toolgateway.ProviderConfig) error
	DeleteProvider(id string)
}

// AnalysisProviderResolver 为告警分析动态解析当前 provider，支持前端换模型/启停后无需重启。
type AnalysisProviderResolver interface {
	AnalysisProviderID() (string, bool)
}

// ProviderSeed 是配置文件兜底种子，仅在数据库为空时写入。
type ProviderSeed struct {
	ID          string
	Name        string
	Type        string
	BaseURL     string
	Model       string
	APIKey      string
	TimeoutMS   int64
	Headers     map[string]string
	Enabled     bool
	Description string
}

// UpsertProviderInput 是 provider 写请求（不含密文）。
type UpsertProviderInput struct {
	ID          string
	Name        string
	Type        string
	BaseURL     string
	Model       string
	APIKey      string
	TimeoutMS   int64
	Headers     map[string]string
	Enabled     bool
	Description string
}

// ProviderService 编排 provider 的持久化、启动加载与运行时注册。
type ProviderService struct {
	store   domain.ProviderRepository
	runtime ProviderRuntime
	cipher  ProviderSecretCipher
}

func NewProviderService(store domain.ProviderRepository, runtime ProviderRuntime, cipher ProviderSecretCipher) *ProviderService {
	return &ProviderService{store: store, runtime: runtime, cipher: cipher}
}

// Bootstrap 在启动时加载 provider：数据库为空时先用配置种子写入，再统一从数据库加载。
// 单条记录加载失败只降级该 provider，不阻断 API 启动。
func (s *ProviderService) Bootstrap(ctx context.Context, seeds []ProviderSeed) error {
	if s == nil || s.store == nil || s.runtime == nil {
		return apperr.New(apperr.CodeUnavailable, "ai provider service is not configured")
	}
	records, err := s.store.List(ctx)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "load ai providers failed")
	}
	if len(records) == 0 && len(seeds) > 0 {
		for _, seed := range seeds {
			if strings.TrimSpace(seed.ID) == "" {
				continue
			}
			record := domain.Provider{
				ProviderID: strings.TrimSpace(seed.ID), Name: seed.Name, Type: seed.Type,
				BaseURL: seed.BaseURL, Model: seed.Model, Headers: seed.Headers,
				TimeoutMS: normalizeProviderTimeout(seed.TimeoutMS), Enabled: seed.Enabled, Description: seed.Description,
			}
			if key := strings.TrimSpace(seed.APIKey); key != "" {
				ciphertext, err := s.encrypt(key)
				if err != nil {
					return apperr.Wrap(err, apperr.CodeInternal, "encrypt seed provider api_key failed")
				}
				record.APIKeyCiphertext = ciphertext
				record.HasAPIKey = true
			}
			if err := s.store.Upsert(ctx, record); err != nil {
				return apperr.Wrap(err, apperr.CodeInternal, "seed ai providers failed")
			}
		}
		if records, err = s.store.List(ctx); err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "load ai providers failed")
		}
	}

	loaded := 0
	for _, record := range records {
		apiKey, err := s.decrypt(record.APIKeyCiphertext)
		if err != nil {
			logger.From(ctx).Warn("ai provider skipped at startup: decrypt api_key failed",
				logger.String("provider_id", record.ProviderID), logger.Error(err))
			continue
		}
		cfg := runtimeConfig(record, apiKey)
		if err := s.runtime.UpsertProvider(cfg); err != nil {
			logger.From(ctx).Warn("ai provider skipped at startup: invalid config",
				logger.String("provider_id", record.ProviderID), logger.Error(err))
			continue
		}
		loaded++
	}
	logger.From(ctx).Info("ai providers loaded from database", logger.Int("count", loaded))
	return nil
}

// ListProviders 以数据库为事实源返回脱敏列表。
func (s *ProviderService) ListProviders(ctx context.Context) ([]toolgateway.ProviderPublic, error) {
	if s == nil || s.store == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai provider service is not configured")
	}
	records, err := s.store.List(ctx)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai providers failed")
	}
	out := make([]toolgateway.ProviderPublic, 0, len(records))
	for _, record := range records {
		out = append(out, toolgateway.ProviderPublic{
			ID: record.ProviderID, Name: record.Name, Type: toolgateway.ProviderType(record.Type),
			BaseURL: record.BaseURL, Model: record.Model, HasAPIKey: record.HasAPIKey,
			TimeoutMS: record.TimeoutMS, Headers: record.Headers, Enabled: record.Enabled,
			Description: record.Description,
		})
	}
	return out, nil
}

// UpsertProvider 校验配置、加密密钥并同时写入数据库与运行时注册表。
// 更新时省略 api_key 表示保留原密钥。
func (s *ProviderService) UpsertProvider(ctx context.Context, in UpsertProviderInput) error {
	if s == nil || s.store == nil || s.runtime == nil {
		return apperr.New(apperr.CodeUnavailable, "ai provider service is not configured")
	}
	providerID := strings.TrimSpace(in.ID)
	if providerID == "" || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Type) == "" || strings.TrimSpace(in.BaseURL) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "id, name, type and base_url are required")
	}
	existing, err := s.store.Get(ctx, providerID)
	if err != nil && !errors.Is(err, domain.ErrProviderNotFound) {
		return apperr.Wrap(err, apperr.CodeInternal, "load ai provider failed")
	}
	existingCiphertext := existing != nil && existing.HasAPIKey

	apiKey := strings.TrimSpace(in.APIKey)
	var ciphertext []byte
	if apiKey == "" {
		if !existingCiphertext {
			return apperr.New(apperr.CodeInvalidArgument, "api_key is required for new provider")
		}
		ciphertext = existing.APIKeyCiphertext
		if apiKey, err = s.decrypt(ciphertext); err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "decrypt provider api_key failed")
		}
	} else if ciphertext, err = s.encrypt(apiKey); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "encrypt provider api_key failed")
	}

	record := domain.Provider{
		ProviderID: providerID, Name: strings.TrimSpace(in.Name), Type: strings.TrimSpace(in.Type),
		BaseURL: strings.TrimSpace(in.BaseURL), Model: strings.TrimSpace(in.Model),
		Headers: in.Headers, TimeoutMS: normalizeProviderTimeout(in.TimeoutMS), Enabled: in.Enabled,
		Description: strings.TrimSpace(in.Description), APIKeyCiphertext: ciphertext, HasAPIKey: true,
	}
	cfg := runtimeConfig(record, apiKey)
	if err := s.runtime.ValidateProvider(cfg); err != nil {
		return err
	}
	if err := s.store.Upsert(ctx, record); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "persist ai provider failed")
	}
	if err := s.runtime.UpsertProvider(cfg); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "apply ai provider failed")
	}
	return nil
}

// DeleteProvider 删除数据库记录并同步移除运行时 provider。
func (s *ProviderService) DeleteProvider(ctx context.Context, providerID string) error {
	if s == nil || s.store == nil || s.runtime == nil {
		return apperr.New(apperr.CodeUnavailable, "ai provider service is not configured")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "provider id is required")
	}
	if err := s.store.Delete(ctx, providerID); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "delete ai provider failed")
	}
	s.runtime.DeleteProvider(providerID)
	return nil
}

func normalizeProviderTimeout(ms int64) int64 {
	if ms <= 0 {
		return 30000
	}
	return ms
}

func (s *ProviderService) encrypt(plaintext string) ([]byte, error) {
	if s.cipher == nil {
		return nil, errors.New("ai provider secret cipher is not configured")
	}
	return s.cipher.Encrypt(plaintext)
}

func (s *ProviderService) decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	if s.cipher == nil {
		return "", errors.New("ai provider secret cipher is not configured")
	}
	return s.cipher.Decrypt(ciphertext)
}

func runtimeConfig(record domain.Provider, apiKey string) toolgateway.ProviderConfig {
	return toolgateway.ProviderConfig{
		ID: record.ProviderID, Name: record.Name, Type: toolgateway.ProviderType(record.Type),
		BaseURL: record.BaseURL, Model: record.Model, APIKey: apiKey,
		TimeoutMS: record.TimeoutMS, Headers: record.Headers, Enabled: record.Enabled,
		Description: record.Description,
	}
}
