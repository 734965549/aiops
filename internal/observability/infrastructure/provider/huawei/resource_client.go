package huawei

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	cce "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3"
	ccemodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	ecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	elb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	rds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

const defaultResourceTimeout = 30 * time.Second

var supportedCloudResourceTypes = []string{"ecs", "cce", "rds", "elb"}

// ResourceDiscoveryClient 华为云资源只读发现客户端。
type ResourceDiscoveryClient interface {
	ListResources(ctx context.Context, cred AKSKCredential, projectID, region, resourceType string, limit int) ([]domain.CloudResource, error)
}

// ResourceClient 封装 ECS/CCE/RDS/ELB 只读 List API。
type ResourceClient struct{}

func NewResourceClient() *ResourceClient {
	return &ResourceClient{}
}

var _ ResourceDiscoveryClient = (*ResourceClient)(nil)

func (c *ResourceClient) ListResources(
	ctx context.Context,
	cred AKSKCredential,
	projectID, region, resourceType string,
	limit int,
) ([]domain.CloudResource, error) {
	if c == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei resource client is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	region = strings.TrimSpace(region)
	if projectID == "" || region == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "project_id and region are required")
	}
	if strings.TrimSpace(cred.AccessKey) == "" || strings.TrimSpace(cred.SecretKey) == "" {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "huawei ak/sk credential is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, mapHuaweiAPIError(err)
	}
	if limit <= 0 {
		limit = 100
	}
	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	types := supportedCloudResourceTypes
	if resourceType != "" {
		if !isSupportedCloudResourceType(resourceType) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "unsupported cloud resource type")
		}
		types = []string{resourceType}
	}

	out := make([]domain.CloudResource, 0, limit)
	for _, t := range types {
		if len(out) >= limit {
			break
		}
		remaining := limit - len(out)
		items, err := c.listByType(ctx, cred, projectID, region, t, remaining)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (c *ResourceClient) listByType(
	ctx context.Context,
	cred AKSKCredential,
	projectID, region, resourceType string,
	limit int,
) ([]domain.CloudResource, error) {
	switch resourceType {
	case "ecs":
		return c.listECS(ctx, cred, projectID, region, limit)
	case "rds":
		return c.listRDS(ctx, cred, projectID, region, limit)
	case "elb":
		return c.listELB(ctx, cred, projectID, region, limit)
	case "cce":
		return c.listCCE(ctx, cred, projectID, region, limit)
	default:
		return nil, apperr.New(apperr.CodeInvalidArgument, "unsupported cloud resource type")
	}
}

func (c *ResourceClient) listECS(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newECSClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 1000))
	offset := int32(0)
	out := make([]domain.CloudResource, 0, limit)
	for len(out) < limit {
		req := &ecsmodel.ListServersDetailsRequest{
			Limit:  &pageLimit,
			Offset: &offset,
		}
		resp, err := client.ListServersDetails(req)
		if err != nil {
			logResourceError(ctx, "ecs", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Servers == nil || len(*resp.Servers) == 0 {
			break
		}
		for _, srv := range *resp.Servers {
			out = append(out, mapECSServer(region, srv))
			if len(out) >= limit {
				break
			}
		}
		if int32(len(*resp.Servers)) < pageLimit {
			break
		}
		offset += pageLimit
	}
	return out, nil
}

func (c *ResourceClient) listRDS(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newRDSClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 100))
	offset := int32(0)
	out := make([]domain.CloudResource, 0, limit)
	for len(out) < limit {
		req := &rdsmodel.ListInstancesRequest{
			Limit:  &pageLimit,
			Offset: &offset,
		}
		resp, err := client.ListInstances(req)
		if err != nil {
			logResourceError(ctx, "rds", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Instances == nil || len(*resp.Instances) == 0 {
			break
		}
		for _, inst := range *resp.Instances {
			out = append(out, mapRDSInstance(region, inst))
			if len(out) >= limit {
				break
			}
		}
		if int32(len(*resp.Instances)) < pageLimit {
			break
		}
		offset += pageLimit
	}
	return out, nil
}

func (c *ResourceClient) listELB(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newELBClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 2000))
	out := make([]domain.CloudResource, 0, limit)
	var marker *string
	for len(out) < limit {
		req := &elbmodel.ListLoadBalancersRequest{
			Limit:  &pageLimit,
			Marker: marker,
		}
		resp, err := client.ListLoadBalancers(req)
		if err != nil {
			logResourceError(ctx, "elb", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Loadbalancers == nil || len(*resp.Loadbalancers) == 0 {
			break
		}
		for _, lb := range *resp.Loadbalancers {
			out = append(out, mapELBLoadBalancer(region, lb))
			if len(out) >= limit {
				break
			}
		}
		if resp.PageInfo == nil || resp.PageInfo.NextMarker == nil || strings.TrimSpace(*resp.PageInfo.NextMarker) == "" {
			break
		}
		next := strings.TrimSpace(*resp.PageInfo.NextMarker)
		marker = &next
	}
	return out, nil
}

func (c *ResourceClient) listCCE(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newCCEClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	resp, err := client.ListClusters(&ccemodel.ListClustersRequest{})
	if err != nil {
		logResourceError(ctx, "cce", region, err)
		return nil, mapHuaweiAPIError(err)
	}
	out := make([]domain.CloudResource, 0, limit)
	if resp == nil || resp.Items == nil {
		return out, nil
	}
	for _, cluster := range *resp.Items {
		out = append(out, mapCCECluster(region, cluster))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func newECSClient(cred AKSKCredential, projectID, region string) (*ecs.EcsClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://ecs.%s.myhuaweicloud.com", region)
	client := ecs.NewEcsClient(
		ecs.EcsClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei ecs client failed")
	}
	return client, nil
}

func newRDSClient(cred AKSKCredential, projectID, region string) (*rds.RdsClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://rds.%s.myhuaweicloud.com", region)
	client := rds.NewRdsClient(
		rds.RdsClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei rds client failed")
	}
	return client, nil
}

func newELBClient(cred AKSKCredential, projectID, region string) (*elb.ElbClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://elb.%s.myhuaweicloud.com", region)
	client := elb.NewElbClient(
		elb.ElbClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei elb client failed")
	}
	return client, nil
}

func newCCEClient(cred AKSKCredential, projectID, region string) (*cce.CceClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://cce.%s.myhuaweicloud.com", region)
	client := cce.NewCceClient(
		cce.CceClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei cce client failed")
	}
	return client, nil
}

func logResourceError(ctx context.Context, resourceType, region string, err error) {
	fields := []logger.Field{
		logger.String("resource_type", resourceType),
		logger.String("region", region),
	}
	var svcErr *sdkerr.ServiceResponseError
	if err != nil && err.Error() != "" {
		mapped := mapHuaweiAPIError(err)
		fields = append(fields, logger.String("error_code", string(apperr.CodeOf(mapped))))
	}
	if err != nil {
		var ok bool
		if svcErr, ok = err.(*sdkerr.ServiceResponseError); ok && svcErr != nil {
			fields = append(fields,
				logger.Int("huawei_status", svcErr.StatusCode),
				logger.String("huawei_error_code", strings.TrimSpace(svcErr.ErrorCode)),
			)
		}
	}
	logger.From(ctx).Warn("huawei resource list failed", fields...)
}

func mapHuaweiAPIError(err error) error {
	return mapCESError(err)
}

func isSupportedCloudResourceType(resourceType string) bool {
	for _, t := range supportedCloudResourceTypes {
		if t == resourceType {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
