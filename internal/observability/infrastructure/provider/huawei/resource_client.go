package huawei

import (
	"context"
	"fmt"
	"strconv"
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
	dcs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2"
	dcsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2/model"
	ecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	elb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	evs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2"
	evsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2/model"
	kafka "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2"
	kafkamodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2/model"
	rds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	rocketmq "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rocketmq/v2"
	rocketmqmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rocketmq/v2/model"
	vpc "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2"
	vpcmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
)

const defaultResourceTimeout = 30 * time.Second

var supportedCloudResourceTypes = []string{"ecs", "cce", "rds", "elb", "evs", "vpc", "dcs", "dms"}

// ResourceDiscoveryClient 华为云资源只读发现客户端。
type ResourceDiscoveryClient interface {
	ListResources(ctx context.Context, cred AKSKCredential, projectID, region, resourceType string, limit int) ([]domain.CloudResource, error)
}

// ResourceClient 封装 ECS/CCE/RDS/ELB/EVS/VPC/DCS/DMS 原生只读 List API。
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
	case "evs":
		return c.listEVS(ctx, cred, projectID, region, limit)
	case "vpc":
		return c.listVPC(ctx, cred, projectID, region, limit)
	case "dcs":
		return c.listDCS(ctx, cred, projectID, region, limit)
	case "dms":
		return c.listDMS(ctx, cred, projectID, region, limit)
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

// listEVS 列举云硬盘，offset 分页。见 docs/huawei-ces-asset-sync-plan.md §8.2 EVS 增强。
func (c *ResourceClient) listEVS(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newEVSClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 1000))
	offset := int32(0)
	out := make([]domain.CloudResource, 0, limit)
	for len(out) < limit {
		req := &evsmodel.ListVolumesRequest{
			Limit:  &pageLimit,
			Offset: &offset,
		}
		resp, err := client.ListVolumes(req)
		if err != nil {
			logResourceError(ctx, "evs", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Volumes == nil || len(*resp.Volumes) == 0 {
			break
		}
		for _, vol := range *resp.Volumes {
			out = append(out, mapEVSVolume(region, vol))
			if len(out) >= limit {
				break
			}
		}
		if int32(len(*resp.Volumes)) < pageLimit {
			break
		}
		offset += pageLimit
	}
	return out, nil
}

// listVPC 列举 VPC，marker 分页；同时统计每个 VPC 的子网数量。
// 见 docs/huawei-ces-asset-sync-plan.md §8.2 VPC 增强。
func (c *ResourceClient) listVPC(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newVPCClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 2000))
	out := make([]domain.CloudResource, 0, limit)
	var marker *string
	// 收集 vpc id 列表，用于后续按 vpc_id 统计子网。
	vpcIDs := make([]string, 0, limit)
	for len(out) < limit {
		req := &vpcmodel.ListVpcsRequest{
			Limit:  &pageLimit,
			Marker: marker,
		}
		resp, err := client.ListVpcs(req)
		if err != nil {
			logResourceError(ctx, "vpc", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Vpcs == nil || len(*resp.Vpcs) == 0 {
			break
		}
		for _, v := range *resp.Vpcs {
			out = append(out, mapVPC(region, v, 0))
			vpcIDs = append(vpcIDs, v.Id)
			if len(out) >= limit {
				break
			}
		}
		last := (*resp.Vpcs)[len(*resp.Vpcs)-1].Id
		if int32(len(*resp.Vpcs)) < pageLimit {
			break
		}
		marker = &last
	}
	// 统计每个 VPC 的子网数量：一次 ListSubnets 拿全量，按 vpc_id 分组。
	// subnet_count 为可选增强；统计失败（如子网权限不足）不丢弃 VPC 结果，仅缺少 subnet_count label，
	// 失败原因已由 countSubnetsByVPC 内部 logResourceError 记录。
	subnetCount, err := countSubnetsByVPC(ctx, client, region)
	if err != nil {
		return out, nil
	}
	for i := range out {
		if cnt, ok := subnetCount[vpcIDs[i]]; ok {
			if out[i].Labels == nil {
				out[i].Labels = map[string]string{}
			}
			out[i].Labels["subnet_count"] = strconv.Itoa(cnt)
		}
	}
	return out, nil
}

// countSubnetsByVPC 全量拉取子网并按 vpc_id 计数。
func countSubnetsByVPC(ctx context.Context, client *vpc.VpcClient, region string) (map[string]int, error) {
	out := map[string]int{}
	pageLimit := int32(2000)
	var marker *string
	for {
		if err := ctx.Err(); err != nil {
			return nil, mapHuaweiAPIError(err)
		}
		resp, err := client.ListSubnets(&vpcmodel.ListSubnetsRequest{
			Limit:  &pageLimit,
			Marker: marker,
		})
		if err != nil {
			logResourceError(ctx, "vpc.subnet", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Subnets == nil || len(*resp.Subnets) == 0 {
			break
		}
		for _, s := range *resp.Subnets {
			if s.VpcId != "" {
				out[s.VpcId]++
			}
		}
		last := (*resp.Subnets)[len(*resp.Subnets)-1].Id
		if int32(len(*resp.Subnets)) < pageLimit {
			break
		}
		marker = &last
	}
	return out, nil
}

// listDCS 列举 DCS 缓存实例，offset 分页。见 docs/huawei-ces-asset-sync-plan.md §8.2 DCS 增强。
func (c *ResourceClient) listDCS(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newDCSClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 1000))
	offset := int32(0)
	out := make([]domain.CloudResource, 0, limit)
	for len(out) < limit {
		req := &dcsmodel.ListInstancesRequest{
			Limit:  &pageLimit,
			Offset: &offset,
		}
		resp, err := client.ListInstances(req)
		if err != nil {
			logResourceError(ctx, "dcs", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Instances == nil || len(*resp.Instances) == 0 {
			break
		}
		for _, inst := range *resp.Instances {
			out = append(out, mapDCSInstance(region, inst))
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

// listDMS 列举 DMS 实例。华为云 SDK 把 DMS 拆为 kafka 与 rocketmq 两个服务包，
// 这里合并两者结果，cloud_resource_type 统一为 dms。见 docs/huawei-ces-asset-sync-plan.md §8.2 DMS 增强。
// Kafka 与 RocketMQ 是独立子服务：任一失败不应阻断另一个，也不应丢弃已获取结果。
// 失败原因已由子服务内部 logResourceError 记录；只有两者都失败才返回错误。
func (c *ResourceClient) listDMS(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	out := make([]domain.CloudResource, 0, limit)
	// Kafka：engine 必填，limit 最大 50，offset/limit 为 string。
	kafkaOut, kafkaErr := c.listDMSKafka(ctx, cred, projectID, region, limit)
	if kafkaErr == nil {
		out = append(out, kafkaOut...)
	}
	// RocketMQ：engine 必填，limit 最大 50，offset/limit 为 int32。
	var rocketErr error
	if len(out) < limit {
		remaining := limit - len(out)
		var rocketOut []domain.CloudResource
		rocketOut, rocketErr = c.listDMSRocketMQ(ctx, cred, projectID, region, remaining)
		if rocketErr == nil {
			out = append(out, rocketOut...)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	// 两个子服务都失败才返回错误；部分成功时返回已收集结果，避免一个子服务失败丢弃另一个的结果。
	if kafkaErr != nil && rocketErr != nil {
		return nil, kafkaErr
	}
	return out, nil
}

func (c *ResourceClient) listDMSKafka(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newKafkaClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 50))
	if pageLimit <= 0 {
		pageLimit = 50
	}
	offset := int32(0)
	out := make([]domain.CloudResource, 0, limit)
	for len(out) < limit {
		limitStr := strconv.Itoa(int(pageLimit))
		offsetStr := strconv.Itoa(int(offset))
		req := &kafkamodel.ListInstancesRequest{
			Engine: kafkamodel.GetListInstancesRequestEngineEnum().KAFKA,
			Limit:  &limitStr,
			Offset: &offsetStr,
		}
		resp, err := client.ListInstances(req)
		if err != nil {
			logResourceError(ctx, "dms.kafka", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Instances == nil || len(*resp.Instances) == 0 {
			break
		}
		for _, inst := range *resp.Instances {
			out = append(out, mapDMSKafkaInstance(region, inst))
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

func (c *ResourceClient) listDMSRocketMQ(ctx context.Context, cred AKSKCredential, projectID, region string, limit int) ([]domain.CloudResource, error) {
	client, err := newRocketMQClient(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	pageLimit := int32(min(limit, 50))
	if pageLimit <= 0 {
		pageLimit = 50
	}
	offset := int32(0)
	out := make([]domain.CloudResource, 0, limit)
	for len(out) < limit {
		req := &rocketmqmodel.ListInstancesRequest{
			Engine: rocketmqmodel.GetListInstancesRequestEngineEnum().ROCKETMQ,
			Limit:  &pageLimit,
			Offset: &offset,
		}
		resp, err := client.ListInstances(req)
		if err != nil {
			logResourceError(ctx, "dms.rocketmq", region, err)
			return nil, mapHuaweiAPIError(err)
		}
		if resp == nil || resp.Instances == nil || len(*resp.Instances) == 0 {
			break
		}
		for _, inst := range *resp.Instances {
			out = append(out, mapDMSRocketMQInstance(region, inst))
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

func newEVSClient(cred AKSKCredential, projectID, region string) (*evs.EvsClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://evs.%s.myhuaweicloud.com", region)
	client := evs.NewEvsClient(
		evs.EvsClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei evs client failed")
	}
	return client, nil
}

func newVPCClient(cred AKSKCredential, projectID, region string) (*vpc.VpcClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://vpc.%s.myhuaweicloud.com", region)
	client := vpc.NewVpcClient(
		vpc.VpcClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei vpc client failed")
	}
	return client, nil
}

func newDCSClient(cred AKSKCredential, projectID, region string) (*dcs.DcsClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://dcs.%s.myhuaweicloud.com", region)
	client := dcs.NewDcsClient(
		dcs.DcsClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei dcs client failed")
	}
	return client, nil
}

func newKafkaClient(cred AKSKCredential, projectID, region string) (*kafka.KafkaClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://dms.%s.myhuaweicloud.com", region)
	client := kafka.NewKafkaClient(
		kafka.KafkaClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei kafka client failed")
	}
	return client, nil
}

func newRocketMQClient(cred AKSKCredential, projectID, region string) (*rocketmq.RocketMQClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://dms.%s.myhuaweicloud.com", region)
	client := rocketmq.NewRocketMQClient(
		rocketmq.RocketMQClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultResourceTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei rocketmq client failed")
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
