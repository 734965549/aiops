package provider

import (
	"context"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/google/uuid"
)

type baseChecker struct {
	provider domain.ProviderType
}

func (b baseChecker) Provider() domain.ProviderType { return b.provider }

func okCheck(account domain.IntegrationAccount, caps []domain.Capability, message string) *domain.ConnectivityCheck {
	return &domain.ConnectivityCheck{
		CheckID:      "chk-" + uuid.NewString(),
		AccountID:    account.AccountID,
		Status:       domain.ConnectivityOK,
		Provider:     account.Provider,
		Capabilities: caps,
		Message:      message,
		CheckedAt:    time.Now(),
	}
}

// HuaweiCloudChecker 华为云占位连通性探测。
type HuaweiCloudChecker struct{ baseChecker }

func NewHuaweiCloudChecker() *HuaweiCloudChecker {
	return &HuaweiCloudChecker{baseChecker{provider: domain.ProviderHuaweiCloud}}
}

func (c *HuaweiCloudChecker) CheckConnectivity(_ context.Context, account domain.IntegrationAccount, material domain.CredentialMaterial) (*domain.ConnectivityCheck, error) {
	switch account.AuthType {
	case domain.AuthNone:
	case domain.AuthAKSK:
		if material["access_key"] == "" || material["secret_key"] == "" {
			return nil, domain.ErrCredentialRequired
		}
	case domain.AuthAgency:
		if material["agency_name"] == "" || material["domain_name"] == "" {
			return nil, domain.ErrCredentialRequired
		}
	default:
		return nil, domain.ErrInvalidAuthType
	}
	if len(account.Regions) == 0 && account.AuthType != domain.AuthNone {
		return nil, domain.ErrConnectivityFailed
	}
	return okCheck(account, domain.DefaultCapabilitiesForProvider(domain.ProviderHuaweiCloud), "ok"), nil
}

// SigNozChecker Signoz 占位连通性探测。
type SigNozChecker struct{ baseChecker }

func NewSigNozChecker() *SigNozChecker {
	return &SigNozChecker{baseChecker{provider: domain.ProviderSigNoz}}
}

func (c *SigNozChecker) CheckConnectivity(_ context.Context, account domain.IntegrationAccount, material domain.CredentialMaterial) (*domain.ConnectivityCheck, error) {
	if account.AuthType != domain.AuthAPIToken && account.AuthType != domain.AuthNone {
		return nil, domain.ErrInvalidAuthType
	}
	if account.AuthType == domain.AuthAPIToken {
		token := strings.TrimSpace(material["api_token"])
		if token == "" {
			token = strings.TrimSpace(material["access_token"])
		}
		if token == "" {
			return nil, domain.ErrCredentialRequired
		}
	}
	return okCheck(account, domain.DefaultCapabilitiesForProvider(domain.ProviderSigNoz), "ok"), nil
}

// PrometheusChecker Prometheus 占位连通性探测。
type PrometheusChecker struct{ baseChecker }

func NewPrometheusChecker() *PrometheusChecker {
	return &PrometheusChecker{baseChecker{provider: domain.ProviderPrometheus}}
}

func (c *PrometheusChecker) CheckConnectivity(_ context.Context, account domain.IntegrationAccount, material domain.CredentialMaterial) (*domain.ConnectivityCheck, error) {
	if account.AuthType == domain.AuthAPIToken {
		if material["api_token"] == "" && material["access_token"] == "" {
			return nil, domain.ErrCredentialRequired
		}
	}
	if baseURL := strings.TrimSpace(material["base_url"]); baseURL == "" && account.AuthType != domain.AuthNone {
		return nil, domain.ErrConnectivityFailed
	}
	return okCheck(account, domain.DefaultCapabilitiesForProvider(domain.ProviderPrometheus), "ok"), nil
}

// AllCheckers 返回第一阶段占位 Provider checker 列表。
func AllCheckers() []domain.ProviderChecker {
	return []domain.ProviderChecker{
		NewHuaweiCloudChecker(),
		NewSigNozChecker(),
		NewPrometheusChecker(),
	}
}
