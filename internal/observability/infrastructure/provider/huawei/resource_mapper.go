package huawei

import (
	"strings"

	ccemodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"

	"github.com/734965549/aiops/internal/observability/domain"
)

func mapECSServer(region string, srv ecsmodel.ServerDetail) domain.CloudResource {
	id := strings.TrimSpace(srv.Id)
	name := strings.TrimSpace(srv.Name)
	if name == "" {
		name = id
	}
	status := strings.TrimSpace(srv.Status)
	return domain.CloudResource{
		ResourceID:  "ecs-" + id,
		Name:        name,
		Type:        "ecs",
		Region:      region,
		Status:      status,
		ProviderRef: id,
		Labels: map[string]string{
			"instance_id": id,
			"host_id":     strings.TrimSpace(srv.HostId),
		},
	}
}

func mapRDSInstance(region string, inst rdsmodel.InstanceResponse) domain.CloudResource {
	id := strings.TrimSpace(inst.Id)
	name := strings.TrimSpace(inst.Name)
	if name == "" {
		name = id
	}
	labels := map[string]string{}
	if inst.Datastore != nil {
		labels["engine"] = strings.TrimSpace(inst.Datastore.Type.Value())
	}
	return domain.CloudResource{
		ResourceID:  "rds-" + id,
		Name:        name,
		Type:        "rds",
		Region:      region,
		Status:      strings.TrimSpace(inst.Status),
		ProviderRef: id,
		Labels:      labels,
	}
}

func mapELBLoadBalancer(region string, lb elbmodel.LoadBalancer) domain.CloudResource {
	id := strings.TrimSpace(lb.Id)
	name := strings.TrimSpace(lb.Name)
	if name == "" {
		name = id
	}
	status := strings.TrimSpace(lb.OperatingStatus)
	if status == "" {
		status = strings.TrimSpace(lb.ProvisioningStatus)
	}
	return domain.CloudResource{
		ResourceID:  "elb-" + id,
		Name:        name,
		Type:        "elb",
		Region:      region,
		Status:      status,
		ProviderRef: id,
	}
}

func mapCCECluster(region string, cluster ccemodel.Cluster) domain.CloudResource {
	id := ""
	name := ""
	status := ""
	if cluster.Metadata != nil {
		name = strings.TrimSpace(cluster.Metadata.Name)
		if cluster.Metadata.Uid != nil {
			id = strings.TrimSpace(*cluster.Metadata.Uid)
		}
	}
	if id == "" && name != "" {
		id = name
	}
	if cluster.Status != nil && cluster.Status.Phase != nil {
		status = strings.TrimSpace(*cluster.Status.Phase)
	}
	if name == "" {
		name = id
	}
	return domain.CloudResource{
		ResourceID:  "cce-" + id,
		Name:        name,
		Type:        "cce",
		Region:      region,
		Status:      status,
		ProviderRef: id,
	}
}

// MapCloudResourceToAssetFields 将 CloudResource 映射为 Asset 注册表字段。
func MapCloudResourceToAssetFields(cloud domain.CloudResource) (resourceType, instance string) {
	switch strings.ToLower(strings.TrimSpace(cloud.Type)) {
	case "ecs":
		return "host", cloudProviderRef(cloud)
	case "rds", "elb", "cce":
		return "service", cloudProviderRef(cloud)
	default:
		return "host", cloudProviderRef(cloud)
	}
}

func cloudProviderRef(cloud domain.CloudResource) string {
	if cloud.Labels != nil {
		if v := strings.TrimSpace(cloud.Labels["instance_id"]); v != "" {
			return v
		}
	}
	ref := strings.TrimSpace(cloud.ProviderRef)
	if ref != "" {
		return ref
	}
	return strings.TrimSpace(cloud.ResourceID)
}
