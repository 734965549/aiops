package huawei

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	cesv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

const defaultCESResourceTimeout = 60 * time.Second

var cesRetryDelays = []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}

// CESResourceDiscoveryRequest CES 资源全量发现请求，见 ops/huawei-ces-sync-contract.md §7.2。
type CESResourceDiscoveryRequest struct {
	ProjectID           string
	Region              string
	EnterpriseProjectID string
	ResourceGroupName   string
	ResourceGroupID     string
	MaxResources        int
}

// CESResourceDiscoverySummary 记录单次发现的逐 scope 摘要，用于 batch message 排查，见 ops/huawei-ces-sync-contract.md §7.2。
type CESResourceDiscoverySummary struct {
	ProjectID              string
	Region                 string
	ResourceGroupID        string
	ResourceGroupName      string
	ResourceGroupSelection string
	CESTotal               int
	RawFetchedCount        int
	MappedCount            int
	UniqueDiscoveredCount  int
	PersistedCount         int
	DuplicateCount         int
	PersistFailedCount     int
	Discovered             int
	FailedScopes           []string
	SuccessfulTypes        []string
	// QueryFailedTypes 记录存在 scope 查询失败的类型，见 ops/huawei-ces-sync-contract.md §13.1。
	// 同一类型只要有一个 service+dim_name scope 查询失败，该类型不得进入 SuccessfulTypes，
	// 否则 sync_service 会把未查询到的资产误标为 stale。
	QueryFailedTypes      []string
	UnknownNamespaceCount int
	InvalidResourceCount  int
	// ConversionFailedTypes 记录存在资源转换失败的类型，sync_service 据此禁止该类型执行 stale（资源转换不完整），见 ops/huawei-ces-sync-contract.md §13。
	ConversionFailedTypes []string
	ProductNamesEmpty     bool
	// ResourceLevel 记录 ShowResourceGroup 响应中的 resource_level 字段（product|dimension）。
	// 仅 resource_level == "product" 且 product_names 非空时，scope 才是权威范围，允许反向 stale。
	// P0 不支持 dimension 级资源组，发现阶段直接返回 FAILED_PRECONDITION。见 §8.5/§13.1。
	ResourceLevel string
	// MaxResourcesReached 表示因达到 max_resources 上限而提前终止发现，
	// 此时 SuccessfulTypes 不含被截断的类型，调用方必须禁止该 scope 执行 stale 标记，见 §13。
	MaxResourcesReached bool
}

// CESResourceDiscoveryResult CES 资源发现结果。
type CESResourceDiscoveryResult struct {
	Resources []domain.CloudResource
	Summary   CESResourceDiscoverySummary
}

// cesResourceGroupAPI 抽象 CES v2 资源分组相关 SDK 调用，便于单测注入 mock。
type cesResourceGroupAPI interface {
	ListResourceGroups(request *cesv2model.ListResourceGroupsRequest) (*cesv2model.ListResourceGroupsResponse, error)
	ShowResourceGroup(request *cesv2model.ShowResourceGroupRequest) (*cesv2model.ShowResourceGroupResponse, error)
	ListResourceGroupsServicesResources(request *cesv2model.ListResourceGroupsServicesResourcesRequest) (*cesv2model.ListResourceGroupsServicesResourcesResponse, error)
}

// CESResourceDiscoveryClient CES 资源全量发现客户端；生产实现基于 cesv2.CesClient，单测可注入 mock。
type CESResourceDiscoveryClient interface {
	ListCESResources(ctx context.Context, cred AKSKCredential, req CESResourceDiscoveryRequest) (*CESResourceDiscoveryResult, error)
}

// CESResourceClient 生产实现。
type CESResourceClient struct{}

func NewCESResourceClient() *CESResourceClient {
	return &CESResourceClient{}
}

var _ CESResourceDiscoveryClient = (*CESResourceClient)(nil)

// ListCESResources 按 ops/huawei-ces-sync-contract.md §8.1 执行 CES 资源全量发现：
// ListResourceGroups -> 选组 -> ShowResourceGroup 解析 product_names ->
// 对每个 service+dim_name 分页 ListResourceGroupsServicesResources -> mapCESResource。
func (c *CESResourceClient) ListCESResources(ctx context.Context, cred AKSKCredential, req CESResourceDiscoveryRequest) (*CESResourceDiscoveryResult, error) {
	if c == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei ces resource client is not configured")
	}
	if err := validateCESDiscoveryRequest(req, cred); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, mapCESError(err)
	}
	client, err := newCESv2Client(cred, req.ProjectID, req.Region)
	if err != nil {
		return nil, err
	}
	return discoverCESResources(ctx, client, req)
}

// discoverCESResources 与 SDK 客户端解耦的发现逻辑，便于单测注入 mock cesResourceGroupAPI。
func discoverCESResources(ctx context.Context, api cesResourceGroupAPI, req CESResourceDiscoveryRequest) (*CESResourceDiscoveryResult, error) {
	summary := CESResourceDiscoverySummary{
		ProjectID: req.ProjectID,
		Region:    req.Region,
	}
	maxResources := req.MaxResources
	if maxResources <= 0 {
		maxResources = obsapp.DefaultRawRowBudget
	}

	group, err := selectResourceGroup(ctx, api, req)
	if err != nil {
		return nil, err
	}
	summary.ResourceGroupID = group.GroupID
	summary.ResourceGroupName = group.GroupName
	summary.ResourceGroupSelection = group.Selection
	if group.Total > 0 {
		summary.CESTotal = group.Total
	}

	products, productNamesEmpty, resourceLevel, err := resolveProducts(ctx, api, group.GroupID, group.shown)
	if err != nil {
		return nil, err
	}
	summary.ProductNamesEmpty = productNamesEmpty
	summary.ResourceLevel = resourceLevel

	// P0 仅支持 resource_level == "product" 的资源组。dimension 级资源组 scope 语义不同
	//（product_names 不含 dim_name），不能直接复用产品级反向 stale 逻辑；未知/空层级同理。
	// 明确返回 FAILED_PRECONDITION，不静默回退，见 §8.5/§13.1。
	if resourceLevel != "product" {
		return nil, apperr.New(apperr.CodeFailedPrecondition,
			fmt.Sprintf("CES resource group resource_level=%q is not supported (only \"product\" is supported in P0)", resourceLevel))
	}

	result := &CESResourceDiscoveryResult{Summary: summary}
	resources := make([]domain.CloudResource, 0, min(maxResources, 1024))
	// seenKeys 按 asset_resource 部分唯一索引（迁移 0026）对齐的唯一键去重，
	// 唯一键为 (cloud_resource_type, cloud_resource_id, region)，integration_account_id 对同一批次恒定，
	// 见 ops/huawei-ces-sync-contract.md §9.4。
	seenKeys := make(map[string]struct{}, maxResources)
	// max_resources 预算按"已从云端接收的原始行数"(RawFetchedCount) 维护，而非去重后的 unique 资源数。
	// 重复资源和无效资源同样消耗预算，确保超大重复/异常返回不会绕开 max_resources 保护持续翻页，
	// 见 ops/huawei-ces-sync-contract.md §7.2/§8.6。
	for i, product := range products {
		if result.Summary.RawFetchedCount >= maxResources {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, mapCESError(err)
		}
		remaining := maxResources - result.Summary.RawFetchedCount
		pageResources, _, remoteExhausted, truncated, pageErr := listResourcesForProduct(ctx, api, group.GroupID, product, req.Region, remaining)
		result.Summary.RawFetchedCount += len(pageResources)
		if pageErr != nil {
			result.Summary.FailedScopes = append(result.Summary.FailedScopes,
				fmt.Sprintf("%s/%s: %s", req.Region, product.Service, apperr.FromError(pageErr).Message))
			result.Summary.QueryFailedTypes = appendUniqueString(
				result.Summary.QueryFailedTypes, resolveNamespaceMappingByDim(product.Service, product.DimNames).CloudResourceType)
			logger.From(ctx).Warn("huawei ces list resources failed",
				logger.String("region", req.Region),
				logger.String("namespace", product.Service),
				logger.String("dim_name", strings.Join(product.DimNames, ",")),
				logger.String("error_code", string(apperr.CodeOf(pageErr))),
			)
			continue
		}
		mappedCount := 0
		dupCount := 0
		for _, in := range pageResources {
			cloud, ok := mapCESResource(req.Region, product.Service, product.DimNames, in, group.GroupID, group.GroupName)
			if !ok {
				result.Summary.InvalidResourceCount++
				result.Summary.ConversionFailedTypes = appendUniqueString(
					result.Summary.ConversionFailedTypes, resolveNamespaceMappingByDim(product.Service, product.DimNames).CloudResourceType)
				continue
			}
			if isUnknownNamespace(product.Service) {
				result.Summary.UnknownNamespaceCount++
			}
			// mapped_count 统计映射成功的资源（含重复），满足 raw = mapped + invalid，
			// 见 ops/huawei-ces-sync-contract.md §9.5。
			mappedCount++
			// 按唯一键去重：重复资源计入 duplicate_count 且不进入待写入集合，
			// 保证 mapped = unique + duplicate、unique = persisted + persist_failed 成立，
			// 见 ops/huawei-ces-sync-contract.md §9.5、§9.4。
			key := cloudResourceDedupKey(cloud)
			if _, exists := seenKeys[key]; exists {
				dupCount++
				continue
			}
			seenKeys[key] = struct{}{}
			resources = append(resources, cloud)
		}
		// discovery 阶段只产出 raw/mapped/unique/invalid/duplicate；
		// persisted/persist_failed 由 Asset 层 upsert 后统一填充，见 docs/huawei-ces-sync-runbook.md。
		result.Summary.MappedCount += mappedCount
		result.Summary.DuplicateCount += dupCount
		if truncated {
			// 单产品资源数超过 remaining 上限，远端仍有未取数据：标记截断，
			// 被截断类型不计入 SuccessfulTypes，整 region 跳过 stale，见 §13.1 截断探测。
			// 提前于 RawFetchedCount>=maxResources 判断，覆盖"全部 conversion failed 或全部重复导致
			// RawFetchedCount 未达上限"的边界（此时类型仍不应计入 SuccessfulTypes）。
			result.Summary.MaxResourcesReached = true
			break
		}
		if result.Summary.RawFetchedCount >= maxResources {
			if !remoteExhausted || i < len(products)-1 {
				result.Summary.MaxResourcesReached = true
				break
			}
		}
		// 查询成功且未截断即记录类型；0 资源也计入 SuccessfulTypes，
		// 使旧资产可被标记 stale，与 native 路径 (adapter.go) 及 §13.1 "查询成功且本轮 0 资源的类型 → 旧资产 stale" 一致。
		// 同类型任一 scope 失败时，由下方 subtractStringSet 从 QueryFailedTypes 剔除。
		resourceType := resolveNamespaceMappingByDim(product.Service, product.DimNames).CloudResourceType
		result.Summary.SuccessfulTypes = appendUniqueString(result.Summary.SuccessfulTypes, resourceType)
	}
	// SuccessfulTypes 只保留所有 scope 都成功的类型：剔除存在 scope 查询失败的类型，见 §13.1。
	// 例如 SYS.ELB/loadbalancer_id 成功但 SYS.ELB/l7policy_id 失败时，elb 不得计入 SuccessfulTypes。
	result.Summary.SuccessfulTypes = subtractStringSet(result.Summary.SuccessfulTypes, result.Summary.QueryFailedTypes)
	result.Resources = resources
	// unique_discovered_count 为去重后最终进入待写入集合的资源数，等于 persisted + persist_failed，
	// 见 ops/huawei-ces-sync-contract.md §9.5。Discovered 与之同义，保留以兼容既有字段语义。
	result.Summary.Discovered = len(resources)
	result.Summary.UniqueDiscoveredCount = len(resources)
	result.Summary.ResourceGroupID = group.GroupID
	result.Summary.ResourceGroupName = group.GroupName
	result.Summary.ResourceGroupSelection = group.Selection
	result.Summary.CESTotal = group.Total
	result.Summary.ProductNamesEmpty = productNamesEmpty
	result.Summary.ResourceLevel = resourceLevel
	return result, nil
}

// selectedResourceGroup 选中的资源组摘要。
type selectedResourceGroup struct {
	GroupID   string
	GroupName string
	Total     int
	Selection string
	// shown 携带 ShowResourceGroup 完整响应（仅指定 resource_group_id 时填充），
	// 供 resolveProducts 复用，避免对同一 group 二次调用 ShowResourceGroup。
	shown *shownResourceGroup
}

// defaultResourceGroupCandidates 默认候选名，需用户在 CES 控制台预先创建同名资源分组，见 §8.4。
// 注意：这些名称并非 CES 系统内置分组；未命中即失败，不回退到最大资源组。
var defaultResourceGroupCandidates = []string{
	"全部资源",
	"All resources",
	"All Resources",
}

// maxResourceGroupOffset ListResourceGroups offset 上限（SDK 区间 [0,10000]），防止异常翻页。
const maxResourceGroupOffset int32 = 10000

// selectResourceGroup 按 §8.4 选择目标资源组：指定ID > 显式名称精确匹配 > 默认候选名。
// 显式名称未命中时直接失败，不回退默认候选名；未指定名称时才尝试默认候选名。
// 任何名称未命中均直接失败，不回退到 total 最大的资源组，避免把某个业务组误当作全量。
// 分页拉全量后客户端匹配（不依赖服务端 GroupName 模糊过滤）。
func selectResourceGroup(ctx context.Context, api cesResourceGroupAPI, req CESResourceDiscoveryRequest) (selectedResourceGroup, error) {
	if id := strings.TrimSpace(req.ResourceGroupID); id != "" {
		// 指定 ID 时仍走 ShowResourceGroup 校验存在并取 total。
		group, err := showGroup(ctx, api, id)
		if err != nil {
			return selectedResourceGroup{}, err
		}
		return selectedResourceGroup{GroupID: id, GroupName: group.GroupName, Total: group.Total, Selection: "specified_id", shown: &group}, nil
	}

	groups, err := listAllResourceGroups(ctx, api, req)
	if err != nil {
		return selectedResourceGroup{}, err
	}
	if len(groups) == 0 {
		return selectedResourceGroup{}, apperr.New(apperr.CodeNotFound, "no CES resource group found for project/region")
	}

	if specifiedName := strings.TrimSpace(req.ResourceGroupName); specifiedName != "" {
		if sel, ok := matchResourceGroupByName(groups, specifiedName, "specified_name"); ok {
			return sel, nil
		}
		return selectedResourceGroup{}, apperr.New(apperr.CodeNotFound,
			"no CES resource group matched (specified id/name or default candidates)")
	}

	// 未指定名称时才依次尝试默认候选名，见 §8.4 step 3。
	for _, wantName := range defaultResourceGroupCandidates {
		if sel, ok := matchResourceGroupByName(groups, wantName, "default_name"); ok {
			return sel, nil
		}
	}
	// 默认候选名均未命中，直接失败，不回退最大资源组，见 §8.4 step 4。
	// CES 官方 ListResourceGroups 只返回用户创建的资源分组，不存在"总览全量"隐式口径。
	return selectedResourceGroup{}, apperr.New(apperr.CodeNotFound,
		"no CES resource group matched (specified id/name or default candidates)")
}

func matchResourceGroupByName(groups []cesv2model.OneResourceGroupResp, wantName, selection string) (selectedResourceGroup, bool) {
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.GroupName), strings.TrimSpace(wantName)) {
			return selectedResourceGroup{GroupID: g.GroupId, GroupName: g.GroupName, Total: groupTotal(g.ResourceStatistics), Selection: selection}, true
		}
	}
	return selectedResourceGroup{}, false
}

// listAllResourceGroups 分页拉取全部 CES 资源组，见 §8.6 分页策略。
// SDK ListResourceGroups 支持 offset[0,10000]/limit[1,100]，每页 100。
func listAllResourceGroups(ctx context.Context, api cesResourceGroupAPI, req CESResourceDiscoveryRequest) ([]cesv2model.OneResourceGroupResp, error) {
	pageLimit := int32(obsapp.DefaultCESPageLimit) // SDK limit 上限 100
	ep := strings.TrimSpace(req.EnterpriseProjectID)
	out := make([]cesv2model.OneResourceGroupResp, 0, pageLimit)
	var offset int32 = 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, mapCESError(err)
		}
		listReq := &cesv2model.ListResourceGroupsRequest{
			Offset: &offset,
			Limit:  &pageLimit,
		}
		if ep != "" {
			listReq.EnterpriseProjectId = &ep
		}
		resp, err := callCESWithRetry(ctx, req.Region, "list resource groups", func() (*cesv2model.ListResourceGroupsResponse, error) {
			return api.ListResourceGroups(listReq)
		})
		if err != nil {
			return nil, err
		}
		if resp == nil || resp.ResourceGroups == nil || len(*resp.ResourceGroups) == 0 {
			break
		}
		out = append(out, *resp.ResourceGroups...)
		// 服务端返回 count，已收集达到总数则停止。
		if resp.Count != nil && int32(len(out)) >= *resp.Count {
			break
		}
		// 不足一页说明已是末页。
		if int32(len(*resp.ResourceGroups)) < pageLimit {
			break
		}
		offset += pageLimit
		if offset >= maxResourceGroupOffset {
			// SDK offset 上限 10000，超出则无法继续翻页。
			// 若服务端 count 表明还有更多资源组，返回错误而非静默截断，避免 selectResourceGroup 在不完整列表上误判。
			if resp.Count != nil && int32(len(out)) < *resp.Count {
				return nil, apperr.New(apperr.CodeFailedPrecondition,
					fmt.Sprintf("CES resource group count %d exceeds SDK offset limit %d, results truncated", *resp.Count, maxResourceGroupOffset))
			}
			break
		}
	}
	return out, nil
}

// shownResourceGroup ShowResourceGroup 结果摘要。
type shownResourceGroup struct {
	GroupName     string
	ProductNames  string
	ResourceLevel string
	Total         int
}

func showGroup(ctx context.Context, api cesResourceGroupAPI, groupID string) (shownResourceGroup, error) {
	resp, err := callCESWithRetry(ctx, "", "show resource group", func() (*cesv2model.ShowResourceGroupResponse, error) {
		return api.ShowResourceGroup(&cesv2model.ShowResourceGroupRequest{GroupId: groupID})
	})
	if err != nil {
		return shownResourceGroup{}, err
	}
	out := shownResourceGroup{}
	if resp != nil {
		if resp.GroupName != nil {
			out.GroupName = strings.TrimSpace(*resp.GroupName)
		}
		if resp.ProductNames != nil {
			out.ProductNames = strings.TrimSpace(*resp.ProductNames)
		}
		if resp.ResourceLevel != nil {
			out.ResourceLevel = resp.ResourceLevel.Value()
		}
		if resp.ResourceStatistics != nil && resp.ResourceStatistics.Total != nil {
			out.Total = int(*resp.ResourceStatistics.Total)
		}
	}
	return out, nil
}

// resolveProducts 解析 product_names；为空时回落到内置白名单，见 §8.5。
// 返回值: products, productNamesEmpty, resourceLevel, error。
// precomputed 非空时复用已有的 ShowResourceGroup 响应，避免重复调用。
func resolveProducts(ctx context.Context, api cesResourceGroupAPI, groupID string, precomputed *shownResourceGroup) ([]cesProduct, bool, string, error) {
	var group shownResourceGroup
	if precomputed != nil {
		group = *precomputed
	} else {
		g, err := showGroup(ctx, api, groupID)
		if err != nil {
			return nil, false, "", err
		}
		group = g
	}
	products, multiDim := parseProductNames(group.ProductNames)
	if multiDim {
		logger.From(ctx).Warn("huawei ces product_names contains single-item multi-dim, split into separate queries",
			logger.String("product_names", group.ProductNames))
	}
	if len(products) == 0 {
		return fallbackProducts(), true, group.ResourceLevel, nil
	}
	return products, false, group.ResourceLevel, nil
}

// listResourcesForProduct 按 service+dim_name 分页拉取资源，见 §8.6。
func listResourcesForProduct(ctx context.Context, api cesResourceGroupAPI, groupID string, product cesProduct, region string, remaining int) ([]cesResourceInput, int, bool, bool, error) {
	pageLimit := int32(obsapp.DefaultCESPageLimit)
	if remaining > 0 && int(pageLimit) > remaining {
		pageLimit = int32(remaining)
	}
	out := make([]cesResourceInput, 0, min(int(pageLimit), remaining))
	offset := int32(0)
	remoteExhausted := false
	truncated := false // 达到 remaining 上限但远端仍可能有数据时置 true，见 §13.1 截断探测
	rawFetched := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, false, false, mapCESError(err)
		}
		req := &cesv2model.ListResourceGroupsServicesResourcesRequest{
			GroupId: groupID,
			Service: product.Service,
			Offset:  &offset,
			Limit:   ptrCESLimit(pageLimit),
		}
		if len(product.DimNames) > 0 {
			joined := strings.Join(product.DimNames, ",")
			req.DimName = &joined
		}
		resp, err := callCESWithRetry(ctx, region, "list resource group services resources", func() (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
			return api.ListResourceGroupsServicesResources(req)
		})
		if err != nil {
			return nil, 0, false, false, err
		}
		if resp == nil {
			remoteExhausted = true
			break
		}
		pageCount := int32(0)
		if resp.Count != nil {
			pageCount = *resp.Count
		}
		rawCount := 0
		if resp.Resources != nil {
			rawCount = len(*resp.Resources)
		}
		if rawCount == 0 {
			if pageCount == 0 {
				remoteExhausted = true
				break
			}
			if offset >= pageCount {
				remoteExhausted = true
				break
			}
			offset += pageLimit
			if offset >= pageCount {
				remoteExhausted = true
				break
			}
			continue
		}
		rawFetched += rawCount
		for _, res := range *resp.Resources {
			if len(out) >= remaining {
				truncated = true // 达到 remaining 上限，远端仍可能有数据，见 §13.1 截断探测
				break
			}
			out = append(out, toCESResourceInput(res))
		}
		if truncated {
			break // 已达上限，不再翻页
		}
		offset += int32(rawCount)
		if pageCount > 0 && offset >= pageCount {
			remoteExhausted = true
			break
		}
		if pageCount == 0 && rawCount < int(pageLimit) {
			remoteExhausted = true
			break
		}
	}
	_ = rawFetched
	return out, len(out), remoteExhausted, truncated, nil
}

// toCESResourceInput 将 SDK GetResourceGroupResources 转为 provider 无关输入。
func toCESResourceInput(res cesv2model.GetResourceGroupResources) cesResourceInput {
	in := cesResourceInput{
		Status:       res.Status.Value(),
		EventStatus:  "",
		ResourceName: "",
		Dimensions:   make([]cesDimInput, 0, len(res.Dimensions)),
	}
	if res.EventStatus != nil {
		in.EventStatus = res.EventStatus.Value()
	}
	if res.ResourceName != nil {
		in.ResourceName = strings.TrimSpace(*res.ResourceName)
	}
	if res.EnterpriseProjectId != nil {
		in.EnterpriseProjectID = strings.TrimSpace(*res.EnterpriseProjectId)
	}
	for _, dim := range res.Dimensions {
		in.Dimensions = append(in.Dimensions, cesDimInput{Name: dim.Name, Value: dim.Value})
	}
	// CES tags 是 JSON 字符串（见 §6 resources[].tags），解析为结构化 tag 列表；
	// 解析失败时 tags 为空，不阻断同步。
	if res.Tags != nil {
		in.Tags = parseCESResourceTagsJSON(*res.Tags)
	}
	return in
}

func groupTotal(stats *cesv2model.OneResourceGroupRespResourceStatistics) int {
	if stats == nil || stats.Total == nil {
		return 0
	}
	return int(*stats.Total)
}

func appendUniqueString(values []string, value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// cloudResourceDedupKey 返回资源在批次内的去重唯一键，与 asset_resource 部分唯一索引
// （迁移 0026：(integration_account_id, cloud_resource_type, cloud_resource_id, region)）对齐，
// 见 ops/huawei-ces-sync-contract.md §9.4。
// integration_account_id 对同一批次恒定，故去重键仅含 type/id/region；
// 字段 TrimSpace 与 Asset 层 buildCloudResourceBatch 落库逻辑保持一致，避免两端口径偏差。
func cloudResourceDedupKey(r domain.CloudResource) string {
	return strings.TrimSpace(r.Type) + "|" + strings.TrimSpace(r.ProviderRef) + "|" + strings.TrimSpace(r.Region)
}

// subtractStringSet 从 values 中剔除存在于 subtract 集合的项，返回新切片。
// 用于从 SuccessfulTypes 移除存在 scope 查询失败的类型，见 §13.1。
func subtractStringSet(values []string, subtract []string) []string {
	if len(subtract) == 0 {
		return values
	}
	bad := make(map[string]struct{}, len(subtract))
	for _, v := range subtract {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			bad[v] = struct{}{}
		}
	}
	if len(bad) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, skip := bad[v]; skip {
			continue
		}
		out = append(out, v)
	}
	return out
}

func validateCESDiscoveryRequest(req CESResourceDiscoveryRequest, cred AKSKCredential) error {
	if strings.TrimSpace(req.ProjectID) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "project_id is required")
	}
	if strings.TrimSpace(req.Region) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "region is required")
	}
	if strings.TrimSpace(cred.AccessKey) == "" || strings.TrimSpace(cred.SecretKey) == "" {
		return apperr.New(apperr.CodeFailedPrecondition, "huawei ak/sk credential is required")
	}
	return nil
}

// ptrCESLimit 将 int32 转为 SDK 要求的 *string。
func ptrCESLimit(n int32) *string {
	s := fmt.Sprintf("%d", n)
	return &s
}

// mapCESDiscoveryError 统一映射 CES v2 资源分组 API 错误，脱敏后返回。
func mapCESDiscoveryError(ctx context.Context, err error, region, op string) error {
	mapped := mapCESError(err)
	fields := []logger.Field{
		logger.String("region", region),
		logger.String("op", op),
		logger.String("error_code", string(apperr.CodeOf(mapped))),
	}
	var svcErr *sdkerr.ServiceResponseError
	if errors.As(err, &svcErr) {
		fields = append(fields,
			logger.Int("huawei_status", svcErr.StatusCode),
			logger.String("huawei_error_code", strings.TrimSpace(svcErr.ErrorCode)),
		)
	}
	logger.From(ctx).Warn("huawei ces resource discovery failed", fields...)
	return mapped
}

func callCESWithRetry[T any](ctx context.Context, region, op string, call func() (*T, error)) (*T, error) {
	var lastErr error
	for attempt := 0; attempt <= len(cesRetryDelays); attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, mapCESError(err)
		}
		resp, err := call()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryableCESError(err) || attempt == len(cesRetryDelays) {
			break
		}
		logger.From(ctx).Warn("huawei ces resource discovery retry",
			logger.String("region", region),
			logger.String("op", op),
			logger.Int("attempt", attempt+1),
			logger.String("error_code", string(apperr.CodeOf(mapCESError(err)))),
		)
		timer := time.NewTimer(cesRetryDelays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, mapCESError(ctx.Err())
		case <-timer.C:
		}
	}
	return nil, mapCESDiscoveryError(ctx, lastErr, region, op)
}

func isRetryableCESError(err error) bool {
	if err == nil {
		return false
	}
	var svcErr *sdkerr.ServiceResponseError
	if errors.As(err, &svcErr) {
		return svcErr.StatusCode == 429 || svcErr.StatusCode == 502 || svcErr.StatusCode == 503 || svcErr.StatusCode == 504
	}
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func newCESv2Client(cred AKSKCredential, projectID, region string) (*cesv2.CesClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint, err := buildEndpoint("ces", region)
	if err != nil {
		return nil, err
	}
	client := cesv2.NewCesClient(
		cesv2.CesClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultCESResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei ces v2 client failed")
	}
	return client, nil
}
