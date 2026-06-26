package huawei

import (
	"context"
	"errors"
	"testing"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
)

func TestValidateCESMetricQuery(t *testing.T) {
	base := domain.ProviderContext{Account: domain.AccountSnapshot{
		ProjectID: "project-1",
		Regions:   []string{"cn-north-4"},
	}}
	valid := domain.MetricQuery{
		Region:     "cn-north-4",
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1,
		To:         2,
	}
	if err := validateCESMetricQuery(base, valid); err != nil {
		t.Fatalf("expected valid query, got %v", err)
	}
}

func TestValidateCESMetricQueryRequiresRegion(t *testing.T) {
	err := validateCESMetricQuery(domain.ProviderContext{Account: domain.AccountSnapshot{
		ProjectID: "project-1",
	}}, domain.MetricQuery{
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestValidateCESMetricQueryRejectsUnknownRegion(t *testing.T) {
	err := validateCESMetricQuery(domain.ProviderContext{Account: domain.AccountSnapshot{
		ProjectID: "project-1",
		Regions:   []string{"cn-north-4"},
	}}, domain.MetricQuery{
		Region:     "cn-south-1",
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestValidateCESMetricQueryRequiresProjectID(t *testing.T) {
	err := validateCESMetricQuery(domain.ProviderContext{Account: domain.AccountSnapshot{}}, domain.MetricQuery{
		Region:     "cn-north-4",
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestValidateCESMetricQueryRequiresDimensions(t *testing.T) {
	err := validateCESMetricQuery(domain.ProviderContext{Account: domain.AccountSnapshot{
		ProjectID: "project-1",
	}}, domain.MetricQuery{
		Region:    "cn-north-4",
		Namespace: "SYS.ECS",
		Metric:    "cpu_util",
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestMapCESError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode apperr.Code
	}{
		{
			name:     "bad request",
			err:      &sdkerr.ServiceResponseError{StatusCode: 400, ErrorCode: "CES.0001"},
			wantCode: apperr.CodeInvalidArgument,
		},
		{
			name:     "auth failed",
			err:      &sdkerr.ServiceResponseError{StatusCode: 401},
			wantCode: apperr.CodeUnavailable,
		},
		{
			name:     "permission denied",
			err:      &sdkerr.ServiceResponseError{StatusCode: 403},
			wantCode: apperr.CodeUnavailable,
		},
		{
			name:     "not found",
			err:      &sdkerr.ServiceResponseError{StatusCode: 404},
			wantCode: apperr.CodeNotFound,
		},
		{
			name:     "rate limit",
			err:      &sdkerr.ServiceResponseError{StatusCode: 429},
			wantCode: apperr.CodeResourceExhausted,
		},
		{
			name:     "service unavailable",
			err:      &sdkerr.ServiceResponseError{StatusCode: 503},
			wantCode: apperr.CodeUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := apperr.MapSentinels(mapCESError(tt.err), "query metrics failed",
				apperr.Sentinel{Err: domain.ErrInvalidArgument, Code: apperr.CodeInvalidArgument},
				apperr.Sentinel{Err: domain.ErrNotFound, Code: apperr.CodeNotFound},
				apperr.Sentinel{Err: domain.ErrProviderUnavailable, Code: apperr.CodeUnavailable},
			)
			if direct := mapCESError(tt.err); apperr.CodeOf(direct) != apperr.CodeInternal {
				mapped = direct
			}
			if apperr.CodeOf(mapped) != tt.wantCode {
				t.Fatalf("expected code %s, got %s (%v)", tt.wantCode, apperr.CodeOf(mapped), mapped)
			}
			if containsSensitiveSDKText(mapped.Error(), tt.err) {
				t.Fatalf("mapped error leaked sdk details: %q", mapped.Error())
			}
		})
	}
}

func containsSensitiveSDKText(mappedMsg string, raw error) bool {
	var svcErr *sdkerr.ServiceResponseError
	if !errors.As(raw, &svcErr) {
		return false
	}
	if svcErr.ErrorMessage != "" && mappedMsg == svcErr.ErrorMessage {
		return true
	}
	if svcErr.EncodedAuthorizationMessage != "" && mappedMsg == svcErr.EncodedAuthorizationMessage {
		return true
	}
	return false
}

func TestAdapterQueryMetricsUsesFakeForAuthNone(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	series, err := adapter.QueryMetrics(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "none"},
	}, domain.MetricQuery{
		Metric: "cpu_util",
		From:   100,
		To:     200,
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(series) != 1 {
		t.Fatalf("expected fake series, got %d", len(series))
	}
}

func TestAdapterQueryMetricsAKSKRequiresValidation(t *testing.T) {
	adapter := NewAdapter(NewCredentialProvider(nil, nil), NewCESClient(), nil)
	_, err := adapter.QueryMetrics(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk", ProjectID: "p1"},
	}, domain.MetricQuery{
		Metric: "cpu_util",
		From:   100,
		To:     200,
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestAdapterSearchLogsUnsupportedForAKSK(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	_, err := adapter.SearchLogs(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk"},
	}, domain.LogQuery{Limit: 10})
	if !errors.Is(err, domain.ErrCapabilityUnsupported) {
		t.Fatalf("expected ErrCapabilityUnsupported, got %v", err)
	}
}

func TestAdapterSearchLogsUsesFakeForAuthNone(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	entries, err := adapter.SearchLogs(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "none"},
	}, domain.LogQuery{Limit: 10})
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected fake log entries")
	}
}

// mockCESDiscovery 用于测试 Adapter.ListAllResources 的 ces 路由。
type mockCESDiscovery struct {
	req    CESResourceDiscoveryRequest
	result *CESResourceDiscoveryResult
	err    error
	called bool
}

func (m *mockCESDiscovery) ListCESResources(_ context.Context, _ AKSKCredential, req CESResourceDiscoveryRequest) (*CESResourceDiscoveryResult, error) {
	m.called = true
	m.req = req
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// mockResourceDiscovery 用于测试 native 路由。
type mockResourceDiscovery struct {
	limit int
	out   []domain.CloudResource
	err   error
}

func (m *mockResourceDiscovery) ListResources(_ context.Context, _ AKSKCredential, _, _, _ string, limit int) ([]domain.CloudResource, error) {
	m.limit = limit
	if m.err != nil {
		return nil, m.err
	}
	return m.out, nil
}

func TestAdapterListAllResourcesCESRoute(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-ces"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{{ResourceID: "ces:r:SYS.ECS:i-1", Type: "ecs", Region: "cn-south-1"}},
		Summary:   CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 1, Discovered: 1, ResourceGroupName: "全部资源"},
	}}
	adapter := NewAdapter(provider, nil, nil).WithCESResourceDiscovery(cesMock)
	resources, summary, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-ces",
			ExtraConfig:     []byte(`{"max_resources":3}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if !cesMock.called {
		t.Fatalf("expected CES discovery client to be called")
	}
	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(resources))
	}
	if summary == nil || summary.CESTotal != 1 || summary.ResourceGroupName != "全部资源" {
		t.Fatalf("summary = %+v, want CESTotal=1 group=全部资源", summary)
	}
	if cesMock.req.MaxResources != 3 {
		t.Fatalf("MaxResources = %d, want 3 from extra_config", cesMock.req.MaxResources)
	}
}

func TestAdapterListAllResourcesNativeRoute(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-native"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	resMock := &mockResourceDiscovery{out: []domain.CloudResource{{ResourceID: "ecs-1", Type: "ecs"}}}
	adapter := NewAdapter(provider, nil, resMock)
	// extra_config 指定 sync_mode=native。
	_, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-native",
			ExtraConfig:     []byte(`{"sync_mode":"native","max_resources":5}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 50})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if resMock.limit != 5 {
		t.Fatalf("native route limit = %d, want 5 from extra_config", resMock.limit)
	}
}

func TestAdapterListAllResourcesAgencyUnsupported(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	_, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "agency"},
	}, obsapp.AssetFullSyncQuery{Region: "cn-south-1"})
	if !errors.Is(err, domain.ErrCapabilityUnsupported) {
		t.Fatalf("expected ErrCapabilityUnsupported, got %v", err)
	}
}

func TestAdapterListAllResourcesAuthNoneUsesFake(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	resources, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "none"},
	}, obsapp.AssetFullSyncQuery{Region: "cn-north-4", MaxResources: 10})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("expected fake resources for auth_type=none")
	}
}
