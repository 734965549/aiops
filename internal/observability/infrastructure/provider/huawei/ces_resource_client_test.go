package huawei

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

// mockCESResourceGroupAPI 模拟 CES v2 资源分组接口。
type mockCESResourceGroupAPI struct {
	listGroupsResp *cesv2model.ListResourceGroupsResponse
	listGroupsErr  error
	listGroupsErrs []error
	showResp       *cesv2model.ShowResourceGroupResponse
	showErr        error
	listResps      map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse // key: service
	listResErr     error

	listGroupsCalls int
	showCalls       int
	listResCalls    int
	listResInputs   []string // 记录请求的 service
}

func (m *mockCESResourceGroupAPI) ListResourceGroups(_ *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error) {
	m.listGroupsCalls++
	if len(m.listGroupsErrs) > 0 {
		err := m.listGroupsErrs[0]
		m.listGroupsErrs = m.listGroupsErrs[1:]
		return nil, err
	}
	if m.listGroupsErr != nil {
		return nil, m.listGroupsErr
	}
	return m.listGroupsResp, nil
}

func (m *mockCESResourceGroupAPI) ShowResourceGroup(_ *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error) {
	m.showCalls++
	if m.showErr != nil {
		return nil, m.showErr
	}
	return m.showResp, nil
}

func (m *mockCESResourceGroupAPI) ListResourceGroupsServicesResources(req *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
	m.listResCalls++
	m.listResInputs = append(m.listResInputs, req.Service)
	if m.listResErr != nil {
		return nil, m.listResErr
	}
	if m.listResps == nil {
		return nil, nil
	}
	return m.listResps[req.Service], nil
}

func int32Ptr(v int32) *int32 { return &v }
func strPtr(v string) *string { return &v }

func buildListGroupsResp(groups ...cesv2model.OneResourceGroupResp) *cesv2model.ListResourceGroupsResponse {
	g := make([]cesv2model.OneResourceGroupResp, len(groups))
	copy(g, groups)
	return &cesv2model.ListResourceGroupsResponse{
		Count:          int32Ptr(int32(len(groups))),
		ResourceGroups: &g,
	}
}

func buildShowResp(name, productNames string, total int) *cesv2model.ShowResourceGroupResponse {
	t := int32(total)
	return &cesv2model.ShowResourceGroupResponse{
		GroupName:    strPtr(name),
		ProductNames: strPtr(productNames),
		ResourceStatistics: &cesv2model.GetResourceGroupRespResourceStatistics{
			Total: &t,
		},
	}
}

func buildListResResp(resources ...cesv2model.GetResourceGroupResources) *cesv2model.ListResourceGroupsServicesResourcesResponse {
	r := make([]cesv2model.GetResourceGroupResources, len(resources))
	copy(r, resources)
	return &cesv2model.ListResourceGroupsServicesResourcesResponse{
		Count:     int32Ptr(int32(len(resources))),
		Resources: &r,
	}
}

func mkRes(name, dimName, dimValue string) cesv2model.GetResourceGroupResources {
	return cesv2model.GetResourceGroupResources{
		Status:       cesv2model.GetGetResourceGroupResourcesStatusEnum().HEALTH,
		ResourceName: strPtr(name),
		Dimensions:   []cesv2model.ResourceDimension{{Name: dimName, Value: dimValue}},
	}
}

func TestDiscoverCESResources_HappyPath(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}},
			cesv2model.OneResourceGroupResp{GroupName: "other", GroupId: "rg002", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
		showResp: buildShowResp("全部资源", "SYS.ECS,instance_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1"), mkRes("ecs-2", "instance_id", "i-2")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", ResourceGroupName: "全部资源", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("resource count = %d, want 2", len(result.Resources))
	}
	if result.Summary.CESTotal != 2 {
		t.Fatalf("CESTotal = %d, want 2", result.Summary.CESTotal)
	}
	if result.Summary.Discovered != 2 {
		t.Fatalf("Discovered = %d, want 2", result.Summary.Discovered)
	}
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"ecs"}) {
		t.Fatalf("SuccessfulTypes = %v, want [ecs]", result.Summary.SuccessfulTypes)
	}
	if result.Summary.ResourceGroupID != "rg001" {
		t.Fatalf("ResourceGroupID = %q, want rg001", result.Summary.ResourceGroupID)
	}
	if result.Summary.ResourceGroupSelection != "specified_name" {
		t.Fatalf("ResourceGroupSelection = %q, want specified_name", result.Summary.ResourceGroupSelection)
	}
	if result.Summary.ProductNamesEmpty {
		t.Fatalf("ProductNamesEmpty should be false")
	}
	want0 := domain.CloudResource{
		ResourceID: "ces:cn-south-1:SYS.ECS:i-1", Name: "ecs-1", Type: "ecs",
		Region: "cn-south-1", Status: "health", ProviderRef: "i-1",
		Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id", "resource_group_id": "rg001", "resource_group_name": "全部资源"},
	}
	if !reflect.DeepEqual(result.Resources[0], want0) {
		t.Fatalf("resource[0] = %+v, want %+v", result.Resources[0], want0)
	}
}

func TestDiscoverCESResources_SpecifiedGroupID(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		showResp: buildShowResp("my-group", "SYS.EVS,disk_name", 1),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.EVS": buildListResResp(mkRes("disk-1", "disk_name", "d-1")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", ResourceGroupID: "rg999", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Summary.ResourceGroupID != "rg999" {
		t.Fatalf("ResourceGroupID = %q, want rg999", result.Summary.ResourceGroupID)
	}
	if result.Resources[0].Type != "evs" {
		t.Fatalf("Type = %q, want evs", result.Resources[0].Type)
	}
	if api.listGroupsResp != nil {
		t.Fatalf("ListResourceGroups should not be called when GroupID specified")
	}
}

func TestDiscoverCESResources_EmptyProductNamesFallsBack(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}}),
		showResp:       buildShowResp("全部资源", "", 1),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Summary.ProductNamesEmpty {
		t.Fatalf("ProductNamesEmpty should be true")
	}
	if len(result.Resources) == 0 {
		t.Fatalf("expected fallback resources")
	}
}

func TestDiscoverCESResources_NoGroupFound(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(),
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound", apperr.CodeOf(err))
	}
}

func TestDiscoverCESResources_CustomGroupNameNotFoundFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "prod-group", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}},
			cesv2model.OneResourceGroupResp{GroupName: "dev-group", GroupId: "rg002", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
	}
	// 自定义名称拼写错误（"prod-gruop"），默认候选名也不命中，应直接失败而非回退最大资源组。
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", ResourceGroupName: "prod-gruop", MaxResources: 100,
	})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound for custom name mismatch", apperr.CodeOf(err))
	}
}

func TestDiscoverCESResources_DefaultNameNotMatchedFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "prod-group", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(5)}},
			cesv2model.OneResourceGroupResp{GroupName: "dev-group", GroupId: "rg002", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}},
		),
		showResp: buildShowResp("prod-group", "SYS.ECS,instance_id", 5),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
		},
	}
	// 默认候选名"全部资源"未命中时（用户未预先创建同名分组），应直接失败，
	// 不回退到 total 最大的资源组，避免把业务组误当作全量，见 §8.4。
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", ResourceGroupName: "全部资源", MaxResources: 100,
	})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound for default name not matched", apperr.CodeOf(err))
	}
}

func TestDiscoverCESResources_PartialFailureDoesNotAbort(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id;SYS.NEW_SERVICE,res_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS":         buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
			"SYS.NEW_SERVICE": buildListResResp(mkRes("new-1", "res_id", "v-1")),
		},
	}
	// SYS.ECS 列表调用失败，SYS.NEW_SERVICE 成功（未知 namespace 计数 +1）。
	api2 := &failingServiceAPI{inner: api, failService: "SYS.ECS", err: apperr.New(apperr.CodeUnavailable, "ecs failed")}
	result, err := discoverCESResources(context.Background(), api2, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (partial success)", len(result.Resources))
	}
	if result.Resources[0].Type != "new_service" {
		t.Fatalf("Type = %q, want new_service", result.Resources[0].Type)
	}
	if len(result.Summary.FailedScopes) != 1 {
		t.Fatalf("FailedScopes = %v, want 1", result.Summary.FailedScopes)
	}
	if result.Summary.UnknownNamespaceCount != 1 {
		t.Fatalf("UnknownNamespaceCount = %d, want 1", result.Summary.UnknownNamespaceCount)
	}
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"new_service"}) {
		t.Fatalf("SuccessfulTypes = %v, want [new_service]", result.Summary.SuccessfulTypes)
	}
}

// failingServiceAPI 包装 mock，对指定 service 的资源列表调用返回错误。
type failingServiceAPI struct {
	inner       *mockCESResourceGroupAPI
	failService string
	err         error
}

func (f *failingServiceAPI) ListResourceGroups(req *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error) {
	return f.inner.ListResourceGroups(req)
}
func (f *failingServiceAPI) ShowResourceGroup(req *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error) {
	return f.inner.ShowResourceGroup(req)
}
func (f *failingServiceAPI) ListResourceGroupsServicesResources(req *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
	if req.Service == f.failService {
		return nil, f.err
	}
	return f.inner.ListResourceGroupsServicesResources(req)
}

// failingDimAPI 包装 mock，对指定 service+dim_name 的资源列表调用返回错误，
// 用于验证同一资源类型的多个 scope 部分失败时该类型不得进入 SuccessfulTypes，见 §13.1。
type failingDimAPI struct {
	inner   *mockCESResourceGroupAPI
	failSvc string
	failDim string
	err     error
}

func (f *failingDimAPI) ListResourceGroups(req *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error) {
	return f.inner.ListResourceGroups(req)
}
func (f *failingDimAPI) ShowResourceGroup(req *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error) {
	return f.inner.ShowResourceGroup(req)
}
func (f *failingDimAPI) ListResourceGroupsServicesResources(req *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
	if req.Service == f.failSvc && req.DimName != nil && *req.DimName == f.failDim {
		return nil, f.err
	}
	return f.inner.ListResourceGroupsServicesResources(req)
}

// TestDiscoverCESResources_SameTypePartialScopeFailure 验证同一资源类型的多个 scope 部分失败时，
// 该类型不得进入 SuccessfulTypes（否则 sync_service 会把未查询到的资产误标为 stale），见 §13.1。
// 例如 SYS.ELB/loadbalancer_id 成功但 SYS.ELB/l7policy_id 失败时，elb 必须从 SuccessfulTypes 剔除。
func TestDiscoverCESResources_SameTypePartialScopeFailure(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.ELB,loadbalancer_id;SYS.ELB,l7policy_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ELB": buildListResResp(mkRes("elb-1", "loadbalancer_id", "lb-1")),
		},
	}
	// loadbalancer_id 成功，l7policy_id 失败：两者都映射到 elb，elb 不得计入 SuccessfulTypes。
	failAPI := &failingDimAPI{inner: api, failSvc: "SYS.ELB", failDim: "l7policy_id", err: apperr.New(apperr.CodeUnavailable, "l7policy failed")}
	result, err := discoverCESResources(context.Background(), failAPI, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (loadbalancer success)", len(result.Resources))
	}
	if len(result.Summary.FailedScopes) != 1 {
		t.Fatalf("FailedScopes = %v, want 1", result.Summary.FailedScopes)
	}
	if !reflect.DeepEqual(result.Summary.QueryFailedTypes, []string{"elb"}) {
		t.Fatalf("QueryFailedTypes = %v, want [elb]", result.Summary.QueryFailedTypes)
	}
	if len(result.Summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (elb has a failed scope, must not be marked successful)", result.Summary.SuccessfulTypes)
	}
}

func TestDiscoverCESResources_MaxResourcesCap(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(10)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 10),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(
				mkRes("ecs-1", "instance_id", "i-1"), mkRes("ecs-2", "instance_id", "i-2"), mkRes("ecs-3", "instance_id", "i-3"),
			),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 2,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("resource count = %d, want 2 (capped)", len(result.Resources))
	}
	// 达到上限时必须标记 MaxResourcesReached，且被截断的类型不计入 SuccessfulTypes，
	// 否则 sync_service 会把未扫描到的资产误标为 stale，见 §13。
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true")
	}
	if len(result.Summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (truncated type must not be marked successful)", result.Summary.SuccessfulTypes)
	}
}

// pagedResourcesAPI 按 offset/limit 切片返回某 service 的资源列表，模拟服务端分页，
// 用于验证 listResourcesForProduct 多页拉取（ListResourceGroupsServicesResources 翻页），见 §8.6。
type pagedResourcesAPI struct {
	resources []cesv2model.GetResourceGroupResources
	showResp  *cesv2model.ShowResourceGroupResponse
	calls     int
}

func (p *pagedResourcesAPI) ListResourceGroups(_ *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error) {
	total := int32(len(p.resources))
	groups := []cesv2model.OneResourceGroupResp{{
		GroupName: "全部资源", GroupId: "rg001",
		ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: &total},
	}}
	return &cesv2model.ListResourceGroupsResponse{Count: &total, ResourceGroups: &groups}, nil
}

func (p *pagedResourcesAPI) ShowResourceGroup(_ *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error) {
	if p.showResp != nil {
		return p.showResp, nil
	}
	return nil, apperr.New(apperr.CodeNotFound, "not found")
}

func (p *pagedResourcesAPI) ListResourceGroupsServicesResources(req *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
	p.calls++
	// 测试场景下 listResourcesForProduct 固定以 defaultCESPageLimit(100) 翻页，无需解析 *string 类型的 Limit。
	limit := int32(defaultCESPageLimit)
	offset := int32(0)
	if req != nil && req.Offset != nil {
		offset = *req.Offset
	}
	total := int32(len(p.resources))
	if offset >= total {
		empty := make([]cesv2model.GetResourceGroupResources, 0)
		return &cesv2model.ListResourceGroupsServicesResourcesResponse{Count: &total, Resources: &empty}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := make([]cesv2model.GetResourceGroupResources, end-offset)
	copy(page, p.resources[offset:end])
	return &cesv2model.ListResourceGroupsServicesResourcesResponse{Count: &total, Resources: &page}, nil
}

// TestDiscoverCESResources_ResourceListMultiPagePagination 验证单个 service 资源数超过单页(100)时，
// listResourcesForProduct 按 offset 翻页拉取全部资源（offset 0/100/200），见 docs/huawei-ces-asset-sync-plan.md §8.6。
func TestDiscoverCESResources_ResourceListMultiPagePagination(t *testing.T) {
	const total = 250
	resources := make([]cesv2model.GetResourceGroupResources, total)
	for i := range resources {
		resources[i] = mkRes(fmt.Sprintf("ecs-%d", i), "instance_id", fmt.Sprintf("i-%d", i))
	}
	api := &pagedResourcesAPI{
		resources: resources,
		showResp:  buildShowResp("全部资源", "SYS.ECS,instance_id", total),
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != total {
		t.Fatalf("resource count = %d, want %d (multi-page collected)", len(result.Resources), total)
	}
	if result.Summary.Discovered != total {
		t.Fatalf("Discovered = %d, want %d", result.Summary.Discovered, total)
	}
	// 单页 100，250 个资源应触发 3 次列表调用（offset 0/100/200），末页返回 50<100 停止。
	if api.calls != 3 {
		t.Fatalf("ListResourceGroupsServicesResources calls = %d, want 3", api.calls)
	}
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"ecs"}) {
		t.Fatalf("SuccessfulTypes = %v, want [ecs]", result.Summary.SuccessfulTypes)
	}
	if len(result.Summary.FailedScopes) != 0 {
		t.Fatalf("FailedScopes = %v, want empty", result.Summary.FailedScopes)
	}
	if result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = true, want false")
	}
	// 校验首尾资源顺序，确认翻页完整拼接而非重复首页。
	if got := result.Resources[0].ProviderRef; got != "i-0" {
		t.Fatalf("first resource ProviderRef = %q, want i-0", got)
	}
	if got := result.Resources[total-1].ProviderRef; got != fmt.Sprintf("i-%d", total-1) {
		t.Fatalf("last resource ProviderRef = %q, want i-%d", got, total-1)
	}
}

func TestDiscoverCESResources_InvalidResourceDropped(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(
				mkRes("valid", "instance_id", "i-1"),
				cesv2model.GetResourceGroupResources{Status: cesv2model.GetGetResourceGroupResourcesStatusEnum().HEALTH, Dimensions: nil}, // 无维度无名称
			),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(result.Resources))
	}
	if result.Summary.InvalidResourceCount != 1 {
		t.Fatalf("InvalidResourceCount = %d, want 1", result.Summary.InvalidResourceCount)
	}
	if !reflect.DeepEqual(result.Summary.ConversionFailedTypes, []string{"ecs"}) {
		t.Fatalf("ConversionFailedTypes = %v, want [ecs]", result.Summary.ConversionFailedTypes)
	}
}

func TestValidateCESDiscoveryRequest(t *testing.T) {
	validCred := AKSKCredential{AccessKey: "ak", SecretKey: "sk"}
	cases := []struct {
		name string
		req  CESResourceDiscoveryRequest
		cred AKSKCredential
		want apperr.Code
	}{
		{"missing project", CESResourceDiscoveryRequest{Region: "r"}, validCred, apperr.CodeInvalidArgument},
		{"missing region", CESResourceDiscoveryRequest{ProjectID: "p"}, validCred, apperr.CodeInvalidArgument},
		{"missing cred", CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"}, AKSKCredential{}, apperr.CodeFailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCESDiscoveryRequest(tc.req, tc.cred)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				return
			}
			if apperr.CodeOf(err) != tc.want {
				t.Fatalf("err code = %v, want %v", apperr.CodeOf(err), tc.want)
			}
		})
	}
	// 合法请求应无错。
	if err := validateCESDiscoveryRequest(CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"}, validCred); err != nil {
		t.Fatalf("valid request err: %v", err)
	}
}

// pagedGroupsAPI 按 offset/limit 切片返回资源组，模拟服务端分页。
type pagedGroupsAPI struct {
	groups   []cesv2model.OneResourceGroupResp
	showResp *cesv2model.ShowResourceGroupResponse
	calls    int
}

func (p *pagedGroupsAPI) ListResourceGroups(req *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error) {
	p.calls++
	limit := int32(defaultCESPageLimit)
	if req != nil && req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}
	offset := int32(0)
	if req != nil && req.Offset != nil {
		offset = *req.Offset
	}
	total := int32(len(p.groups))
	if offset >= total {
		empty := make([]cesv2model.OneResourceGroupResp, 0)
		return &cesv2model.ListResourceGroupsResponse{Count: &total, ResourceGroups: &empty}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := make([]cesv2model.OneResourceGroupResp, end-offset)
	copy(page, p.groups[offset:end])
	return &cesv2model.ListResourceGroupsResponse{Count: &total, ResourceGroups: &page}, nil
}

func (p *pagedGroupsAPI) ShowResourceGroup(_ *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error) {
	if p.showResp != nil {
		return p.showResp, nil
	}
	return nil, apperr.New(apperr.CodeNotFound, "not found")
}

func (p *pagedGroupsAPI) ListResourceGroupsServicesResources(_ *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
	return nil, nil
}

// 250 个资源组、默认候选名"全部资源"位于第二页之后，验证分页拉取后能命中它。
func TestSelectResourceGroup_PaginationCollectsAllGroups(t *testing.T) {
	groups := make([]cesv2model.OneResourceGroupResp, 250)
	for i := range groups {
		total := int32(i%10 + 1)
		groups[i] = cesv2model.OneResourceGroupResp{
			GroupName:          fmt.Sprintf("group-%d", i),
			GroupId:            fmt.Sprintf("rg%03d", i),
			ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: &total},
		}
	}
	// 将默认候选名"全部资源"放在第 150 个（跨越分页第二页之后），验证分页拉取后仍能命中。
	groups[150].GroupName = "全部资源"
	bigTotal := int32(9999)
	groups[150].ResourceStatistics = &cesv2model.OneResourceGroupRespResourceStatistics{Total: &bigTotal}

	api := &pagedGroupsAPI{groups: groups}
	sel, err := selectResourceGroup(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel.GroupID != "rg150" {
		t.Fatalf("GroupID = %q, want rg150 (全部资源 across all pages)", sel.GroupID)
	}
	if sel.Total != 9999 {
		t.Fatalf("Total = %d, want 9999", sel.Total)
	}
	if api.calls < 3 {
		t.Fatalf("ListResourceGroups calls = %d, want >=3 (paginated)", api.calls)
	}
}

// 没有"全部资源"，但有"All Resources"，应命中英文名兜底。
func TestSelectResourceGroup_FallsBackToEnglishName(t *testing.T) {
	total := int32(5)
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "All Resources", GroupId: "rg-en", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: &total}},
		{GroupName: "small", GroupId: "rg-sm", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	sel, err := selectResourceGroup(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel.GroupID != "rg-en" {
		t.Fatalf("GroupID = %q, want rg-en (english name fallback)", sel.GroupID)
	}
}

// 没有任何候选名命中，应直接失败，不回退到 total 最大的组。
func TestSelectResourceGroup_NoMatchFails(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "team-a", GroupId: "rg1", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}},
		{GroupName: "team-b", GroupId: "rg2", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(7)}},
		{GroupName: "team-c", GroupId: "rg3", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	_, err := selectResourceGroup(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound for no match", apperr.CodeOf(err))
	}
}

func TestSelectResourceGroup_RetriesRateLimit(t *testing.T) {
	oldDelays := cesRetryDelays
	cesRetryDelays = []time.Duration{time.Millisecond}
	defer func() { cesRetryDelays = oldDelays }()

	api := &mockCESResourceGroupAPI{
		listGroupsErrs: []error{&sdkerr.ServiceResponseError{StatusCode: 429}},
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{
			GroupName: "全部资源", GroupId: "rg-retry", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)},
		}),
	}
	sel, err := selectResourceGroup(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel.GroupID != "rg-retry" || api.listGroupsCalls != 2 {
		t.Fatalf("selection=%+v calls=%d, want retry success on second call", sel, api.listGroupsCalls)
	}
}

// 用户指定名优先于默认候选名。
func TestSelectResourceGroup_SpecifiedNamePriority(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "全部资源", GroupId: "rg-default", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(100)}},
		{GroupName: "my-custom", GroupId: "rg-custom", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	sel, err := selectResourceGroup(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r", ResourceGroupName: "my-custom"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel.GroupID != "rg-custom" {
		t.Fatalf("GroupID = %q, want rg-custom (specified name priority)", sel.GroupID)
	}
}

func TestResourceGroupNameCandidates_Dedup(t *testing.T) {
	// 默认候选名中 "All resources" 与 "All Resources" 大小写不同但 EqualFold 等价，按小写去重。
	got := resourceGroupNameCandidates("")
	want := []string{"全部资源", "All resources"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	// 用户指定名优先，且与默认英文名大小写不同时按小写去重，中文默认名仍保留。
	got2 := resourceGroupNameCandidates("ALL RESOURCES")
	want2 := []string{"ALL RESOURCES", "全部资源"}
	if !reflect.DeepEqual(got2, want2) {
		t.Fatalf("candidates = %v, want %v", got2, want2)
	}
}
