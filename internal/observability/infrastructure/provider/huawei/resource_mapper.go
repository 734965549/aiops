package huawei

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	ccemodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cce/v3/model"
	dcsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2/model"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	elbmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/model"
	evsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2/model"
	kafkamodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2/model"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	rocketmqmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rocketmq/v2/model"
	vpcmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"

	"github.com/734965549/aiops/internal/observability/domain"
)

func mapECSServer(region string, srv ecsmodel.ServerDetail) domain.CloudResource {
	id := strings.TrimSpace(srv.Id)
	name := strings.TrimSpace(srv.Name)
	if name == "" {
		name = id
	}
	status := strings.TrimSpace(srv.Status)
	labels := map[string]string{
		"instance_id": id,
		"host_id":     strings.TrimSpace(srv.HostId),
	}
	if ip := ecsPrivateIP(srv.Addresses); ip != "" {
		labels["private_ip"] = ip
	}
	if srv.Flavor != nil {
		if flavor := strings.TrimSpace(srv.Flavor.Name); flavor != "" {
			labels["flavor"] = flavor
		} else if flavor := strings.TrimSpace(srv.Flavor.Id); flavor != "" {
			labels["flavor"] = flavor
		}
	}
	if vpcID := strings.TrimSpace(srv.Metadata["vpc_id"]); vpcID != "" {
		labels["vpc_id"] = vpcID
	}
	if az := strings.TrimSpace(srv.OSEXTAZavailabilityZone); az != "" {
		labels["az"] = az
	}
	return domain.CloudResource{
		ResourceID:  "ecs-" + id,
		Name:        name,
		Type:        "ecs",
		Region:      region,
		Status:      status,
		ProviderRef: id,
		Labels:      labels,
	}
}

// ecsPrivateIP 从 ECS Addresses 中提取首个固定 IPv4 地址，用于 hybrid enrichment。
func ecsPrivateIP(addresses map[string][]ecsmodel.ServerAddress) string {
	for _, addrs := range addresses {
		for _, a := range addrs {
			if a.Version == "4" && a.Addr != "" {
				return a.Addr
			}
		}
	}
	return ""
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
	if len(inst.PrivateIps) > 0 {
		if ip := strings.TrimSpace(inst.PrivateIps[0]); ip != "" {
			labels["private_ip"] = ip
		}
	}
	if vpcID := strings.TrimSpace(inst.VpcId); vpcID != "" {
		labels["vpc_id"] = vpcID
	}
	if subnetID := strings.TrimSpace(inst.SubnetId); subnetID != "" {
		labels["subnet_id"] = subnetID
	}
	if flavor := strings.TrimSpace(inst.FlavorRef); flavor != "" {
		labels["flavor"] = flavor
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

// mapEVSVolume 将 EVS VolumeDetail 映射为 CloudResource，用于 hybrid enrichment。
// ProviderRef 取 volume name（对齐 CES SYS.EVS 的 dim_name=disk_name）；增强 label 含
// volume_id/volume_type/size_gb/attached_to/az/created_at/charging_mode。
// charging_mode 从 Metadata.orderID 推断：有值=包周期(prepaid)，无值=按需(postpaid)。
func mapEVSVolume(region string, vol evsmodel.VolumeDetail) domain.CloudResource {
	id := strings.TrimSpace(vol.Id)
	name := strings.TrimSpace(vol.Name)
	if name == "" {
		name = id
	}
	labels := map[string]string{
		"volume_id":   id,
		"volume_type": strings.TrimSpace(vol.VolumeType),
		"size_gb":     strconvI32(vol.Size),
		"status":      strings.TrimSpace(vol.Status),
	}
	if az := strings.TrimSpace(vol.AvailabilityZone); az != "" {
		labels["az"] = az
	}
	if createdAt := strings.TrimSpace(vol.CreatedAt); createdAt != "" {
		labels["created_at"] = createdAt
	}
	if len(vol.Attachments) > 0 {
		if sid := strings.TrimSpace(vol.Attachments[0].ServerId); sid != "" {
			labels["attached_to"] = sid
		}
	}
	labels["charging_mode"] = evsChargingMode(vol.Metadata)
	return domain.CloudResource{
		ResourceID:  "evs-" + id,
		Name:        name,
		Type:        "evs",
		Region:      region,
		Status:      strings.TrimSpace(vol.Status),
		ProviderRef: name,
		Labels:      labels,
	}
}

// evsChargingMode 从 EVS Metadata 推断计费模式：orderID 非空=包周期，否则按需。
func evsChargingMode(meta map[string]interface{}) string {
	if meta != nil {
		if v, ok := meta["orderID"]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" && s != "<nil>" {
				return "prepaid"
			}
		}
	}
	return "postpaid"
}

// mapVPC 将 VPC 映射为 CloudResource，用于 hybrid enrichment。
// ProviderRef 取 vpc id（对齐 CES SYS.VPC 的 dim_name=vpc_id）；增强 label 含
// vpc_name/cidr/status/enterprise_project_id/created_at。az 不在 VPC 模型中（省略），
// subnet_count 由 listVPC 调用 ListSubnets 统计后回填。
func mapVPC(region string, v vpcmodel.Vpc, subnetCount int) domain.CloudResource {
	id := strings.TrimSpace(v.Id)
	name := strings.TrimSpace(v.Name)
	if name == "" {
		name = id
	}
	labels := map[string]string{
		"vpc_name": name,
		"cidr":     strings.TrimSpace(v.Cidr),
		"status":   v.Status.Value(),
	}
	if ep := strings.TrimSpace(v.EnterpriseProjectId); ep != "" {
		labels["enterprise_project_id"] = ep
	}
	if v.CreatedAt != nil {
		labels["created_at"] = (time.Time)(*v.CreatedAt).Format("2006-01-02T15:04:05")
	}
	return domain.CloudResource{
		ResourceID:  "vpc-" + id,
		Name:        name,
		Type:        "vpc",
		Region:      region,
		Status:      v.Status.Value(),
		ProviderRef: id,
		Labels:      labels,
	}
}

// mapDCSInstance 将 DCS 实例映射为 CloudResource，用于 hybrid enrichment。
// ProviderRef 取 instance_id（对齐 CES SYS.DCS 的 dim_name=dcs_instance_id）；增强 label 含
// instance_name/engine/engine_version/capacity_gb/spec_code/private_ip/az/vpc_id/charging_mode/created_at。
func mapDCSInstance(region string, inst dcsmodel.InstanceListInfo) domain.CloudResource {
	id := ptrString(inst.InstanceId)
	name := ptrString(inst.Name)
	if name == "" {
		name = id
	}
	labels := map[string]string{
		"instance_name": name,
	}
	setLabel(labels, "engine", ptrString(inst.Engine))
	setLabel(labels, "engine_version", ptrString(inst.EngineVersion))
	setLabel(labels, "capacity_gb", ptrI32String(inst.Capacity))
	setLabel(labels, "spec_code", ptrString(inst.SpecCode))
	setLabel(labels, "private_ip", ptrString(inst.Ip))
	setLabel(labels, "vpc_id", ptrString(inst.VpcId))
	setLabel(labels, "charging_mode", dcsChargingMode(inst.ChargingMode))
	setLabel(labels, "created_at", ptrString(inst.CreatedAt))
	if inst.AzCodes != nil && len(*inst.AzCodes) > 0 {
		setLabel(labels, "az", strings.Join(*inst.AzCodes, ","))
	}
	return domain.CloudResource{
		ResourceID:  "dcs-" + id,
		Name:        name,
		Type:        "dcs",
		Region:      region,
		Status:      ptrString(inst.Status),
		ProviderRef: id,
		Labels:      labels,
	}
}

// mapDMSKafkaInstance 将 DMS Kafka 实例映射为 CloudResource，cloud_resource_type=dms。
// ProviderRef 取 instance_id（对齐 CES SYS.DMS 的 dim_name=dms_instance_id）；增强 label 含
// instance_name/engine/engine_version/spec_code/capacity_gb/private_ip/vpc_id/charging_mode/created_at。
// Kafka List 响应无 az 字段（省略）。
func mapDMSKafkaInstance(region string, inst kafkamodel.ShowInstanceResp) domain.CloudResource {
	id := ptrString(inst.InstanceId)
	name := ptrString(inst.Name)
	if name == "" {
		name = id
	}
	labels := map[string]string{
		"instance_name": name,
	}
	setLabel(labels, "engine", ptrString(inst.Engine))
	setLabel(labels, "engine_version", ptrString(inst.EngineVersion))
	setLabel(labels, "spec_code", ptrString(inst.ResourceSpecCode))
	setLabel(labels, "capacity_gb", ptrI32String(inst.StorageSpace))
	setLabel(labels, "private_ip", ptrString(inst.ConnectAddress))
	setLabel(labels, "vpc_id", ptrString(inst.VpcId))
	setLabel(labels, "charging_mode", dmsChargingMode(inst.ChargingMode))
	setLabel(labels, "created_at", ptrString(inst.CreatedAt))
	return domain.CloudResource{
		ResourceID:  "dms-kafka-" + id,
		Name:        name,
		Type:        "dms",
		Region:      region,
		Status:      ptrString(inst.Status),
		ProviderRef: id,
		Labels:      labels,
	}
}

// mapDMSRocketMQInstance 将 DMS RocketMQ 实例映射为 CloudResource，cloud_resource_type=dms。
// ProviderRef 取 instance_id；增强 label 含 instance_name/engine/engine_version/spec_code/
// capacity_gb/az/vpc_id/charging_mode/created_at。RocketMQ List 响应无 connect_address（private_ip 省略）。
func mapDMSRocketMQInstance(region string, inst rocketmqmodel.InstanceDetail) domain.CloudResource {
	id := ptrString(inst.InstanceId)
	name := ptrString(inst.Name)
	if name == "" {
		name = id
	}
	labels := map[string]string{
		"instance_name": name,
	}
	setLabel(labels, "engine", ptrString(inst.Engine))
	setLabel(labels, "engine_version", ptrString(inst.EngineVersion))
	setLabel(labels, "spec_code", ptrString(inst.Specification))
	setLabel(labels, "capacity_gb", ptrI32String(inst.StorageSpace))
	setLabel(labels, "vpc_id", ptrString(inst.VpcId))
	setLabel(labels, "charging_mode", dmsChargingMode(inst.ChargingMode))
	setLabel(labels, "created_at", ptrString(inst.CreatedAt))
	if inst.AvailableZoneNames != nil && len(*inst.AvailableZoneNames) > 0 {
		setLabel(labels, "az", strings.Join(*inst.AvailableZoneNames, ","))
	}
	return domain.CloudResource{
		ResourceID:  "dms-rocketmq-" + id,
		Name:        name,
		Type:        "dms",
		Region:      region,
		Status:      ptrString(inst.Status),
		ProviderRef: id,
		Labels:      labels,
	}
}

// setLabel 仅在 value 非空时写入 label，避免空值覆盖。
func setLabel(labels map[string]string, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		labels[key] = v
	}
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func ptrI32String(p *int32) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(int(*p))
}

func strconvI32(v int32) string {
	return strconv.Itoa(int(v))
}

// dcsChargingMode DCS：0=按需(postpaid)，1=包年/包月(prepaid)。
func dcsChargingMode(p *int32) string {
	if p == nil {
		return ""
	}
	if *p == 1 {
		return "prepaid"
	}
	return "postpaid"
}

// dmsChargingMode Kafka/RocketMQ：0=包年/包月(prepaid)，1=按需(postpaid)。
// 注意：与 DCS 的 0/1 语义相反。
func dmsChargingMode(p *int32) string {
	if p == nil {
		return ""
	}
	if *p == 0 {
		return "prepaid"
	}
	return "postpaid"
}
