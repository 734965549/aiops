package huawei

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	obsapp "github.com/734965549/aiops/internal/observability/application"
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

	// showRespsByGroup 按 group_id 返回不同的 ShowResourceGroup 响应，
	// 用于模拟聚合口径下各资源组 product_names 不同的情况，见 §8.4。
	showRespsByGroup map[string]*cesv2model.ShowResourceGroupResponse
	// listRespsByGroup 按 group_id 返回不同的资源列表响应，key 为 service。
	listRespsByGroup map[string]map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse
	// showErrByGroup 按 group_id 返回错误，用于模拟单个资源组解析失败。
	showErrByGroup map[string]error
	// showCallsByGroup 记录每个 group_id 的 ShowResourceGroup 调用次数。
	showCallsByGroup map[string]int
	// listResGroups 记录 ListResourceGroupsServicesResources 被请求的 group_id 顺序。
	listResGroups []string

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

func (m *mockCESResourceGroupAPI) ShowResourceGroup(req *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error) {
	m.showCalls++
	groupID := ""
	if req != nil {
		groupID = req.GroupId
	}
	if m.showCallsByGroup == nil {
		m.showCallsByGroup = map[string]int{}
	}
	m.showCallsByGroup[groupID]++
	if err, ok := m.showErrByGroup[groupID]; ok && err != nil {
		return nil, err
	}
	if resp, ok := m.showRespsByGroup[groupID]; ok {
		return resp, nil
	}
	if m.showErr != nil {
		return nil, m.showErr
	}
	return m.showResp, nil
}

func (m *mockCESResourceGroupAPI) ListResourceGroupsServicesResources(req *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
	m.listResCalls++
	m.listResInputs = append(m.listResInputs, req.Service)
	groupID := ""
	if req != nil {
		groupID = req.GroupId
	}
	m.listResGroups = append(m.listResGroups, groupID)
	if m.listResErr != nil {
		return nil, m.listResErr
	}
	resps := m.listResps
	if byGroup, ok := m.listRespsByGroup[groupID]; ok {
		resps = byGroup
	}
	if resps == nil {
		return nil, nil
	}
	resp := resps[req.Service]
	if resp == nil {
		return nil, nil
	}
	limit := 0
	if req.Limit != nil {
		if parsed, err := strconv.Atoi(*req.Limit); err == nil {
			limit = parsed
		}
	}
	offset := 0
	if req.Offset != nil {
		offset = int(*req.Offset)
	}
	if offset >= len(*resp.Resources) {
		return &cesv2model.ListResourceGroupsServicesResourcesResponse{Count: resp.Count, Resources: &[]cesv2model.GetResourceGroupResources{}}, nil
	}
	end := len(*resp.Resources)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	slice := make([]cesv2model.GetResourceGroupResources, end-offset)
	copy(slice, (*resp.Resources)[offset:end])
	count := resp.Count
	return &cesv2model.ListResourceGroupsServicesResourcesResponse{Count: count, Resources: &slice}, nil
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
	return buildShowRespWithLevel(name, productNames, total, "product")
}

func buildShowRespWithLevel(name, productNames string, total int, level string) *cesv2model.ShowResourceGroupResponse {
	t := int32(total)
	resp := &cesv2model.ShowResourceGroupResponse{
		GroupName:    strPtr(name),
		ProductNames: strPtr(productNames),
		ResourceStatistics: &cesv2model.GetResourceGroupRespResourceStatistics{
			Total: &t,
		},
	}
	if level != "" {
		lvl := cesv2model.GetShowResourceGroupResponseResourceLevelEnum()
		switch level {
		case "product":
			resp.ResourceLevel = &lvl.PRODUCT
		case "dimension":
			resp.ResourceLevel = &lvl.DIMENSION
		default:
			// 对于未知层级，构造一个自定义值
			custom := cesv2model.ShowResourceGroupResponseResourceLevel{}
			resp.ResourceLevel = &custom
		}
	}
	return resp
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
	// "全部资源"是默认候选名，表达的是"全量"意图而非某个具体分组，
	// 因此命中后口径记为 default_name（与 ParseSyncModeConfig 的 placeholder 语义一致）。
	if result.Summary.ResourceGroupSelection != resourceGroupModeDefaultName {
		t.Fatalf("ResourceGroupSelection = %q, want %q", result.Summary.ResourceGroupSelection, resourceGroupModeDefaultName)
	}
	if result.Summary.ProductNamesEmpty {
		t.Fatalf("ProductNamesEmpty should be false")
	}
	want0 := domain.CloudResource{
		ResourceID: "ces:cn-south-1:SYS.ECS:i-1", Name: "ecs-1", Type: "ecs",
		Region: "cn-south-1", Status: "health", ProviderRef: "i-1",
		Labels: map[string]string{"namespace": "SYS.ECS", "dim_name": "instance_id", "resource_group_id": "rg001", "resource_group_name": "全部资源", "ces_status": "health"},
	}
	if !reflect.DeepEqual(result.Resources[0], want0) {
		t.Fatalf("resource[0] = %+v, want %+v", result.Resources[0], want0)
	}
}

// TestDiscoverCESResources_ZeroResourcesTypeRecordedSuccessful 验证查询成功但本轮返回 0 资源时，
// 该类型仍应计入 SuccessfulTypes，使旧资产可被标记 stale，见 ops/huawei-ces-sync-contract.md §13.1
// 与 ops/huawei-ces-sync-contract.md §13.1 "查询成功且本轮 0 资源的类型 → 旧资产 stale"。
// 修复前 ces_resource_client.go 在 len(pageResources)==0 时直接 continue，导致该类型漏记，
// sync_service 无法对其执行 stale 标记，与 native 路径 (adapter.go) 行为不一致。
func TestDiscoverCESResources_ZeroResourcesTypeRecordedSuccessful(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(0)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 0),
		// 查询成功但返回 0 条资源：Count=0、Resources 为空切片。
		// listResourcesForProduct 检测到 rawCount==0 && pageCount==0 时置 remoteExhausted=true 并正常返回（无 error、未截断）。
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", ResourceGroupName: "全部资源", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 0 {
		t.Fatalf("resource count = %d, want 0 (query succeeded but cloud has no resources)", len(result.Resources))
	}
	if result.Summary.Discovered != 0 {
		t.Fatalf("Discovered = %d, want 0", result.Summary.Discovered)
	}
	if result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = true, want false (no truncation when remote exhausted with 0 resources)")
	}
	if len(result.Summary.QueryFailedTypes) != 0 {
		t.Fatalf("QueryFailedTypes = %v, want empty (query succeeded)", result.Summary.QueryFailedTypes)
	}
	// 核心断言：0 资源但查询成功的类型必须进入 SuccessfulTypes，否则旧资产无法被标记 stale。
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"ecs"}) {
		t.Fatalf("SuccessfulTypes = %v, want [ecs] (zero-resource successful query must be recorded for stale gating)", result.Summary.SuccessfulTypes)
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
	if result.Summary.ResourceLevel != "product" {
		t.Fatalf("ResourceLevel = %q, want product", result.Summary.ResourceLevel)
	}
	if len(result.Resources) == 0 {
		t.Fatalf("expected fallback resources")
	}
}

// TestDiscoverCESResources_DimensionLevelFails 验证 resource_level=dimension 的资源组
// 在 P0 阶段直接返回 FAILED_PRECONDITION，不静默回退。见 §8.5/§13.1。
func TestDiscoverCESResources_DimensionLevelFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowRespWithLevel("全部资源", "SYS.ECS,instance_id", 2, "dimension"),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
		},
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err == nil {
		t.Fatalf("expected FAILED_PRECONDITION error for dimension level")
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("err code = %v, want FAILED_PRECONDITION", apperr.CodeOf(err))
	}
}

// TestDiscoverCESResources_EmptyLevelFails 验证 resource_level 为空（API 未返回该字段）时
// 直接返回 FAILED_PRECONDITION，不静默回退。见 §8.5/§13.1。
func TestDiscoverCESResources_EmptyLevelFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowRespWithLevel("全部资源", "SYS.ECS,instance_id", 2, ""),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
		},
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err == nil {
		t.Fatalf("expected FAILED_PRECONDITION error for empty resource_level")
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("err code = %v, want FAILED_PRECONDITION", apperr.CodeOf(err))
	}
}

// TestDiscoverCESResources_UnknownLevelFails 验证 resource_level 为未知值时
// 直接返回 FAILED_PRECONDITION，不静默回退。见 §8.5/§13.1。
func TestDiscoverCESResources_UnknownLevelFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowRespWithLevel("全部资源", "SYS.ECS,instance_id", 2, "unknown"),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
		},
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err == nil {
		t.Fatalf("expected FAILED_PRECONDITION error for unknown resource_level")
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("err code = %v, want FAILED_PRECONDITION", apperr.CodeOf(err))
	}
}

// TestDiscoverCESResources_DimensionWithProductNamesFails 验证不一致响应：
// resource_level=dimension 但 product_names 非空时仍应返回 FAILED_PRECONDITION。
// dimension 级资源组的 product_names 语义不同于 product 级，不能复用产品级反向 stale 逻辑。
func TestDiscoverCESResources_DimensionWithProductNamesFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowRespWithLevel("全部资源", "SYS.ECS,instance_id;SYS.EVS,disk_name", 2, "dimension"),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
		},
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err == nil {
		t.Fatalf("expected FAILED_PRECONDITION error for dimension level with non-empty product_names")
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("err code = %v, want FAILED_PRECONDITION", apperr.CodeOf(err))
	}
}

// TestDiscoverCESResources_ProductLevelPropagated 验证 product 级资源组的
// ResourceLevel 被正确传递到 CESResourceDiscoverySummary。
func TestDiscoverCESResources_ProductLevelPropagated(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 2),
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
	if result.Summary.ResourceLevel != "product" {
		t.Fatalf("ResourceLevel = %q, want product", result.Summary.ResourceLevel)
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

// 默认候选名"全部资源"未命中时（用户未预先创建同名分组），进入聚合口径：
// 按该 project/region 下全部资源组的并集发现资源，而不是直接失败。
// 这是真实生产账号的常见形态——资源按业务线自建分组（生产ecs/EVS生产/ELB生产/...），
// 没有任何一个组叫"全部资源"，硬失败会让全量同步完全不可用。
// 见 ops/huawei-ces-sync-contract.md §8.4 step 4。
func TestDiscoverCESResources_DefaultNameNotMatchedAggregatesAllGroups(t *testing.T) {
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
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", ResourceGroupName: "全部资源", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Summary.ResourceGroupSelection != resourceGroupModeAllGroups {
		t.Fatalf("ResourceGroupSelection = %q, want %q", result.Summary.ResourceGroupSelection, resourceGroupModeAllGroups)
	}
	if result.Summary.ResourceGroupCount != 2 {
		t.Fatalf("ResourceGroupCount = %d, want 2 (all groups aggregated)", result.Summary.ResourceGroupCount)
	}
	// 聚合口径没有单一 group_id，名称固定为"全部资源"；具体覆盖的组由 count 表达。
	if result.Summary.ResourceGroupName != aggregateResourceGroupName {
		t.Fatalf("ResourceGroupName = %q, want %q", result.Summary.ResourceGroupName, aggregateResourceGroupName)
	}
	if result.Summary.ResourceGroupID != "" {
		t.Fatalf("ResourceGroupID = %q, want empty in aggregate mode", result.Summary.ResourceGroupID)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (union of both groups)", len(result.Resources))
	}
	// 两个组都成功解析 → scope 完整 → resource_level=product，允许反向 stale。
	if result.Summary.ScopeIncomplete {
		t.Fatalf("ScopeIncomplete should be false when all groups resolve")
	}
	if result.Summary.ResourceLevel != resourceLevelProduct {
		t.Fatalf("ResourceLevel = %q, want %q", result.Summary.ResourceLevel, resourceLevelProduct)
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

// TestDiscoverCESResources_MaxResourcesCap 验证单产品资源数超过 max_resources 上限时
// 必须标记 MaxResourcesReached=true 且被截断类型不计入 SuccessfulTypes，
// 避免该类型旧资产被误标 stale，见 ops/huawei-ces-sync-contract.md §13.1。
// 修复前 listResourcesForProduct 的 truncated 恒为 false，最后一个产品自身超限会被
// 误判为完整扫描（remoteExhausted=true 且无后续产品），导致 MaxResourcesReached 漏标。
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
	// 单产品自身超过上限：远端仍有第 3 条未取，必须标记截断，禁止 stale。
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (single product exceeds max_resources, remote has more)")
	}
	if len(result.Summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (truncated ecs must not be marked successful)", result.Summary.SuccessfulTypes)
	}
}

// TestDiscoverCESResources_MaxResourcesExactMatchNotTruncated 验证资源数恰好等于上限时
// 不应误报截断，否则该类型会被剔除出 SuccessfulTypes 而跳过 stale 标记。
func TestDiscoverCESResources_MaxResourcesExactMatchNotTruncated(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(
				mkRes("ecs-1", "instance_id", "i-1"), mkRes("ecs-2", "instance_id", "i-2"),
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
		t.Fatalf("resource count = %d, want 2", len(result.Resources))
	}
	if result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = true, want false (exact match is not truncation)")
	}
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"ecs"}) {
		t.Fatalf("SuccessfulTypes = %v, want [ecs]", result.Summary.SuccessfulTypes)
	}
}

// TestDiscoverCESResources_MaxResourcesTruncatedWhenMoreProductsRemain 验证达到上限且后面还有未扫描的
// product 时，即使当前 product 已拉完，仍应标记 MaxResourcesReached 并禁止 stale。
func TestDiscoverCESResources_MaxResourcesTruncatedWhenMoreProductsRemain(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id;SYS.EVS,disk_name", 3),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
			"SYS.EVS": buildListResResp(mkRes("disk-1", "disk_name", "d-1")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 1,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(result.Resources))
	}
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (more products remain un-scanned)")
	}
	if len(result.Summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (truncated)", result.Summary.SuccessfulTypes)
	}
}

// TestDiscoverCESResources_SingleProductExceedsMaxResources 验证当“最后一个（且唯一一个）
// 产品自身资源数超过 max_resources”时必须正确标记截断，见 ops/huawei-ces-sync-contract.md
// §13.1（例：max_resources=1、云端 2 台 ECS → MaxResourcesReached=true）。
// 修复前 listResourcesForProduct 的 truncated 恒为 false，且此场景既无后续产品
// （i<len-1=false）又因翻页翻完 remoteExhausted=true，三条件全 false 导致 MaxResourcesReached
// 漏标，ecs 被错误计入 SuccessfulTypes，sync_service 进而把未扫到的旧 ECS 误标 stale。
func TestDiscoverCESResources_SingleProductExceedsMaxResources(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(
				mkRes("ecs-1", "instance_id", "i-1"), mkRes("ecs-2", "instance_id", "i-2"),
			),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 1,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 只取 1 台，另一台因上限未取。
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (capped at max_resources)", len(result.Resources))
	}
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (single product exceeds max_resources, remote has more)")
	}
	// 被截断的 ecs 不计入 SuccessfulTypes，避免另一台旧 ECS 被误标 stale。
	if len(result.Summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (truncated ecs must not be marked successful)", result.Summary.SuccessfulTypes)
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
	// 测试场景下 listResourcesForProduct 固定以 DefaultCESPageLimit(100) 翻页，无需解析 *string 类型的 Limit。
	limit := int32(obsapp.DefaultCESPageLimit)
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
// listResourcesForProduct 按 offset 翻页拉取全部资源（offset 0/100/200），见 ops/huawei-ces-sync-contract.md §8.6。
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

// TestDiscoverCESResources_DuplicateDedupByUniqueKey 验证跨 scope 按唯一键
// (cloud_resource_type, cloud_resource_id, region) 去重，满足契约公式
// mapped = unique + duplicate、raw = mapped + invalid，
// 见 ops/huawei-ces-sync-contract.md §9.5、§9.4。
// 场景：SYS.RDS 与 SYS.RDS_MYSQL_CLUSTER 均以 rds_cluster_id 为主维度，
// 同一 cluster_id 会映射为相同 type=rds/cloud_resource_id/region，应被去重折损 1 条。
func TestDiscoverCESResources_DuplicateDedupByUniqueKey(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}}),
		showResp:       buildShowResp("全部资源", "SYS.RDS,rds_cluster_id;SYS.RDS_MYSQL_CLUSTER,rds_cluster_id", 2),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.RDS":               buildListResResp(mkRes("rds-1", "rds_cluster_id", "rds-cluster-1")),
			"SYS.RDS_MYSQL_CLUSTER": buildListResResp(mkRes("rds-mysql-1", "rds_cluster_id", "rds-cluster-1")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 去重后只保留 1 条进入待写入集合。
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (deduped)", len(result.Resources))
	}
	if result.Summary.Discovered != 1 {
		t.Fatalf("Discovered = %d, want 1", result.Summary.Discovered)
	}
	// mapped_count 含重复（2 条均映射成功），unique 为去重后（1），duplicate 为折损（1）。
	if result.Summary.MappedCount != 2 {
		t.Fatalf("MappedCount = %d, want 2 (includes duplicate)", result.Summary.MappedCount)
	}
	if result.Summary.UniqueDiscoveredCount != 1 {
		t.Fatalf("UniqueDiscoveredCount = %d, want 1", result.Summary.UniqueDiscoveredCount)
	}
	if result.Summary.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d, want 1 (cross-scope dedup loss)", result.Summary.DuplicateCount)
	}
	// 非法资源为 0，确保 duplicate 与 invalid 不混淆（修复前会把 invalid 误算进 duplicate）。
	if result.Summary.InvalidResourceCount != 0 {
		t.Fatalf("InvalidResourceCount = %d, want 0", result.Summary.InvalidResourceCount)
	}
	// 契约公式：raw = mapped + invalid。
	if result.Summary.RawFetchedCount != result.Summary.MappedCount+result.Summary.InvalidResourceCount {
		t.Fatalf("raw(%d) != mapped(%d)+invalid(%d)", result.Summary.RawFetchedCount, result.Summary.MappedCount, result.Summary.InvalidResourceCount)
	}
	// 契约公式：mapped = unique + duplicate。
	if result.Summary.MappedCount != result.Summary.UniqueDiscoveredCount+result.Summary.DuplicateCount {
		t.Fatalf("mapped(%d) != unique(%d)+duplicate(%d)", result.Summary.MappedCount, result.Summary.UniqueDiscoveredCount, result.Summary.DuplicateCount)
	}
	// 两个 scope 均查询成功且都映射为 rds，rds 应计入 SuccessfulTypes。
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"rds"}) {
		t.Fatalf("SuccessfulTypes = %v, want [rds]", result.Summary.SuccessfulTypes)
	}
	// 保留的是先到的 SYS.RDS 资源。
	if result.Resources[0].ProviderRef != "rds-cluster-1" || result.Resources[0].Type != "rds" {
		t.Fatalf("retained resource = %+v, want type=rds provider_ref=rds-cluster-1", result.Resources[0])
	}
}

// TestDiscoverCESResources_InvalidBoundaryPageKeepsScanning 验证含 invalid 资源的页
// 仍应继续翻页拉取全部有效资源。max_resources 设为 100（远大于云端 3 条）以保留原意图：
// 确认远端被完整扫描、不触发截断、invalid 边界不中断翻页。
// 修复前 max_resources=2 与云端 3 条会触发截断语义，与本测试“完全扫描”的初衷冲突。
func TestDiscoverCESResources_InvalidBoundaryPageKeepsScanning(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id", 3),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": {
				Count: int32Ptr(3),
				Resources: &[]cesv2model.GetResourceGroupResources{
					{Status: cesv2model.GetGetResourceGroupResourcesStatusEnum().HEALTH, Dimensions: nil},
					mkRes("valid-2", "instance_id", "i-2"),
					mkRes("valid-3", "instance_id", "i-3"),
				},
			},
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("resource count = %d, want 2 (1 invalid dropped)", len(result.Resources))
	}
	if result.Summary.InvalidResourceCount != 1 {
		t.Fatalf("InvalidResourceCount = %d, want 1", result.Summary.InvalidResourceCount)
	}
	if result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = true, want false when remote count is fully scanned")
	}
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"ecs"}) {
		t.Fatalf("SuccessfulTypes = %v, want [ecs]", result.Summary.SuccessfulTypes)
	}
}

// TestDiscoverCESResources_AllDuplicatesConsumeRawBudget 验证全重复资源消耗 raw 预算：
// max_resources=4，3 个产品各返回 2 条相同 dedup key 的资源（1 unique + 1 dup）。
// 修复前预算按 len(resources) 计算，全重复产品不消耗预算，3 个产品共 fetch 6 条原始行
// 但 len(resources)=3 < 4，MaxResourcesReached=false，扫描未截断。
// 修复后预算按 RawFetchedCount 计算，前两个产品 fetch 4 条即达上限，
// MaxResourcesReached=true，第三个产品不被扫描。见 §7.2/§8.6/§13.1。
func TestDiscoverCESResources_AllDuplicatesConsumeRawBudget(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(6)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id;SYS.EVS,disk_name;SYS.RDS,rds_cluster_id", 6),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1"), mkRes("ecs-1-dup", "instance_id", "i-1")),
			"SYS.EVS": buildListResResp(mkRes("disk-1", "disk_name", "d-1"), mkRes("disk-1-dup", "disk_name", "d-1")),
			"SYS.RDS": buildListResResp(mkRes("rds-1", "rds_cluster_id", "rds-1"), mkRes("rds-1-dup", "rds_cluster_id", "rds-1")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 4,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 前两个产品各 fetch 2 条，RawFetchedCount=4 即达上限，第三个产品不被扫描。
	if result.Summary.RawFetchedCount != 4 {
		t.Fatalf("RawFetchedCount = %d, want 4 (raw budget exhausted after 2 products)", result.Summary.RawFetchedCount)
	}
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (raw fetched reached max_resources with more products remaining)")
	}
	// 每个产品 1 unique + 1 dup，2 个产品共 2 unique + 2 dup。
	if len(result.Resources) != 2 {
		t.Fatalf("resource count = %d, want 2 (1 unique per product, 2 products scanned)", len(result.Resources))
	}
	if result.Summary.DuplicateCount != 2 {
		t.Fatalf("DuplicateCount = %d, want 2 (1 dup per product, 2 products scanned)", result.Summary.DuplicateCount)
	}
	// 第三个产品（rds）未被扫描，不应出现在 SuccessfulTypes 中。
	for _, st := range result.Summary.SuccessfulTypes {
		if st == "rds" {
			t.Fatalf("SuccessfulTypes contains rds, but rds was not scanned (truncated)")
		}
	}
}

// TestDiscoverCESResources_AllInvalidConsumeRawBudget 验证全无效资源消耗 raw 预算：
// max_resources=2，产品 A 返回 2 条无维度无效资源，产品 B 返回 1 条有效资源。
// 修复前预算按 len(resources) 计算，全无效资源不消耗预算，
// A fetch 2 条但 len(resources)=0，B 继续扫描，MaxResourcesReached=false。
// 修复后预算按 RawFetchedCount 计算，A fetch 2 条即达上限，
// MaxResourcesReached=true，B 不被扫描。见 §7.2/§8.6/§13.1。
func TestDiscoverCESResources_AllInvalidConsumeRawBudget(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(cesv2model.OneResourceGroupResp{GroupName: "全部资源", GroupId: "rg001", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}}),
		showResp:       buildShowResp("全部资源", "SYS.ECS,instance_id;SYS.EVS,disk_name", 3),
		listResps: map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"SYS.ECS": buildListResResp(
				cesv2model.GetResourceGroupResources{Status: cesv2model.GetGetResourceGroupResourcesStatusEnum().HEALTH, Dimensions: nil},
				cesv2model.GetResourceGroupResources{Status: cesv2model.GetGetResourceGroupResourcesStatusEnum().HEALTH, Dimensions: nil},
			),
			"SYS.EVS": buildListResResp(mkRes("disk-1", "disk_name", "d-1")),
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "r", MaxResources: 2,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// A fetch 2 条无效资源，RawFetchedCount=2 即达上限，B 不被扫描。
	if result.Summary.RawFetchedCount != 2 {
		t.Fatalf("RawFetchedCount = %d, want 2 (raw budget exhausted by invalid resources)", result.Summary.RawFetchedCount)
	}
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached = false, want true (raw fetched reached max_resources with more products remaining)")
	}
	if result.Summary.InvalidResourceCount != 2 {
		t.Fatalf("InvalidResourceCount = %d, want 2", result.Summary.InvalidResourceCount)
	}
	if len(result.Resources) != 0 {
		t.Fatalf("resource count = %d, want 0 (all fetched resources are invalid)", len(result.Resources))
	}
	// B（evs）未被扫描，SuccessfulTypes 应为空。
	if len(result.Summary.SuccessfulTypes) != 0 {
		t.Fatalf("SuccessfulTypes = %v, want empty (truncated before any successful type recorded)", result.Summary.SuccessfulTypes)
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

// TestDiscoverCESResources_AggregateUnionsAllGroups 验证聚合口径下每个资源组都按自己的
// product_names 独立查询，资源取并集。这是真实生产账号的形态：资源按业务线自建分组，
// 没有任何一个组叫"全部资源"，见 ops/huawei-ces-sync-contract.md §8.4 step 4。
func TestDiscoverCESResources_AggregateUnionsAllGroups(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "生产ecs", GroupId: "rg-ecs", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
			cesv2model.OneResourceGroupResp{GroupName: "ELB生产资源分组", GroupId: "rg-elb", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
		showRespsByGroup: map[string]*cesv2model.ShowResourceGroupResponse{
			"rg-ecs": buildShowResp("生产ecs", "SYS.ECS,instance_id", 1),
			"rg-elb": buildShowResp("ELB生产资源分组", "SYS.ELB,loadbalancer_id", 1),
		},
		listRespsByGroup: map[string]map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"rg-ecs": {
				"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
			},
			"rg-elb": {
				"SYS.ELB": buildListResResp(mkRes("elb-1", "loadbalancer_id", "lb-1")),
			},
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Summary.ResourceGroupSelection != resourceGroupModeAllGroups {
		t.Fatalf("ResourceGroupSelection = %q, want %q", result.Summary.ResourceGroupSelection, resourceGroupModeAllGroups)
	}
	if result.Summary.ResourceGroupCount != 2 {
		t.Fatalf("ResourceGroupCount = %d, want 2", result.Summary.ResourceGroupCount)
	}
	// 两个组的资源都要入库，且各自带正确的资源组 label（可追溯到来源分组）。
	if len(result.Resources) != 2 {
		t.Fatalf("resource count = %d, want 2 (union of both groups)", len(result.Resources))
	}
	byID := map[string]domain.CloudResource{}
	for _, r := range result.Resources {
		byID[r.ProviderRef] = r
	}
	ecs, ok := byID["i-1"]
	if !ok {
		t.Fatalf("missing ecs resource: %+v", result.Resources)
	}
	if ecs.Labels["resource_group_id"] != "rg-ecs" || ecs.Labels["resource_group_name"] != "生产ecs" {
		t.Fatalf("ecs labels = %+v, want rg-ecs/生产ecs", ecs.Labels)
	}
	elb, ok := byID["lb-1"]
	if !ok {
		t.Fatalf("missing elb resource: %+v", result.Resources)
	}
	if elb.Labels["resource_group_id"] != "rg-elb" {
		t.Fatalf("elb labels = %+v, want rg-elb", elb.Labels)
	}
	if !reflect.DeepEqual(result.Summary.SuccessfulTypes, []string{"ecs", "elb"}) {
		t.Fatalf("SuccessfulTypes = %v, want [ecs elb]", result.Summary.SuccessfulTypes)
	}
	// 所有组都解析成功 → scope 完整 → 允许反向 stale。
	if result.Summary.ScopeIncomplete {
		t.Fatalf("ScopeIncomplete should be false when every group resolves")
	}
	if result.Summary.ResourceLevel != resourceLevelProduct {
		t.Fatalf("ResourceLevel = %q, want %q", result.Summary.ResourceLevel, resourceLevelProduct)
	}
}

// TestDiscoverCESResources_AggregateDedupsAcrossGroups 验证聚合口径下同一资源同时命中
// 多个资源组时按唯一键去重，不重复入库，见 ops/huawei-ces-sync-contract.md §9.4。
func TestDiscoverCESResources_AggregateDedupsAcrossGroups(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "生产ecs", GroupId: "rg-ecs", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
			cesv2model.OneResourceGroupResp{GroupName: "测试环境资源", GroupId: "rg-test", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
		showRespsByGroup: map[string]*cesv2model.ShowResourceGroupResponse{
			"rg-ecs":  buildShowResp("生产ecs", "SYS.ECS,instance_id", 1),
			"rg-test": buildShowResp("测试环境资源", "SYS.ECS,instance_id", 1),
		},
		listRespsByGroup: map[string]map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"rg-ecs": {
				"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
			},
			"rg-test": {
				"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
			},
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (deduped across groups)", len(result.Resources))
	}
	if result.Summary.MappedCount != 2 {
		t.Fatalf("MappedCount = %d, want 2 (includes duplicate)", result.Summary.MappedCount)
	}
	if result.Summary.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d, want 1", result.Summary.DuplicateCount)
	}
	if result.Summary.Discovered != 1 {
		t.Fatalf("Discovered = %d, want 1", result.Summary.Discovered)
	}
}

// TestDiscoverCESResources_AggregateSkippedGroupMarksScopeIncomplete 验证聚合口径下
// 单个资源组解析失败时：不影响其余组同步，但 scope 标记为不完整（禁止反向 stale），
// 见 ops/huawei-ces-sync-contract.md §8.4/§13.1。
func TestDiscoverCESResources_AggregateSkippedGroupMarksScopeIncomplete(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "生产ecs", GroupId: "rg-ecs", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
			cesv2model.OneResourceGroupResp{GroupName: "坏组", GroupId: "rg-bad", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
		showRespsByGroup: map[string]*cesv2model.ShowResourceGroupResponse{
			"rg-ecs": buildShowResp("生产ecs", "SYS.ECS,instance_id", 1),
		},
		showErrByGroup: map[string]error{
			"rg-bad": apperr.New(apperr.CodeUnavailable, "show failed"),
		},
		listRespsByGroup: map[string]map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"rg-ecs": {
				"SYS.ECS": buildListResResp(mkRes("ecs-1", "instance_id", "i-1")),
			},
		},
	}
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", MaxResources: 100,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 可解析的组仍要正常同步。
	if len(result.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1 (healthy group still synced)", len(result.Resources))
	}
	// scope 不完整 → 禁止反向 stale。
	if !result.Summary.ScopeIncomplete {
		t.Fatalf("ScopeIncomplete should be true when a group is skipped")
	}
	if len(result.Summary.FailedScopes) != 1 {
		t.Fatalf("FailedScopes = %v, want 1", result.Summary.FailedScopes)
	}
}

// TestDiscoverCESResources_AggregateSharesMaxResourcesBudget 验证聚合口径下 max_resources
// 原始行预算在所有资源组之间共享，不会因组数变多而被放大，见契约 §11。
func TestDiscoverCESResources_AggregateSharesMaxResourcesBudget(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "g1", GroupId: "rg1", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}},
			cesv2model.OneResourceGroupResp{GroupName: "g2", GroupId: "rg2", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}},
		),
		showRespsByGroup: map[string]*cesv2model.ShowResourceGroupResponse{
			"rg1": buildShowResp("g1", "SYS.ECS,instance_id", 3),
			"rg2": buildShowResp("g2", "SYS.ECS,instance_id", 3),
		},
		listRespsByGroup: map[string]map[string]*cesv2model.ListResourceGroupsServicesResourcesResponse{
			"rg1": {
				"SYS.ECS": buildListResResp(
					mkRes("ecs-1", "instance_id", "i-1"),
					mkRes("ecs-2", "instance_id", "i-2"),
					mkRes("ecs-3", "instance_id", "i-3"),
				),
			},
			"rg2": {
				"SYS.ECS": buildListResResp(
					mkRes("ecs-4", "instance_id", "i-4"),
					mkRes("ecs-5", "instance_id", "i-5"),
					mkRes("ecs-6", "instance_id", "i-6"),
				),
			},
		},
	}
	// 预算 4：两个组共 6 条原始行，聚合后必须整体截断。
	result, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", MaxResources: 4,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Summary.RawFetchedCount > 4 {
		t.Fatalf("RawFetchedCount = %d, want <= 4 (shared budget)", result.Summary.RawFetchedCount)
	}
	if !result.Summary.MaxResourcesReached {
		t.Fatalf("MaxResourcesReached should be true when budget exhausted across groups")
	}
}

// TestDiscoverCESResources_AggregateAllGroupsUnresolvableFails 验证聚合口径下所有资源组
// 都解析不出可查询产品时直接失败，而不是返回空结果（空结果会被误当作"云端无资源"）。
func TestDiscoverCESResources_AggregateAllGroupsUnresolvableFails(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "g1", GroupId: "rg1", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
			cesv2model.OneResourceGroupResp{GroupName: "g2", GroupId: "rg2", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
		showErrByGroup: map[string]error{
			"rg1": apperr.New(apperr.CodeUnavailable, "show failed"),
			"rg2": apperr.New(apperr.CodeUnavailable, "show failed"),
		},
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", MaxResources: 100,
	})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound when no group resolves", apperr.CodeOf(err))
	}
	if !strings.Contains(err.Error(), "no CES resource group could be resolved") {
		t.Fatalf("error message should explain no group resolved: %s", err.Error())
	}
}

// TestDiscoverCESResources_ExplicitNameStillFailsWhenMissing 验证显式指定名称未命中时
// 仍然直接失败——用户明确指定了业务口径，不能静默改成全量聚合，见契约 §8.4 step 2。
func TestDiscoverCESResources_ExplicitNameStillFailsWhenMissing(t *testing.T) {
	api := &mockCESResourceGroupAPI{
		listGroupsResp: buildListGroupsResp(
			cesv2model.OneResourceGroupResp{GroupName: "生产ecs", GroupId: "rg-ecs", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
		),
	}
	_, err := discoverCESResources(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "proj", Region: "cn-south-1", ResourceGroupName: "生产ecs-typo", MaxResources: 100,
	})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound for explicit name mismatch", apperr.CodeOf(err))
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
	limit := int32(obsapp.DefaultCESPageLimit)
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
	sel, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sel.Groups) != 1 {
		t.Fatalf("Groups count = %d, want 1 (single group hit)", len(sel.Groups))
	}
	if sel.Groups[0].GroupID != "rg150" {
		t.Fatalf("GroupID = %q, want rg150 (全部资源 across all pages)", sel.Groups[0].GroupID)
	}
	if sel.Groups[0].Total != 9999 {
		t.Fatalf("Total = %d, want 9999", sel.Groups[0].Total)
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
	sel, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sel.Groups) != 1 || sel.Groups[0].GroupID != "rg-en" {
		t.Fatalf("selection = %+v, want single group rg-en (english name fallback)", sel.Groups)
	}
}

// 没有任何候选名命中时进入聚合口径（全部资源组并集），不再直接失败，
// 也不回退到 total 最大的组——单个业务组不是全量，静默当成全量会造成资产缺失与误标 stale。
// 见 ops/huawei-ces-sync-contract.md §8.4 step 4。
func TestSelectResourceGroup_NoMatchAggregatesAllGroups(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "team-a", GroupId: "rg1", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}},
		{GroupName: "team-b", GroupId: "rg2", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(7)}},
		{GroupName: "team-c", GroupId: "rg3", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(2)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	sel, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sel.Mode != resourceGroupModeAllGroups {
		t.Fatalf("Mode = %q, want %q", sel.Mode, resourceGroupModeAllGroups)
	}
	if len(sel.Groups) != 3 {
		t.Fatalf("Groups count = %d, want 3 (aggregate all groups)", len(sel.Groups))
	}
	// 聚合口径必须包含全部组，不能只取 total 最大的那个。
	got := map[string]bool{}
	for _, g := range sel.Groups {
		got[g.GroupID] = true
	}
	for _, want := range []string{"rg1", "rg2", "rg3"} {
		if !got[want] {
			t.Fatalf("aggregate selection missing group %q: %+v", want, sel.Groups)
		}
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
	sel, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sel.Groups) != 1 || sel.Groups[0].GroupID != "rg-retry" || api.listGroupsCalls != 2 {
		t.Fatalf("selection=%+v calls=%d, want retry success on second call", sel.Groups, api.listGroupsCalls)
	}
}

// 用户指定名优先于默认候选名。
func TestSelectResourceGroup_SpecifiedNamePriority(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "全部资源", GroupId: "rg-default", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(100)}},
		{GroupName: "my-custom", GroupId: "rg-custom", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(1)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	sel, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r", ResourceGroupName: "my-custom"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(sel.Groups) != 1 || sel.Groups[0].GroupID != "rg-custom" {
		t.Fatalf("selection = %+v, want rg-custom (specified name priority)", sel.Groups)
	}
	if sel.Groups[0].Selection != resourceGroupModeSpecifiedName {
		t.Fatalf("Selection = %q, want %q", sel.Groups[0].Selection, resourceGroupModeSpecifiedName)
	}
}

func TestSelectResourceGroup_SpecifiedNameNoFallbackToDefault(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "全部资源", GroupId: "rg-default", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(100)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	_, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r", ResourceGroupName: "my-custom"})
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err code = %v, want NotFound for unmatched specified name", apperr.CodeOf(err))
	}
}

// 显式指定名称未命中时，错误应带上可用资源组（名称+ID），让运维能自助定位，
// 见 ops/huawei-ces-sync-contract.md §8.4。
func TestSelectResourceGroup_NoMatchErrorListsAvailableGroups(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "team-a", GroupId: "rg1", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}},
		{GroupName: "team-b", GroupId: "rg2", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(7)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	_, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "p", Region: "r", ResourceGroupName: "team-typo",
	})
	if err == nil {
		t.Fatalf("expected error when specified name misses")
	}
	msg := err.Error()
	for _, want := range []string{"no CES resource group matched", "team-a", "rg1", "team-b", "rg2"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message missing %q: %s", want, msg)
		}
	}
}

// 显式指定名称未命中时，错误应回显所指定的名称，便于判断是"名称写错"。
func TestSelectResourceGroup_SpecifiedNameMismatchErrorEchoesName(t *testing.T) {
	groups := []cesv2model.OneResourceGroupResp{
		{GroupName: "team-a", GroupId: "rg1", ResourceStatistics: &cesv2model.OneResourceGroupRespResourceStatistics{Total: int32Ptr(3)}},
	}
	api := &pagedGroupsAPI{groups: groups}
	_, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{
		ProjectID: "p", Region: "r", ResourceGroupName: "my-typo",
	})
	if err == nil {
		t.Fatalf("expected error when specified name misses")
	}
	msg := err.Error()
	if !strings.Contains(msg, "my-typo") {
		t.Fatalf("error message should echo the specified name: %s", msg)
	}
	if !strings.Contains(msg, "team-a") {
		t.Fatalf("error message should list available groups: %s", msg)
	}
}

// 该 project/region 下完全没有资源组时，错误应提示去 CES 控制台创建或显式指定 resource_group_id。
func TestSelectResourceGroup_NoGroupsErrorSuggestsCreate(t *testing.T) {
	api := &pagedGroupsAPI{groups: []cesv2model.OneResourceGroupResp{}}
	_, err := selectResourceGroups(context.Background(), api, CESResourceDiscoveryRequest{ProjectID: "p", Region: "r"})
	if err == nil {
		t.Fatalf("expected error when no resource group exists")
	}
	msg := err.Error()
	if !strings.Contains(msg, "resource_group_id") {
		t.Fatalf("error message should suggest configuring resource_group_id: %s", msg)
	}
}
