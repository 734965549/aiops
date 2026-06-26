package huawei

import (
	"context"
	"reflect"
	"testing"

	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

// mockCESResourceGroupAPI 模拟 CES v2 资源分组接口。
type mockCESResourceGroupAPI struct {
	listGroupsResp *cesv2model.ListResourceGroupsResponse
	listGroupsErr  error
	showResp       *cesv2model.ShowResourceGroupResponse
	showErr        error
	listResps      map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse // key: service
	listResErr     error

	showCalls     int
	listResCalls  int
	listResInputs []string // 记录请求的 service
}

func (m *mockCESResourceGroupAPI) ListResourceGroups(_ *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error) {
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
