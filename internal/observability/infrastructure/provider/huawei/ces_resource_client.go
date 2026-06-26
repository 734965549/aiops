package huawei

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

// CESResourceDiscoveryRequest CES 资源全量发现请求，见 docs/huawei-ces-asset-sync-plan.md §7.1。
type CESResourceDiscoveryRequest struct {
	ProjectID           string
	Region              string
	EnterpriseProjectID string
	ResourceGroupName   string
	ResourceGroupID     string
	MaxResources        int
}

// CESResourceDiscoverySummary 记录单次发现的逐 scope 摘要，用于 batch message 排查，见 §7.1。
type CESResourceDiscoverySummary struct {
	ProjectID             string
	Region                string
	ResourceGroupID       string
	ResourceGroupName     string
	CESTotal              int
	Discovered            int
	FailedScopes          []string
	SuccessfulTypes       []string
	UnknownNamespaceCount int
	InvalidResourceCount  int
	ProductNamesEmpty     bool
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

// ListCESResources 按 docs/huawei-ces-asset-sync-plan.md §8.1 执行 CES 资源全量发现：
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
		maxResources = defaultMaxResources
	}

	group, err := selectResourceGroup(ctx, api, req)
	if err != nil {
		return nil, err
	}
	summary.ResourceGroupID = group.GroupID
	summary.ResourceGroupName = group.GroupName
	if group.Total > 0 {
		summary.CESTotal = group.Total
	}

	products, productNamesEmpty, err := resolveProducts(ctx, api, group.GroupID)
	if err != nil {
		return nil, err
	}
	summary.ProductNamesEmpty = productNamesEmpty

	result := &CESResourceDiscoveryResult{Summary: summary}
	resources := make([]domain.CloudResource, 0, min(maxResources, 1024))
	for _, product := range products {
		if len(resources) >= maxResources {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, mapCESError(err)
		}
		pageResources, pageErr := listResourcesForProduct(ctx, api, group.GroupID, product, req.Region, maxResources-len(resources))
		if pageErr != nil {
			result.Summary.FailedScopes = append(result.Summary.FailedScopes,
				fmt.Sprintf("%s/%s: %s", req.Region, product.Service, apperr.FromError(pageErr).Message))
			logger.From(ctx).Warn("huawei ces list resources failed",
				logger.String("region", req.Region),
				logger.String("namespace", product.Service),
				logger.String("dim_name", product.DimName),
				logger.String("error_code", string(apperr.CodeOf(pageErr))),
			)
			continue
		}
		resourceType := resolveNamespaceMapping(product.Service).CloudResourceType
		result.Summary.SuccessfulTypes = appendUniqueString(result.Summary.SuccessfulTypes, resourceType)
		for _, in := range pageResources {
			if len(resources) >= maxResources {
				break
			}
			cloud, ok := mapCESResource(req.Region, product.Service, product.DimName, in, group.GroupID, group.GroupName)
			if !ok {
				result.Summary.InvalidResourceCount++
				continue
			}
			if isUnknownNamespace(product.Service) {
				result.Summary.UnknownNamespaceCount++
			}
			resources = append(resources, cloud)
		}
	}
	result.Resources = resources
	result.Summary.Discovered = len(resources)
	result.Summary.ResourceGroupID = group.GroupID
	result.Summary.ResourceGroupName = group.GroupName
	result.Summary.CESTotal = group.Total
	result.Summary.ProductNamesEmpty = productNamesEmpty
	return result, nil
}

// selectedResourceGroup 选中的资源组摘要。
type selectedResourceGroup struct {
	GroupID   string
	GroupName string
	Total     int
}

// selectResourceGroup 按 §8.4 选择目标资源组：指定ID > 指定名称匹配 > total 最大。
func selectResourceGroup(ctx context.Context, api cesResourceGroupAPI, req CESResourceDiscoveryRequest) (selectedResourceGroup, error) {
	if id := strings.TrimSpace(req.ResourceGroupID); id != "" {
		// 指定 ID 时仍走 ShowResourceGroup 校验存在并取 total。
		group, err := showGroup(ctx, api, id)
		if err != nil {
			return selectedResourceGroup{}, err
		}
		return selectedResourceGroup{GroupID: id, GroupName: group.GroupName, Total: group.Total}, nil
	}

	listReq := &cesv2model.ListResourceGroupsRequest{}
	if ep := strings.TrimSpace(req.EnterpriseProjectID); ep != "" {
		listReq.EnterpriseProjectId = &ep
	}
	wantName := strings.TrimSpace(req.ResourceGroupName)
	if wantName != "" {
		listReq.GroupName = &wantName
	}
	resp, err := api.ListResourceGroups(listReq)
	if err != nil {
		return selectedResourceGroup{}, mapCESDiscoveryError(ctx, err, req.Region, "list resource groups")
	}
	if resp == nil || resp.ResourceGroups == nil || len(*resp.ResourceGroups) == 0 {
		return selectedResourceGroup{}, apperr.New(apperr.CodeNotFound, "no CES resource group found for project/region")
	}

	groups := *resp.ResourceGroups
	// 名称精确匹配优先（兼容 "全部资源"/"All Resources" 大小写差异）。
	if wantName != "" {
		for _, g := range groups {
			if strings.EqualFold(strings.TrimSpace(g.GroupName), wantName) {
				return selectedResourceGroup{GroupID: g.GroupId, GroupName: g.GroupName, Total: groupTotal(g.ResourceStatistics)}, nil
			}
		}
	}
	// 否则选 total 最大的资源组，见 §8.4。
	var best selectedResourceGroup
	bestIdx := -1
	for i, g := range groups {
		total := groupTotal(g.ResourceStatistics)
		if bestIdx == -1 || total > best.Total {
			best = selectedResourceGroup{GroupID: g.GroupId, GroupName: g.GroupName, Total: total}
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return selectedResourceGroup{}, apperr.New(apperr.CodeNotFound, "no CES resource group found for project/region")
	}
	return best, nil
}

// shownResourceGroup ShowResourceGroup 结果摘要。
type shownResourceGroup struct {
	GroupName    string
	ProductNames string
	Total        int
}

func showGroup(ctx context.Context, api cesResourceGroupAPI, groupID string) (shownResourceGroup, error) {
	resp, err := api.ShowResourceGroup(&cesv2model.ShowResourceGroupRequest{GroupId: groupID})
	if err != nil {
		return shownResourceGroup{}, mapCESDiscoveryError(ctx, err, "", "show resource group")
	}
	out := shownResourceGroup{}
	if resp != nil {
		if resp.GroupName != nil {
			out.GroupName = strings.TrimSpace(*resp.GroupName)
		}
		if resp.ProductNames != nil {
			out.ProductNames = strings.TrimSpace(*resp.ProductNames)
		}
		if resp.ResourceStatistics != nil && resp.ResourceStatistics.Total != nil {
			out.Total = int(*resp.ResourceStatistics.Total)
		}
	}
	return out, nil
}

// resolveProducts 解析 product_names；为空时回落到内置白名单，见 §8.5。
func resolveProducts(ctx context.Context, api cesResourceGroupAPI, groupID string) ([]cesProduct, bool, error) {
	group, err := showGroup(ctx, api, groupID)
	if err != nil {
		return nil, false, err
	}
	products := parseProductNames(group.ProductNames)
	if len(products) == 0 {
		return fallbackProducts(), true, nil
	}
	return products, false, nil
}

// listResourcesForProduct 按 service+dim_name 分页拉取资源，见 §8.6。
func listResourcesForProduct(ctx context.Context, api cesResourceGroupAPI, groupID string, product cesProduct, region string, remaining int) ([]cesResourceInput, error) {
	pageLimit := int32(defaultCESPageLimit)
	if int(pageLimit) > remaining {
		pageLimit = int32(remaining)
	}
	out := make([]cesResourceInput, 0, min(int(pageLimit), remaining))
	offset := int32(0)
	for len(out) < remaining {
		if err := ctx.Err(); err != nil {
			return nil, mapCESError(err)
		}
		req := &cesv2model.ListResourceGroupsServicesResourcesRequest{
			GroupId: groupID,
			Service: product.Service,
			Offset:  &offset,
			Limit:   ptrCESLimit(pageLimit),
		}
		if dimName := strings.TrimSpace(product.DimName); dimName != "" {
			req.DimName = &dimName
		}
		resp, err := api.ListResourceGroupsServicesResources(req)
		if err != nil {
			return nil, mapCESDiscoveryError(ctx, err, region, "list resource group services resources")
		}
		if resp == nil || resp.Resources == nil || len(*resp.Resources) == 0 {
			break
		}
		for _, res := range *resp.Resources {
			out = append(out, toCESResourceInput(res))
			if len(out) >= remaining {
				break
			}
		}
		if int32(len(*resp.Resources)) < pageLimit {
			break
		}
		offset += pageLimit
	}
	return out, nil
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

func newCESv2Client(cred AKSKCredential, projectID, region string) (*cesv2.CesClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://ces.%s.myhuaweicloud.com", region)
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
