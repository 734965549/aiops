package huawei

import (
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
)

// cesProduct 表示从 product_names 解析出的单个云产品维度定义，见 docs/huawei-ces-asset-sync-plan.md §8.5。
type cesProduct struct {
	Service string // CES namespace，保留大写原值用于查询，例如 SYS.ECS
	DimName string // 服务首层维度名称，例如 instance_id
}

// cesResourceInput 是 CES 资源列表项的 provider 无关中间结构，
// 由 ces_resource_client.go 从 SDK GetResourceGroupResources 转换而来，
// 供 mapper 纯函数处理，避免 mapper 直接依赖厂商 SDK。
type cesResourceInput struct {
	Status             string
	EventStatus        string
	ResourceName       string
	Dimensions         []cesDimInput
	EnterpriseProjectID string
}

// cesDimInput CES 资源维度。
type cesDimInput struct {
	Name  string
	Value string
}

// namespaceMapping 对应 docs/huawei-ces-asset-sync-plan.md §9.3，CES namespace -> 平台类型。
type namespaceMapping struct {
	CloudResourceType string // 平台 cloud_resource_type，例如 ecs
	ResourceType      string // 平台 resource_type，例如 host
}

var cesNamespaceMappings = map[string]namespaceMapping{
	"SYS.ECS":   {CloudResourceType: "ecs", ResourceType: "host"},
	"SYS.EVS":   {CloudResourceType: "evs", ResourceType: "storage"},
	"SYS.VPC":   {CloudResourceType: "vpc", ResourceType: "network"},
	"SYS.ELB":   {CloudResourceType: "elb", ResourceType: "service"},
	"SYS.RDS":   {CloudResourceType: "rds", ResourceType: "database"},
	"SYS.OBS":   {CloudResourceType: "obs", ResourceType: "storage"},
	"SYS.DCS":   {CloudResourceType: "dcs", ResourceType: "middleware"},
	"SYS.DMS":   {CloudResourceType: "dms", ResourceType: "middleware"},
	"SYS.CCE":   {CloudResourceType: "cce", ResourceType: "service"},
	"SYS.CBR":   {CloudResourceType: "cbr", ResourceType: "backup"},
	"SYS.VPCEP": {CloudResourceType: "vpcep", ResourceType: "network"},
	"SYS.NAT":   {CloudResourceType: "nat", ResourceType: "network"},
	"SYS.SFS":   {CloudResourceType: "sfs", ResourceType: "storage"},
	"SYS.APM":   {CloudResourceType: "apm", ResourceType: "service"},
	"SYS.CES":   {CloudResourceType: "ces", ResourceType: "monitor"},
}

// fallbackCESDimensionWhitelist 对应 docs/huawei-ces-asset-sync-plan.md §8.5，
// 当 ShowResourceGroup 的 product_names 为空时用作兜底发现维度。
var fallbackCESProducts = []cesProduct{
	{Service: "SYS.ECS", DimName: "instance_id"},
	{Service: "SYS.EVS", DimName: "disk_name"},
	{Service: "SYS.VPC", DimName: "vpc_id"},
	{Service: "SYS.ELB", DimName: "loadbalancer_id"},
	{Service: "SYS.RDS", DimName: "rds_cluster_id"},
	{Service: "SYS.OBS", DimName: "bucket_name"},
	{Service: "SYS.DCS", DimName: "dcs_instance_id"},
	{Service: "SYS.DMS", DimName: "dms_instance_id"},
	{Service: "SYS.CCE", DimName: "cluster_id"},
	{Service: "SYS.CBR", DimName: "vault_id"},
	{Service: "SYS.VPCEP", DimName: "vpcep_id"},
	{Service: "SYS.NAT", DimName: "natgw_id"},
	{Service: "SYS.SFS", DimName: "share_id"},
}

// parseProductNames 解析 ShowResourceGroup 的 product_names 字段，见 §8.5。
// 格式形如 "SYS.ECS,instance_id;SYS.EVS,disk_name"，转为 cesProduct 列表。
// 要求：去重、忽略空项、service 保留大写原值。空字符串返回 nil。
func parseProductNames(productNames string) []cesProduct {
	productNames = strings.TrimSpace(productNames)
	if productNames == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]cesProduct, 0, 8)
	for _, part := range strings.Split(productNames, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ",", 2)
		service := strings.TrimSpace(kv[0])
		if service == "" {
			continue
		}
		dimName := ""
		if len(kv) == 2 {
			dimName = strings.TrimSpace(kv[1])
		}
		key := service + "|" + dimName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cesProduct{Service: service, DimName: dimName})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// fallbackProducts 当 product_names 为空时返回内置兜底维度白名单，见 §8.5。
func fallbackProducts() []cesProduct {
	out := make([]cesProduct, len(fallbackCESProducts))
	copy(out, fallbackCESProducts)
	return out
}

// resolveNamespaceMapping 返回 namespace 对应的平台类型映射；未知 namespace 按小写兜底，见 §9.3。
func resolveNamespaceMapping(namespace string) namespaceMapping {
	if m, ok := cesNamespaceMappings[namespace]; ok {
		return m
	}
	cloudType := strings.ToLower(strings.TrimPrefix(namespace, "SYS."))
	if cloudType == "" {
		cloudType = strings.ToLower(namespace)
	}
	return namespaceMapping{CloudResourceType: cloudType, ResourceType: "service"}
}

// isUnknownNamespace 判断 namespace 是否不在已知映射表内。
func isUnknownNamespace(namespace string) bool {
	_, ok := cesNamespaceMappings[namespace]
	return !ok
}

// selectPrimaryDimension 按 §9.2 选择主维度值。
// 顺序：dim_name 匹配的 dimension > 第一个非空 dimension > resource_name；都为空返回空串（调用方丢弃）。
func selectPrimaryDimension(dimName string, in cesResourceInput) string {
	for _, dim := range in.Dimensions {
		if dimName != "" && strings.EqualFold(strings.TrimSpace(dim.Name), dimName) {
			if v := strings.TrimSpace(dim.Value); v != "" {
				return v
			}
		}
	}
	for _, dim := range in.Dimensions {
		if v := strings.TrimSpace(dim.Value); v != "" {
			return v
		}
	}
	return strings.TrimSpace(in.ResourceName)
}

// mapCESResource 将单个 CES 资源映射为平台 CloudResource，见 §9.1。
// ok=false 表示资源缺少主维度，调用方应丢弃并计入 invalid_resource_count。
func mapCESResource(region, service, dimName string, in cesResourceInput, groupID, groupName string) (domain.CloudResource, bool) {
	primary := selectPrimaryDimension(dimName, in)
	if primary == "" {
		return domain.CloudResource{}, false
	}
	mapping := resolveNamespaceMapping(service)
	name := strings.TrimSpace(in.ResourceName)
	if name == "" {
		name = primary
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = strings.TrimSpace(in.EventStatus)
	}
	labels := map[string]string{
		"namespace": service,
		"dim_name":  dimName,
	}
	if ep := strings.TrimSpace(in.EnterpriseProjectID); ep != "" {
		labels["enterprise_project_id"] = ep
	}
	if groupID != "" {
		labels["resource_group_id"] = groupID
	}
	if groupName != "" {
		labels["resource_group_name"] = groupName
	}
	return domain.CloudResource{
		ResourceID:  "ces:" + region + ":" + service + ":" + primary,
		Name:        name,
		Type:        mapping.CloudResourceType,
		Region:      region,
		Status:      status,
		ProviderRef: primary,
		Labels:      labels,
	}, true
}
