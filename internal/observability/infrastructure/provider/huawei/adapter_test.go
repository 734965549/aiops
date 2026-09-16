package huawei

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
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

// TestValidateCESMetricQueryAcceptsRegionProjects 验证顶层 ProjectID 为空、
// 但 region_projects 覆盖查询 region 时校验通过，见 ops/huawei-ces-sync-contract.md §5.3。
func TestValidateCESMetricQueryAcceptsRegionProjects(t *testing.T) {
	extra := []byte(`{"region_projects":[
		{"region":"cn-south-1","project_id":"pid-south"}
	]}`)
	pctx := domain.ProviderContext{Account: domain.AccountSnapshot{
		ProjectID:   "",
		Regions:     []string{"cn-south-1"},
		ExtraConfig: extra,
	}}
	valid := domain.MetricQuery{
		Region:     "cn-south-1",
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1,
		To:         2,
	}
	if err := validateCESMetricQuery(pctx, valid); err != nil {
		t.Fatalf("expected region_projects to satisfy project_id, got %v", err)
	}
}

// TestValidateCESMetricQueryRejectsUncoveredRegion 验证 region_projects 未覆盖查询 region
// 且顶层 ProjectID 为空时仍报错。
func TestValidateCESMetricQueryRejectsUncoveredRegion(t *testing.T) {
	extra := []byte(`{"region_projects":[
		{"region":"cn-south-1","project_id":"pid-south"}
	]}`)
	pctx := domain.ProviderContext{Account: domain.AccountSnapshot{
		ProjectID:   "",
		Regions:     []string{"cn-south-1", "cn-north-4"},
		ExtraConfig: extra,
	}}
	err := validateCESMetricQuery(pctx, domain.MetricQuery{
		Region:     "cn-north-4",
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
	})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for uncovered region, got %v", err)
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

func TestAdapterQueryMetricsAgencyUnsupported(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	_, err := adapter.QueryMetrics(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: "agency"},
	}, domain.MetricQuery{
		Metric: "cpu_util",
		From:   100,
		To:     200,
	})
	if !errors.Is(err, domain.ErrCapabilityUnsupported) {
		t.Fatalf("expected ErrCapabilityUnsupported, got %v", err)
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected code %s, got %s", apperr.CodeFailedPrecondition, apperr.CodeOf(err))
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

func (m *mockResourceDiscovery) ListResources(_ context.Context, _ AKSKCredential, _, _, _ string, limit int) ([]domain.CloudResource, []string, error) {
	m.limit = limit
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.out, nil, nil
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
	// extra_config 未指定 resource_group_name 时，adapter 应传空名，
	// 交由 CES 选组逻辑按 §8.4 step 3 走默认候选名回退，而非短路成"全部资源"精确匹配。
	if cesMock.req.ResourceGroupName != "" {
		t.Fatalf("ResourceGroupName = %q, want empty when extra_config omits it", cesMock.req.ResourceGroupName)
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

// TestAdapterListAllResourcesRegionProjects 验证 region_projects 把不同 project_id 传给 CES client。
// 见 ops/huawei-ces-sync-contract.md §5.3。
func TestAdapterListAllResourcesRegionProjects(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-region-projects"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{{ResourceID: "ces:r:SYS.ECS:i-1", Type: "ecs", Region: "cn-south-1"}},
		Summary:   CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 1, Discovered: 1, ResourceGroupName: "全部资源"},
	}}
	adapter := NewAdapter(provider, nil, nil).WithCESResourceDiscovery(cesMock)
	// Account.ProjectID 为默认 fallback；region_projects 覆盖 cn-south-1 与 cn-north-4。
	extra := []byte(`{"region_projects":[
		{"region":"cn-south-1","project_id":"pid-south"},
		{"region":"cn-north-4","project_id":"pid-north"}
	]}`)

	// region=cn-south-1 命中映射 -> pid-south。
	if _, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "fallback-pid", Regions: []string{"cn-south-1", "cn-north-4"},
			CredentialRefID: "ref-rp",
			ExtraConfig:     extra,
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100}); err != nil {
		t.Fatalf("ListAllResources south: %v", err)
	}
	if cesMock.req.ProjectID != "pid-south" {
		t.Fatalf("south project_id = %q, want pid-south", cesMock.req.ProjectID)
	}

	// region=cn-north-4 命中映射 -> pid-north。
	if _, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "fallback-pid", Regions: []string{"cn-south-1", "cn-north-4"},
			CredentialRefID: "ref-rp",
			ExtraConfig:     extra,
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-north-4", MaxResources: 100}); err != nil {
		t.Fatalf("ListAllResources north: %v", err)
	}
	if cesMock.req.ProjectID != "pid-north" {
		t.Fatalf("north project_id = %q, want pid-north", cesMock.req.ProjectID)
	}

	// region 未在映射中 -> 回落 Account.ProjectID。
	if _, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "fallback-pid", Regions: []string{"cn-east-3"},
			CredentialRefID: "ref-rp",
			ExtraConfig:     extra,
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-east-3", MaxResources: 100}); err != nil {
		t.Fatalf("ListAllResources east: %v", err)
	}
	if cesMock.req.ProjectID != "fallback-pid" {
		t.Fatalf("east project_id = %q, want fallback-pid", cesMock.req.ProjectID)
	}
}

// TestAdapterListAllResourcesRegionProjectResourceGroup 验证 region_projects 把不同
// resource_group_id / resource_group_name 传给 CES client，未命中回落全局值。
// 见 ops/huawei-ces-sync-contract.md §5.3。
func TestAdapterListAllResourcesRegionProjectResourceGroup(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-region-rg"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{{ResourceID: "ces:r:SYS.ECS:i-1", Type: "ecs", Region: "cn-south-1"}},
		Summary:   CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 1, Discovered: 1, ResourceGroupName: "全部资源"},
	}}
	adapter := NewAdapter(provider, nil, nil).WithCESResourceDiscovery(cesMock)
	// 全局 resource_group_id=rg-global、resource_group_name=全局组；region_projects 覆盖 cn-south-1。
	extra := []byte(`{
		"resource_group_id":"rg-global",
		"resource_group_name":"全局组",
		"region_projects":[
			{"region":"cn-south-1","project_id":"pid-south","resource_group_id":"rg-south","resource_group_name":"南方组"},
			{"region":"cn-north-4","project_id":"pid-north"}
		]
	}`)

	// region=cn-south-1 命中映射 -> rg-south / 南方组。
	if _, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "fallback-pid", Regions: []string{"cn-south-1", "cn-north-4"},
			CredentialRefID: "ref-rg",
			ExtraConfig:     extra,
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100}); err != nil {
		t.Fatalf("ListAllResources south: %v", err)
	}
	if cesMock.req.ResourceGroupID != "rg-south" {
		t.Fatalf("south resource_group_id = %q, want rg-south", cesMock.req.ResourceGroupID)
	}
	if cesMock.req.ResourceGroupName != "南方组" {
		t.Fatalf("south resource_group_name = %q, want 南方组", cesMock.req.ResourceGroupName)
	}

	// region=cn-north-4 命中映射但未配资源组 -> 回落全局 rg-global / 全局组。
	if _, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "fallback-pid", Regions: []string{"cn-south-1", "cn-north-4"},
			CredentialRefID: "ref-rg",
			ExtraConfig:     extra,
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-north-4", MaxResources: 100}); err != nil {
		t.Fatalf("ListAllResources north: %v", err)
	}
	if cesMock.req.ResourceGroupID != "rg-global" {
		t.Fatalf("north resource_group_id = %q, want rg-global", cesMock.req.ResourceGroupID)
	}
	if cesMock.req.ResourceGroupName != "全局组" {
		t.Fatalf("north resource_group_name = %q, want 全局组", cesMock.req.ResourceGroupName)
	}

	// region 未在映射中 -> project_id 回落账号值，资源组回落全局值。
	if _, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "fallback-pid", Regions: []string{"cn-east-3"},
			CredentialRefID: "ref-rg",
			ExtraConfig:     extra,
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-east-3", MaxResources: 100}); err != nil {
		t.Fatalf("ListAllResources east: %v", err)
	}
	if cesMock.req.ProjectID != "fallback-pid" {
		t.Fatalf("east project_id = %q, want fallback-pid", cesMock.req.ProjectID)
	}
	if cesMock.req.ResourceGroupID != "rg-global" || cesMock.req.ResourceGroupName != "全局组" {
		t.Fatalf("east resource group = %q/%q, want rg-global/全局组", cesMock.req.ResourceGroupID, cesMock.req.ResourceGroupName)
	}
}

// cesDiscoveryWithAPI 包装一个 mock 的 cesResourceGroupAPI，运行真实 discoverCESResources，
// 用于在 adapter 级测试中验证资源组选择逻辑（含默认候选名回退），见 §8.4。
type cesDiscoveryWithAPI struct {
	api cesResourceGroupAPI
}

func (c *cesDiscoveryWithAPI) ListCESResources(ctx context.Context, cred AKSKCredential, req CESResourceDiscoveryRequest) (*CESResourceDiscoveryResult, error) {
	if err := validateCESDiscoveryRequest(req, cred); err != nil {
		return nil, err
	}
	return discoverCESResources(ctx, c.api, req)
}

// TestAdapterListAllResources_DefaultCandidatesEnglishFallback 验证 extra_config 未指定
// resource_group_name 时，adapter 传空名，CES 选组按 §8.4 step 3 依次尝试默认候选名，
// 仅存在 "All Resources" 分组时也能命中（不再被预填的"全部资源"短路成精确匹配）。
func TestAdapterListAllResources_DefaultCandidatesEnglishFallback(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-default-candidates"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	total := int32(5)
	api := &pagedGroupsAPI{
		groups: []cesv2model.OneResourceGroupResp{
			{GroupName: "All Resources", GroupId: "rg-en", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: &total}},
		},
		showResp: buildShowResp("All Resources", "SYS.ECS,instance_id", 5),
	}
	adapter := NewAdapter(provider, nil, nil).WithCESResourceDiscovery(&cesDiscoveryWithAPI{api: api})
	_, summary, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-default",
			ExtraConfig:     []byte(`{"sync_mode":"ces"}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.ResourceGroupID != "rg-en" || summary.ResourceGroupName != "All Resources" {
		t.Fatalf("resource group = %q/%q, want rg-en/All Resources (default candidate fallback)", summary.ResourceGroupID, summary.ResourceGroupName)
	}
	if summary.ResourceGroupSelection != "default_name" {
		t.Fatalf("ResourceGroupSelection = %q, want default_name", summary.ResourceGroupSelection)
	}
}

func TestAdapterListAllResourcesNativeRoute(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-native"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	// native 仅覆盖旧 4 类（ecs/cce/rds/elb），不触碰 evs/vpc/dcs/dms。
	// cce 返回 0 条（模拟该类资源全部消失），elb 鉴权失败。
	// 预期：ecs/rds/cce 计入 SuccessfulTypes（cce 即使 0 条也算查询成功，供 stale 门控），
	// elb 计入 FailedScopes，整批不失败，ecs+rds 资源正常返回。
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"ecs": {{ResourceID: "ecs-1", Type: "ecs", Region: "cn-south-1"}},
			"rds": {{ResourceID: "rds-1", Type: "rds", Region: "cn-south-1"}},
			"cce": nil,
		},
		errByType: map[string]error{
			"elb": apperr.New(apperr.CodeFailedPrecondition, "huawei elb forbidden"),
		},
	}
	adapter := NewAdapter(provider, nil, resMock)
	// extra_config 指定 sync_mode=native。
	resources, summary, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-native",
			ExtraConfig:     []byte(`{"sync_mode":"native","max_resources":5}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 50})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	// 仅调用旧 4 类，按 legacyNativeResourceTypes 顺序。
	if !reflect.DeepEqual(resMock.calledTypes, []string{"ecs", "cce", "rds", "elb"}) {
		t.Fatalf("native called types = %v, want [ecs cce rds elb]", resMock.calledTypes)
	}
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2 (ecs+rds)", len(resources))
	}
	if summary == nil {
		t.Fatalf("summary is nil")
	}
	// SuccessfulTypes 含 cce（0 条也算查询成功），不含 elb（失败）。
	gotSuccessful := map[string]bool{}
	for _, st := range summary.SuccessfulTypes {
		gotSuccessful[st] = true
	}
	wantSuccessful := map[string]bool{"ecs": true, "cce": true, "rds": true}
	if !reflect.DeepEqual(gotSuccessful, wantSuccessful) {
		t.Fatalf("SuccessfulTypes = %v, want %v", summary.SuccessfulTypes, wantSuccessful)
	}
	if len(summary.FailedScopes) != 1 || !strings.Contains(summary.FailedScopes[0], "elb") {
		t.Fatalf("FailedScopes = %v, want one entry containing elb", summary.FailedScopes)
	}
}

// TestAdapterListAllResourcesNativeTruncation 验证 native 路径 limit+1 截断探测：
// 配置 max_resources=1，云端有 2 台 ECS。预期只返回 1 台，MaxResourcesReached=true，
// SuccessfulTypes 为空（被截断的 ecs 不计入），避免另一台旧 ECS 被误标 stale，见 §13.1。
func TestAdapterListAllResourcesNativeTruncation(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-native-trunc"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"ecs": {
				{ResourceID: "ecs-1", Type: "ecs", Region: "cn-south-1"},
				{ResourceID: "ecs-2", Type: "ecs", Region: "cn-south-1"},
			},
		},
	}
	adapter := NewAdapter(provider, nil, resMock)
	resources, summary, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-native",
			ExtraConfig:     []byte(`{"sync_mode":"native","max_resources":1}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 1})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (truncated to max_resources)", len(resources))
	}
	if summary == nil {
		t.Fatalf("summary is nil")
	}
	if !summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (cloud has more resources than limit)")
	}
	if len(summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (truncated ecs must not be marked successful)", summary.SuccessfulTypes)
	}
}

// TestAdapterListAllResourcesNativeLimitExactlyFilled 验证 native 路径某类资源恰好填满上限时，
// 后续类型虽未扫描，但必须标记 MaxResourcesReached=true，避免批次被误标为完整成功。
func TestAdapterListAllResourcesNativeLimitExactlyFilled(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-native-limit-filled"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"ecs": {
				{ResourceID: "ecs-1", Type: "ecs", Region: "cn-south-1"},
				{ResourceID: "ecs-2", Type: "ecs", Region: "cn-south-1"},
			},
		},
	}
	adapter := NewAdapter(provider, nil, resMock)
	resources, summary, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-native",
			ExtraConfig:     []byte(`{"sync_mode":"native","max_resources":2}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 2})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}
	if summary == nil {
		t.Fatalf("summary is nil")
	}
	if !summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (limit exactly filled, subsequent types not scanned)")
	}
	gotSuccessful := map[string]bool{}
	for _, st := range summary.SuccessfulTypes {
		gotSuccessful[st] = true
	}
	wantSuccessful := map[string]bool{"ecs": true}
	if !reflect.DeepEqual(gotSuccessful, wantSuccessful) {
		t.Fatalf("SuccessfulTypes = %v, want %v", summary.SuccessfulTypes, wantSuccessful)
	}
}

// TestAdapterListAllResourcesNativeAllTypesFail 验证旧 4 类全部失败时不返回空成功结果，
// 避免被误当作“云端无资源”而触发 stale。
func TestAdapterListAllResourcesNativeAllTypesFail(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-native-allfail"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	resMock := &mockResourceDiscoveryByType{
		errByType: map[string]error{
			"ecs": apperr.New(apperr.CodeFailedPrecondition, "forbidden"),
			"cce": apperr.New(apperr.CodeFailedPrecondition, "forbidden"),
			"rds": apperr.New(apperr.CodeFailedPrecondition, "forbidden"),
			"elb": apperr.New(apperr.CodeFailedPrecondition, "forbidden"),
		},
	}
	adapter := NewAdapter(provider, nil, resMock)
	_, _, err := adapter.ListAllResources(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-native",
			ExtraConfig:     []byte(`{"sync_mode":"native"}`),
		},
	}, obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1"})
	if err == nil {
		t.Fatalf("expected error when all native types fail, got nil")
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

// mockResourceDiscoveryByType 按资源类型返回不同结果，用于 hybrid enrichment 测试。
type mockResourceDiscoveryByType struct {
	byType         map[string][]domain.CloudResource
	errByType      map[string]error
	warningsByType map[string][]string
	calledTypes    []string
	limit          int
}

func (m *mockResourceDiscoveryByType) ListResources(_ context.Context, _ AKSKCredential, _, _, resourceType string, limit int) ([]domain.CloudResource, []string, error) {
	m.calledTypes = append(m.calledTypes, resourceType)
	m.limit = limit
	if err, ok := m.errByType[resourceType]; ok {
		return nil, nil, err
	}
	all := m.byType[resourceType]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	var warnings []string
	if w, ok := m.warningsByType[resourceType]; ok {
		warnings = w
	}
	return all, warnings, nil
}

func TestAdapterListAllResourcesHybridRoute(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			{
				ResourceID: "ces:r:SYS.ECS:i-1", Name: "web-01", Type: "ecs",
				Region: "cn-south-1", ProviderRef: "i-1",
				Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id", "private_ip": "10.9.9.9"},
			},
			{
				ResourceID: "ces:r:SYS.RDS:rds-1", Name: "db-main", Type: "rds",
				Region: "cn-south-1", ProviderRef: "rds-1",
				Labels: map[string]string{"namespace": "SYS.RDS", "dim_name": "rds_cluster_id"},
			},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 2, Discovered: 2, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"ecs": {{
				ResourceID: "ecs-i-1", Type: "ecs", ProviderRef: "i-1",
				Labels: map[string]string{"instance_id": "i-1", "private_ip": "10.0.0.1", "flavor": "s6.large.2", "vpc_id": "vpc-aaa", "az": "cn-south-1a"},
			}},
			"rds": {{
				ResourceID: "rds-rds-1", Type: "rds", ProviderRef: "rds-1",
				Labels: map[string]string{"private_ip": "192.168.1.1", "vpc_id": "vpc-bbb", "subnet_id": "subnet-1", "flavor": "rds.pg.c2.medium"},
			}},
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-hybrid",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, summary, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if !cesMock.called {
		t.Fatal("expected CES discovery client to be called for hybrid mode")
	}
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}
	// 阶段一：基础资源只含 CES labels，无原生增强 label。
	if resources[0].Labels["flavor"] != "" {
		t.Fatalf("basic CES resource should not contain native label before enrichment: %+v", resources[0].Labels)
	}
	// 阶段二：基础资源落库后调用独立增强（见 §8.2 两阶段拆分）。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// 验证 ECS 资源 labels 合并：CES labels 保留，native labels 新增。
	ecs := resources[0]
	if ecs.Labels["namespace"] != "SYS.ECS" {
		t.Fatalf("CES label namespace overwritten: %+v", ecs.Labels)
	}
	if ecs.Labels["private_ip"] != "10.9.9.9" {
		t.Fatalf("CES label private_ip overwritten: %+v", ecs.Labels)
	}
	if ecs.Labels["flavor"] != "s6.large.2" {
		t.Fatalf("native label flavor missing: %+v", ecs.Labels)
	}
	// 验证 RDS 资源 labels 合并。
	rds := resources[1]
	if rds.Labels["namespace"] != "SYS.RDS" {
		t.Fatalf("CES label namespace overwritten: %+v", rds.Labels)
	}
	if rds.Labels["vpc_id"] != "vpc-bbb" {
		t.Fatalf("native label vpc_id missing: %+v", rds.Labels)
	}
	if rds.Labels["subnet_id"] != "subnet-1" {
		t.Fatalf("native label subnet_id missing: %+v", rds.Labels)
	}
	// 验证摘要。
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if enrichResult.Summary.EnrichedCount != 2 {
		t.Fatalf("EnrichedCount = %d, want 2", enrichResult.Summary.EnrichedCount)
	}
	if len(enrichResult.Summary.EnrichmentFailedTypes) != 0 {
		t.Fatalf("EnrichmentFailedTypes = %v, want empty", enrichResult.Summary.EnrichmentFailedTypes)
	}
	// enrichResources 用 maxResources+1 探测截断，故 limit=101。
	if resMock.limit != 101 {
		t.Fatalf("enrichment limit = %d, want 101", resMock.limit)
	}
}

func TestAdapterListAllResourcesHybridEnrichmentFailure(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-fail"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			{
				ResourceID: "ces:r:SYS.ECS:i-1", Name: "web-01", Type: "ecs",
				Region: "cn-south-1", ProviderRef: "i-1",
				Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id"},
			},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 1, Discovered: 1, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		errByType: map[string]error{
			"ecs": apperr.New(apperr.CodeUnavailable, "ecs api permission denied"),
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-hybrid-fail",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, summary, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	// 阶段一：CES 资源仍应返回，不被增强失败丢弃。
	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (CES base not dropped)", len(resources))
	}
	if resources[0].Labels["namespace"] != "SYS.ECS" {
		t.Fatalf("CES label should be preserved: %+v", resources[0].Labels)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	// 阶段二：增强阶段记录类型失败，但不返回错误（增强失败不影响基础入库）。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources should not return error for per-type failure: %v", err)
	}
	if enrichResult.Summary.EnrichedCount != 0 {
		t.Fatalf("EnrichedCount = %d, want 0", enrichResult.Summary.EnrichedCount)
	}
	if len(enrichResult.Summary.EnrichmentFailedTypes) != 1 || enrichResult.Summary.EnrichmentFailedTypes[0] != "ecs" {
		t.Fatalf("EnrichmentFailedTypes = %v, want [ecs]", enrichResult.Summary.EnrichmentFailedTypes)
	}
	if enrichResult.Summary.EnrichmentFailedCount != 1 {
		t.Fatalf("EnrichmentFailedCount = %d, want 1", enrichResult.Summary.EnrichmentFailedCount)
	}
}

// TestAdapterListAllResourcesHybridEnrichmentWarnings 验证 best-effort warnings 产出：
// DMS 单一子服务失败、VPC subnet_count 失败时，资源正常返回且 warnings 进入 summary.EnrichmentWarnings，
// 不进 EnrichmentFailedTypes，不影响批次状态。见 §8.2。
func TestAdapterListAllResourcesHybridEnrichmentWarnings(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-warn"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			{
				ResourceID: "ces:r:SYS.DMS:inst-1", Name: "kafka-01", Type: "dms",
				Region: "cn-south-1", ProviderRef: "inst-1",
				Labels: map[string]string{"namespace": "SYS.DMS", "dim_name": "instance_id"},
			},
			{
				ResourceID: "ces:r:SYS.VPC:vpc-1", Name: "vpc-01", Type: "vpc",
				Region: "cn-south-1", ProviderRef: "vpc-1",
				Labels: map[string]string{"namespace": "SYS.VPC", "dim_name": "vpc_id"},
			},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 2, Discovered: 2, ResourceGroupName: "全部资源"},
	}}
	// DMS 返回成功但带 dms.kafka warning；VPC 返回成功但带 vpc.subnet_count warning。
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"dms": {
				{ResourceID: "dms:inst-1", Type: "dms", Region: "cn-south-1", ProviderRef: "inst-1",
					Labels: map[string]string{"engine": "kafka"}},
			},
			"vpc": {
				{ResourceID: "vpc:vpc-1", Type: "vpc", Region: "cn-south-1", ProviderRef: "vpc-1",
					Labels: map[string]string{"cidr": "192.168.0.0/16"}},
			},
		},
		warningsByType: map[string][]string{
			"dms": {"dms.kafka"},
			"vpc": {"vpc.subnet_count"},
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-hybrid-warn",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, _, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}
	// 阶段二：增强成功，warnings 进入 EnrichmentWarnings，不进 EnrichmentFailedTypes。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// 增强 label 合并成功。
	if resources[0].Labels["engine"] != "kafka" {
		t.Fatalf("DMS native label engine missing: %+v", resources[0].Labels)
	}
	if resources[1].Labels["cidr"] != "192.168.0.0/16" {
		t.Fatalf("VPC native label cidr missing: %+v", resources[1].Labels)
	}
	if enrichResult.Summary.EnrichedCount != 2 {
		t.Fatalf("EnrichedCount = %d, want 2", enrichResult.Summary.EnrichedCount)
	}
	// 不应有 failed types。
	if len(enrichResult.Summary.EnrichmentFailedTypes) != 0 || enrichResult.Summary.EnrichmentFailedCount != 0 {
		t.Fatalf("EnrichmentFailedTypes = %v, count = %d, want empty/0",
			enrichResult.Summary.EnrichmentFailedTypes, enrichResult.Summary.EnrichmentFailedCount)
	}
	// warnings 应包含 dms.kafka 和 vpc.subnet_count。
	if len(enrichResult.Summary.EnrichmentWarnings) != 2 {
		t.Fatalf("EnrichmentWarnings = %v, want 2 warnings", enrichResult.Summary.EnrichmentWarnings)
	}
	warningSet := map[string]bool{}
	for _, w := range enrichResult.Summary.EnrichmentWarnings {
		warningSet[w] = true
	}
	if !warningSet["dms.kafka"] || !warningSet["vpc.subnet_count"] {
		t.Fatalf("EnrichmentWarnings = %v, want dms.kafka and vpc.subnet_count", enrichResult.Summary.EnrichmentWarnings)
	}
}

// TestAdapterListAllResourcesHybridEVSVPCDCSDMS 验证 EVS/VPC/DCS/DMS 四类原生增强 label 合并与失败计入。
//
// 注意（§21.4）：本用例的 CES 资源与 mock 原生资源 ProviderRef 被人为写成相等
// （disk-01/disk-01、vpc-1/vpc-1），并不符合真实 CES dim 格式，因此掩盖了 EVS/VPC
// 匹配键不成立的问题。真实 dim 格式下的不匹配行为见 TestAdapterListAllResourcesHybridEVSVPCRealDimNoMatch。
func TestAdapterListAllResourcesHybridEVSVPCDCSDMS(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-newtypes"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			{ResourceID: "ces:r:SYS.EVS:d1", Name: "disk-01", Type: "evs", Region: "cn-south-1", ProviderRef: "disk-01",
				Labels: map[string]string{"namespace": "SYS.EVS", "dim_name": "disk_name"}},
			{ResourceID: "ces:r:SYS.VPC:v1", Name: "prod-vpc", Type: "vpc", Region: "cn-south-1", ProviderRef: "vpc-1",
				Labels: map[string]string{"namespace": "SYS.VPC", "dim_name": "vpc_id"}},
			{ResourceID: "ces:r:SYS.DCS:c1", Name: "cache-01", Type: "dcs", Region: "cn-south-1", ProviderRef: "dcs-1",
				Labels: map[string]string{"namespace": "SYS.DCS", "dim_name": "dcs_instance_id"}},
			{ResourceID: "ces:r:SYS.DMS:k1", Name: "mq-01", Type: "dms", Region: "cn-south-1", ProviderRef: "kafka-1",
				Labels: map[string]string{"namespace": "SYS.DMS", "dim_name": "dms_instance_id"}},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 4, Discovered: 4, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"evs": {{ResourceID: "evs-vol-1", Type: "evs", ProviderRef: "disk-01",
				Labels: map[string]string{"volume_id": "vol-1", "volume_type": "SSD", "size_gb": "100", "charging_mode": "prepaid"}}},
			"vpc": {{ResourceID: "vpc-1", Type: "vpc", ProviderRef: "vpc-1",
				Labels: map[string]string{"vpc_name": "prod-vpc", "cidr": "192.168.0.0/16", "subnet_count": "3"}}},
			"dcs": {{ResourceID: "dcs-1", Type: "dcs", ProviderRef: "dcs-1",
				Labels: map[string]string{"engine": "Redis", "capacity_gb": "2", "charging_mode": "prepaid"}}},
			// dms 故意不提供，验证未匹配不报错。
		},
		errByType: map[string]error{
			"dms": apperr.New(apperr.CodeUnavailable, "dms api permission denied"),
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-newtypes",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, summary, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) != 4 {
		t.Fatalf("resource count = %d, want 4", len(resources))
	}
	// 阶段二：基础资源落库后调用独立增强。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// EVS 增强 label 合并，CES label 保留。
	if resources[0].Labels["namespace"] != "SYS.EVS" {
		t.Fatalf("EVS CES label overwritten: %+v", resources[0].Labels)
	}
	if resources[0].Labels["volume_type"] != "SSD" {
		t.Fatalf("EVS native label volume_type missing: %+v", resources[0].Labels)
	}
	if resources[0].Labels["charging_mode"] != "prepaid" {
		t.Fatalf("EVS charging_mode missing: %+v", resources[0].Labels)
	}
	// VPC 增强 label 合并。
	if resources[1].Labels["cidr"] != "192.168.0.0/16" {
		t.Fatalf("VPC native label cidr missing: %+v", resources[1].Labels)
	}
	if resources[1].Labels["subnet_count"] != "3" {
		t.Fatalf("VPC subnet_count missing: %+v", resources[1].Labels)
	}
	// DCS 增强 label 合并。
	if resources[2].Labels["engine"] != "Redis" {
		t.Fatalf("DCS native label engine missing: %+v", resources[2].Labels)
	}
	// DMS 增强失败计入 EnrichmentFailedTypes，但 CES 资源仍在。
	if resources[3].Labels["namespace"] != "SYS.DMS" {
		t.Fatalf("DMS CES label should be preserved: %+v", resources[3].Labels)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if enrichResult.Summary.EnrichedCount != 3 {
		t.Fatalf("EnrichedCount = %d, want 3 (evs/vpc/dcs)", enrichResult.Summary.EnrichedCount)
	}
	if len(enrichResult.Summary.EnrichmentFailedTypes) != 1 || enrichResult.Summary.EnrichmentFailedTypes[0] != "dms" {
		t.Fatalf("EnrichmentFailedTypes = %v, want [dms]", enrichResult.Summary.EnrichmentFailedTypes)
	}
	if enrichResult.Summary.EnrichmentFailedCount != 1 {
		t.Fatalf("EnrichmentFailedCount = %d, want 1", enrichResult.Summary.EnrichmentFailedCount)
	}
}

// TestAdapterListAllResourcesHybridEVSRealDimNoMatch 验证使用真实 CES dim 格式时，
// EVS 的 ProviderRef 无法与原生资源对齐，hybrid 增强不命中（见 §21.4）。
// 注意：VPC 子资源（publicip_id/bandwidth_id 等）已通过拆分 cloud_resource_type 修复匹配键，
// 不再属于「不匹配」场景，由 TestAdapterListAllResourcesHybridVPCSubtypeEnrichment 覆盖。
func TestAdapterListAllResourcesHybridEVSRealDimNoMatch(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-realdim"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			// EVS: CES disk_name 真实格式为「服务器ID-盘符」，不是卷显示名称。
			{ResourceID: "ces:r:SYS.EVS:d1", Name: "disk-01", Type: "evs", Region: "cn-south-1", ProviderRef: "6f3a-xxxx-vda",
				Labels: map[string]string{"namespace": "SYS.EVS", "dim_name": "disk_name"}},
			// DCS: 作为对照，匹配键成立。
			{ResourceID: "ces:r:SYS.DCS:c1", Name: "cache-01", Type: "dcs", Region: "cn-south-1", ProviderRef: "dcs-1",
				Labels: map[string]string{"namespace": "SYS.DCS", "dim_name": "dcs_instance_id"}},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 2, Discovered: 2, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			// EVS 原生资源 ProviderRef=卷显示名称，与 CES disk_name 真实格式不一致。
			"evs": {{ResourceID: "evs-vol-1", Type: "evs", ProviderRef: "my-disk-name",
				Labels: map[string]string{"volume_id": "vol-1", "volume_type": "SSD", "size_gb": "100"}}},
			// DCS 原生资源 ProviderRef 与 CES 一致，应命中。
			"dcs": {{ResourceID: "dcs-1", Type: "dcs", ProviderRef: "dcs-1",
				Labels: map[string]string{"engine": "Redis", "capacity_gb": "2"}}},
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-realdim",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, summary, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}
	// 阶段二：基础资源落库后调用独立增强。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// EVS 不应合并原生 label（匹配键不成立）。
	if resources[0].Labels["volume_type"] != "" {
		t.Fatalf("EVS should NOT be enriched with real dim format, got volume_type=%q (labels=%+v)", resources[0].Labels["volume_type"], resources[0].Labels)
	}
	// DCS 作为对照，应命中增强。
	if resources[1].Labels["engine"] != "Redis" {
		t.Fatalf("DCS should be enriched as control case, got engine=%q (labels=%+v)", resources[1].Labels["engine"], resources[1].Labels)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	// 只有 DCS 命中增强。
	if enrichResult.Summary.EnrichedCount != 1 {
		t.Fatalf("EnrichedCount = %d, want 1 (only dcs matches; evs mismatch)", enrichResult.Summary.EnrichedCount)
	}
}

// TestAdapterListAllResourcesHybridEnrichmentFailedSummary 验证多类型增强失败时，
// summary.EnrichmentFailedCount 与 EnrichmentFailedTypes 长度一致且等于失败类型数。
// 契约要求 adapter 必须填充 EnrichmentFailedCount（见 dto.go / sync_support.go），
// 否则 sync_service 的 batch summary 聚合后 enrichment_failed_count 恒为 0，前端数值错误。
func TestAdapterListAllResourcesHybridEnrichmentFailedSummary(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-failed-summary"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			{ResourceID: "ces:r:SYS.ECS:e1", Name: "ecs-01", Type: "ecs", Region: "cn-south-1", ProviderRef: "ecs-01",
				Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id"}},
			{ResourceID: "ces:r:SYS.RDS:r1", Name: "rds-01", Type: "rds", Region: "cn-south-1", ProviderRef: "rds-01",
				Labels: map[string]string{"namespace": "SYS.RDS", "dim_name": "rds_instance_id"}},
			{ResourceID: "ces:r:SYS.DMS:k1", Name: "mq-01", Type: "dms", Region: "cn-south-1", ProviderRef: "kafka-1",
				Labels: map[string]string{"namespace": "SYS.DMS", "dim_name": "dms_instance_id"}},
			{ResourceID: "ces:r:SYS.DCS:c1", Name: "cache-01", Type: "dcs", Region: "cn-south-1", ProviderRef: "dcs-1",
				Labels: map[string]string{"namespace": "SYS.DCS", "dim_name": "dcs_instance_id"}},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 4, Discovered: 4, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			// 仅 DCS 提供原生资源并匹配成功，作为对照。
			"dcs": {{ResourceID: "dcs-1", Type: "dcs", ProviderRef: "dcs-1",
				Labels: map[string]string{"engine": "Redis", "capacity_gb": "2"}}},
		},
		errByType: map[string]error{
			"ecs": apperr.New(apperr.CodeUnavailable, "ecs api permission denied"),
			"rds": apperr.New(apperr.CodeUnavailable, "rds api permission denied"),
			"dms": apperr.New(apperr.CodeUnavailable, "dms api permission denied"),
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-failed-summary",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, summary, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	// 阶段一：CES 资源仍应返回，不被增强失败丢弃。
	if len(resources) != 4 {
		t.Fatalf("resource count = %d, want 4 (CES base not dropped)", len(resources))
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	// 阶段二：基础资源落库后调用独立增强。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// 仅 DCS 命中增强。
	if enrichResult.Summary.EnrichedCount != 1 {
		t.Fatalf("EnrichedCount = %d, want 1 (only dcs succeeds)", enrichResult.Summary.EnrichedCount)
	}
	// 三类增强失败。
	if len(enrichResult.Summary.EnrichmentFailedTypes) != 3 {
		t.Fatalf("EnrichmentFailedTypes = %v, want 3 failed types", enrichResult.Summary.EnrichmentFailedTypes)
	}
	// 契约字段：EnrichmentFailedCount 必须等于失败类型数。
	if enrichResult.Summary.EnrichmentFailedCount != 3 {
		t.Fatalf("EnrichmentFailedCount = %d, want 3", enrichResult.Summary.EnrichmentFailedCount)
	}
	if enrichResult.Summary.EnrichmentFailedCount != len(enrichResult.Summary.EnrichmentFailedTypes) {
		t.Fatalf("EnrichmentFailedCount = %d but len(EnrichmentFailedTypes) = %d, must be equal",
			enrichResult.Summary.EnrichmentFailedCount, len(enrichResult.Summary.EnrichmentFailedTypes))
	}
}

// TestAdapterListAllResourcesHybridVPCSubtypeEnrichment 验证 SYS.VPC 拆分子类型后，
// eip/bandwidth 的 CES ProviderRef 与原生资源 ProviderRef 对齐，hybrid 增强命中（见 §21.4 修复）。
func TestAdapterListAllResourcesHybridVPCSubtypeEnrichment(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-vpc-subtype"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			// EIP: CES publicip_id 维度值 = publicip id，与原生 EIP ProviderRef 对齐。
			{ResourceID: "ces:r:SYS.VPC:e1", Name: "prod-eip", Type: "eip", Region: "cn-south-1", ProviderRef: "pub-uuid-1",
				Labels: map[string]string{"namespace": "SYS.VPC", "dim_name": "publicip_id"}},
			// Bandwidth: CES bandwidth_id 维度值 = bandwidth id。
			{ResourceID: "ces:r:SYS.VPC:b1", Name: "shared-bw", Type: "bandwidth", Region: "cn-south-1", ProviderRef: "bw-uuid-1",
				Labels: map[string]string{"namespace": "SYS.VPC", "dim_name": "bandwidth_id"}},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 2, Discovered: 2, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"eip": {{ResourceID: "eip-pub-uuid-1", Type: "eip", ProviderRef: "pub-uuid-1",
				Labels: map[string]string{"public_ip": "1.2.3.4", "bandwidth_id": "bw-uuid-1"}}},
			"bandwidth": {{ResourceID: "bandwidth-bw-uuid-1", Type: "bandwidth", ProviderRef: "bw-uuid-1",
				Labels: map[string]string{"size_mbps": "100", "share_type": "WHOLE"}}},
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-vpc-subtype",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, summary, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(resources))
	}
	// 阶段二：基础资源落库后调用独立增强。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// EIP 增强 label 合并，CES label 保留。
	if resources[0].Labels["namespace"] != "SYS.VPC" {
		t.Fatalf("EIP CES label overwritten: %+v", resources[0].Labels)
	}
	if resources[0].Labels["public_ip"] != "1.2.3.4" {
		t.Fatalf("EIP native label public_ip missing: %+v", resources[0].Labels)
	}
	// Bandwidth 增强 label 合并。
	if resources[1].Labels["dim_name"] != "bandwidth_id" {
		t.Fatalf("Bandwidth CES label overwritten: %+v", resources[1].Labels)
	}
	if resources[1].Labels["size_mbps"] != "100" {
		t.Fatalf("Bandwidth native label size_mbps missing: %+v", resources[1].Labels)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if enrichResult.Summary.EnrichedCount != 2 {
		t.Fatalf("EnrichedCount = %d, want 2 (eip+bandwidth)", enrichResult.Summary.EnrichedCount)
	}
}

// TestAdapterEnrichResourcesTruncation 验证原生 API 结果超过 maxResources 时：
// - 截断被正确检测并记录到 EnrichmentWarnings（"<type>.truncated"）
// - 被截断后不在前 maxResources 条中的 CES 资源不会被增强
// - 在前 maxResources 条中的 CES 资源仍被正确增强
func TestAdapterEnrichResourcesTruncation(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-hybrid-trunc"
	seedAKSKCredential(t, provider, repo, vault, accountID)
	cesMock := &mockCESDiscovery{result: &CESResourceDiscoveryResult{
		Resources: []domain.CloudResource{
			{ResourceID: "ces:r:SYS.ECS:i-1", Name: "web-01", Type: "ecs", Region: "cn-south-1", ProviderRef: "i-1",
				Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id"}},
			{ResourceID: "ces:r:SYS.ECS:i-2", Name: "web-02", Type: "ecs", Region: "cn-south-1", ProviderRef: "i-2",
				Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id"}},
			// i-3 存在于 CES 资源组，但原生 API 结果被截断后不在前 maxResources 条中。
			{ResourceID: "ces:r:SYS.ECS:i-3", Name: "web-03", Type: "ecs", Region: "cn-south-1", ProviderRef: "i-3",
				Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id"}},
		},
		Summary: CESResourceDiscoverySummary{Region: "cn-south-1", CESTotal: 3, Discovered: 3, ResourceGroupName: "全部资源"},
	}}
	resMock := &mockResourceDiscoveryByType{
		byType: map[string][]domain.CloudResource{
			"ecs": {
				{ResourceID: "ecs-i-1", Type: "ecs", ProviderRef: "i-1",
					Labels: map[string]string{"flavor": "s6.large.2", "az": "cn-south-1a"}},
				{ResourceID: "ecs-i-2", Type: "ecs", ProviderRef: "i-2",
					Labels: map[string]string{"flavor": "s6.medium.2", "az": "cn-south-1b"}},
				{ResourceID: "ecs-i-3", Type: "ecs", ProviderRef: "i-3",
					Labels: map[string]string{"flavor": "s6.small.1", "az": "cn-south-1c"}},
				{ResourceID: "ecs-i-4", Type: "ecs", ProviderRef: "i-4",
					Labels: map[string]string{"flavor": "s6.tiny.1", "az": "cn-south-1d"}},
			},
		},
	}
	adapter := NewAdapter(provider, nil, resMock).WithCESResourceDiscovery(cesMock)
	pctx := domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "p1", Regions: []string{"cn-south-1"},
			CredentialRefID: "ref-trunc",
			ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
		},
	}
	resources, _, err := adapter.ListAllResources(context.Background(), pctx,
		obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 100})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	// 阶段二：MaxResources=2，原生 API 返回 4 条，应截断到 2 条。
	enrichQ := obsapp.AssetFullSyncQuery{AccountID: accountID, Provider: "huawei_cloud", Region: "cn-south-1", MaxResources: 2, Account: &pctx.Account}
	enrichResult, err := adapter.EnrichAllResources(context.Background(), obsapp.Actor{}, enrichQ, resources)
	if err != nil {
		t.Fatalf("EnrichAllResources: %v", err)
	}
	// i-1 和 i-2 在前 2 条中，应被增强。
	if resources[0].Labels["flavor"] != "s6.large.2" {
		t.Fatalf("i-1 flavor = %q, want s6.large.2 (labels=%+v)", resources[0].Labels["flavor"], resources[0].Labels)
	}
	if resources[1].Labels["flavor"] != "s6.medium.2" {
		t.Fatalf("i-2 flavor = %q, want s6.medium.2 (labels=%+v)", resources[1].Labels["flavor"], resources[1].Labels)
	}
	// i-3 被截断，不应被增强。
	if resources[2].Labels["flavor"] != "" {
		t.Fatalf("i-3 should NOT be enriched after truncation, got flavor=%q (labels=%+v)", resources[2].Labels["flavor"], resources[2].Labels)
	}
	// 截断警告应记录到 EnrichmentWarnings。
	found := false
	for _, w := range enrichResult.Summary.EnrichmentWarnings {
		if w == "ecs.truncated" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("EnrichmentWarnings = %v, want contains ecs.truncated", enrichResult.Summary.EnrichmentWarnings)
	}
	// EnrichedCount 应为 2（只有 i-1 和 i-2 被增强）。
	if enrichResult.Summary.EnrichedCount != 2 {
		t.Fatalf("EnrichedCount = %d, want 2", enrichResult.Summary.EnrichedCount)
	}
}
