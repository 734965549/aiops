package provider

import (
	"fmt"
	"strings"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/prometheus"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/signoz"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// Registry 按 provider 类型路由 ProviderEntry；具体能力由 QueryService 按需断言小 Port。
type Registry struct {
	providers map[string]obsapp.ProviderEntry
}

func NewRegistry(entries ...obsapp.ProviderEntry) *Registry {
	m := make(map[string]obsapp.ProviderEntry, len(entries))
	for _, p := range entries {
		if p == nil {
			continue
		}
		key := strings.TrimSpace(p.ProviderType())
		if key == "" {
			continue
		}
		m[key] = p
	}
	return &Registry{providers: m}
}

// DefaultFakeRegistry 返回第一阶段 provider 注册表（厂商 Adapter 均位于 infrastructure/provider/*）。
func DefaultFakeRegistry() *Registry {
	return NewRegistry(
		huawei.NewAdapter(),
		signoz.NewAdapter(),
		prometheus.NewAdapter(),
	)
}

func (r *Registry) Get(provider string) (obsapp.ProviderEntry, error) {
	if r == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "observability provider registry is not configured")
	}
	key := strings.TrimSpace(provider)
	if key == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "provider is required")
	}
	p, ok := r.providers[key]
	if !ok {
		return nil, apperr.Wrap(domain.ErrUnsupportedProvider, apperr.CodeFailedPrecondition,
			fmt.Sprintf("unsupported provider %q", key))
	}
	return p, nil
}
