package huawei

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/734965549/aiops/internal/observability/domain"
)

// cesProduct 表示从 product_names 解析出的单个云产品维度定义，见 ops/huawei-ces-sync-contract.md §8.5。
type cesProduct struct {
	Service  string   // CES namespace，保留大写原值用于查询，例如 SYS.ECS
	DimNames []string // 保持配置顺序、不做排序的维度名集合；为空表示未指定维度
}

// cesResourceInput 是 CES 资源列表项的 provider 无关中间结构，
// 由 ces_resource_client.go 从 SDK GetResourceGroupResources 转换而来，
// 供 mapper 纯函数处理，避免 mapper 直接依赖厂商 SDK。
type cesResourceInput struct {
	Status              string
	EventStatus         string
	ResourceName        string
	Dimensions          []cesDimInput
	EnterpriseProjectID string
	Tags                []cesTagInput
}

// cesDimInput CES 资源维度。
type cesDimInput struct {
	Name  string
	Value string
}

// cesTagInput CES 资源 tag，由 toCESResourceInput 从 tags JSON 字符串解析。
type cesTagInput struct {
	Key   string
	Value string
}

// namespaceMapping 对应 ops/huawei-ces-sync-contract.md §9.3，CES namespace -> 平台类型。
type namespaceMapping struct {
	CloudResourceType string // 平台 cloud_resource_type，例如 ecs
	ResourceType      string // 平台 resource_type，例如 host
}

var cesNamespaceMappings = map[string]namespaceMapping{
	"SYS.ECS":               {CloudResourceType: "ecs", ResourceType: "host"},
	"SYS.EVS":               {CloudResourceType: "evs", ResourceType: "storage"},
	"SYS.VPC":               {CloudResourceType: "vpc", ResourceType: "network"},
	"SYS.ELB":               {CloudResourceType: "elb", ResourceType: "service"},
	"SYS.RDS":               {CloudResourceType: "rds", ResourceType: "database"},
	"SYS.RDS_MYSQL_CLUSTER": {CloudResourceType: "rds", ResourceType: "database"},
	"SYS.OBS":               {CloudResourceType: "obs", ResourceType: "storage"},
	"SYS.DCS":               {CloudResourceType: "dcs", ResourceType: "middleware"},
	"SYS.DMS":               {CloudResourceType: "dms", ResourceType: "middleware"},
	"SYS.CCE":               {CloudResourceType: "cce", ResourceType: "service"},
	"SYS.CBR":               {CloudResourceType: "cbr", ResourceType: "backup"},
	"SYS.VPCEP":             {CloudResourceType: "vpcep", ResourceType: "network"},
	"SYS.NAT":               {CloudResourceType: "nat", ResourceType: "network"},
	"SYS.SFS":               {CloudResourceType: "sfs", ResourceType: "storage"},
	"SYS.APM":               {CloudResourceType: "apm", ResourceType: "service"},
	"SYS.CES":               {CloudResourceType: "ces", ResourceType: "monitor"},
}

// fallbackCESDimensionWhitelist 对应 ops/huawei-ces-sync-contract.md §8.5，
// 当 ShowResourceGroup 的 product_names 为空时用作兜底发现维度。
// 该白名单只允许兼容展示，不得提升为权威 scope，不得触发反向 stale；
// 业务语义判断请使用 isFallbackWhitelistScope / isAuthoritativeCESScope。
var fallbackCESProducts = []cesProduct{
	{Service: "SYS.ECS", DimNames: []string{"instance_id"}},
	{Service: "SYS.EVS", DimNames: []string{"disk_name"}},
	// SYS.VPC 仅保留证据较明确的兜底维度；vpc_id/peering_id 只允许在显式 product_names 中出现。
	{Service: "SYS.VPC", DimNames: []string{"publicip_id"}},
	{Service: "SYS.VPC", DimNames: []string{"bandwidth_id"}},
	{Service: "SYS.VPC", DimNames: []string{"subnet_id"}},
	{Service: "SYS.ELB", DimNames: []string{"loadbalancer_id"}},
	{Service: "SYS.RDS", DimNames: []string{"rds_cluster_id"}},
	// SYS.RDS_MYSQL_CLUSTER 是 RDS for MySQL 集群版实例的 namespace（见
	// https://support.huaweicloud.com/usermanual-rds-mysql/rds_06_0001.html），
	// 主维度 rds_cluster_id 与 SYS.RDS 一致；集群版另有 rds_instance_id 子维度，
	// 资产发现以 cluster 级别为主，避免逐节点重复入库。
	{Service: "SYS.RDS_MYSQL_CLUSTER", DimNames: []string{"rds_cluster_id"}},
	{Service: "SYS.OBS", DimNames: []string{"bucket_name"}},
	{Service: "SYS.DCS", DimNames: []string{"dcs_instance_id"}},
	{Service: "SYS.DMS", DimNames: []string{"dms_instance_id"}},
	{Service: "SYS.CCE", DimNames: []string{"cluster_id"}},
	{Service: "SYS.CBR", DimNames: []string{"instance_id"}},
	{Service: "SYS.VPCEP", DimNames: []string{"vpcep_id"}},
	{Service: "SYS.NAT", DimNames: []string{"nat_gateway_id"}},
	{Service: "SYS.SFS", DimNames: []string{"share_id"}},
}

// parseProductNames 解析 ShowResourceGroup 的 product_names 字段，见 §8.5。
// 格式形如 "SYS.ECS,instance_id;SYS.EVS,disk_name"，转为 cesProduct 列表。
// CES product_names 单项只含单个首层维度名称（"服务命名空间,首层维度名称"）；
// 同一 service 多维度须以 ";" 拆成多个 product 项，不支持单项多 dim。
// 解析时若遇单项多 dim，按单项单 dim 拆成多个 cesProduct，避免 SYS.VPC 子类型
// 映射只取首个 dim 导致错配；同时返回 multiDimDetected=true 供调用方记录 warn 日志。
// 要求：去重、忽略空项、service 保留大写原值。空字符串返回 nil。
func parseProductNames(productNames string) ([]cesProduct, bool) {
	productNames = strings.TrimSpace(productNames)
	if productNames == "" {
		return nil, false
	}
	seen := map[string]struct{}{}
	out := make([]cesProduct, 0, 8)
	multiDimDetected := false
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
		var dimNames []string
		if len(kv) == 2 {
			for _, dim := range strings.Split(strings.TrimSpace(kv[1]), ",") {
				if d := strings.TrimSpace(dim); d != "" {
					dimNames = append(dimNames, d)
				}
			}
		}
		if len(dimNames) == 0 {
			key := service + "|"
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, cesProduct{Service: service})
			continue
		}
		if len(dimNames) > 1 {
			multiDimDetected = true // 单项多 dim 属异常格式，供调用方 warn
		}
		for _, dn := range dimNames {
			key := service + "|" + dn
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, cesProduct{Service: service, DimNames: []string{dn}})
		}
	}
	if len(out) == 0 {
		return nil, multiDimDetected
	}
	return out, multiDimDetected
}

// fallbackProducts 当 product_names 为空时返回内置兜底维度白名单，见 §8.5。
func fallbackProducts() []cesProduct {
	out := make([]cesProduct, len(fallbackCESProducts))
	copy(out, fallbackCESProducts)
	return out
}

// preferredCESDimensions 是每服务优选主维度表，初始值取 fallbackCESProducts 中该服务的
// 首个维度（如 ECS→instance_id、EVS→disk_name、ELB→loadbalancer_id、OBS→bucket_name）。
// 用于 dimension 级资源组的主维度规范化：资源实际返回的 dimensions[] 中包含优选维度时
// 优先取它，缺失时才回落首个非空维度。这样同一物理资源无论来自 product 组还是 dimension
// 组，ResourceID/ProviderRef 一致，唯一键 (cloud_resource_type, cloud_resource_id, region)
// 跨组去重才能生效，见 dimension 组支持方案 §4.3。
var preferredCESDimensions = buildPreferredCESDimensions()

func buildPreferredCESDimensions() map[string]string {
	out := make(map[string]string, len(fallbackCESProducts))
	for _, p := range fallbackCESProducts {
		if len(p.DimNames) == 0 {
			continue
		}
		if _, exists := out[p.Service]; !exists {
			out[p.Service] = p.DimNames[0]
		}
	}
	return out
}

// preferredCESDimension 返回该服务的优选主维度；未配置（如 SYS.APM/SYS.CES 无兜底维度）时返回空串。
func preferredCESDimension(service string) string {
	return preferredCESDimensions[service]
}

// dimensionServiceProducts 返回 dimension 级资源组的服务枚举工作项：按已知 namespace
// 集合（cesNamespaceMappings 的全部 key）逐服务构造 cesProduct{Service: svc}，DimNames 为空
// ——查询时不带 dim_name，由服务端返回该组该服务的全量资源，映射时按资源实际 dimensions
// 推导维度，不使用 fallbackCESProducts 的固定维度（dimension 组资源自带实际维度，无需猜测），
// 见 dimension 组支持方案 §4.2。按 service 排序保证工作项顺序确定（map 遍历无序）。
func dimensionServiceProducts() []cesProduct {
	out := make([]cesProduct, 0, len(cesNamespaceMappings))
	for svc := range cesNamespaceMappings {
		out = append(out, cesProduct{Service: svc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
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

// vpcDimScopes 对应 ops/huawei-ces-sync-contract.md §9.3，SYS.VPC 按 dim_name 拆分子类型。
// CES SYS.VPC 是网络资源聚合 namespace，其主维度为 publicip_id/bandwidth_id/subnet_id/peering_id/vpc_id
// （参考 https://support.huaweicloud.com/eu/usermanual-ces/en-us_topic_0202622212.html），
// 并非单一 VPC 实体。按 dim_name 拆分为独立 cloud_resource_type，避免语义混合与 ID 碰撞。
var vpcDimScopes = map[string]namespaceMapping{
	"publicip_id":  {CloudResourceType: "eip", ResourceType: "network"},
	"bandwidth_id": {CloudResourceType: "bandwidth", ResourceType: "network"},
	"subnet_id":    {CloudResourceType: "subnet", ResourceType: "network"},
	"peering_id":   {CloudResourceType: "peering", ResourceType: "network"},
	"vpc_id":       {CloudResourceType: "vpc", ResourceType: "network"},
}

// resolveNamespaceMappingByDim 在 resolveNamespaceMapping 基础上感知 dim_name。
// 仅 SYS.VPC 按 dim_name 拆分子类型；其它 namespace 忽略 dim_name 走原映射。
// 未知 dim_name（SYS.VPC 下未在 vpcDimScopes 列出）兜底为 vpc，保守保留实体语义。
func resolveNamespaceMappingByDim(namespace string, dimNames []string) namespaceMapping {
	if namespace == "SYS.VPC" {
		for _, dimName := range dimNames {
			if m, ok := vpcDimScopes[strings.ToLower(strings.TrimSpace(dimName))]; ok {
				return m
			}
		}
		return cesNamespaceMappings["SYS.VPC"]
	}
	return resolveNamespaceMapping(namespace)
}

// isUnknownNamespace 判断 namespace 是否不在已知映射表内。
func isUnknownNamespace(namespace string) bool {
	_, ok := cesNamespaceMappings[namespace]
	return !ok
}

// selectPrimaryDimension 按 §9.2 选择主维度值。
// 顺序：dim_name 匹配的 dimension > 第一个非空 dimension > resource_name；都为空返回空串（调用方丢弃）。
func selectPrimaryDimension(dimNames []string, in cesResourceInput) string {
	for _, want := range dimNames {
		for _, dim := range in.Dimensions {
			if strings.EqualFold(strings.TrimSpace(dim.Name), want) {
				if v := strings.TrimSpace(dim.Value); v != "" {
					return v
				}
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

// CES tag -> label 转换的限制常量，见 ops/huawei-ces-sync-contract.md §9.1。
const (
	maxCESTagsPerResource = 20  // 单资源最多保留的 tag 数量
	maxCESTagKeyLen       = 128 // tag key 截断长度
	maxCESTagValueLen     = 256 // tag value 截断长度
)

// cesTagSensitiveKeyPattern 匹配敏感 tag key，命中则丢弃该 tag 不写入 labels。
// 与前端 sensitiveLabelKeyPattern 口径一致，见 web/src/views/assets/composables/assetUtils.ts。
var cesTagSensitiveKeyPattern = regexp.MustCompile(`(?i)(secret|token|password|passwd|pwd|key|authorization|credential)`)

// parseCESResourceTagsJSON 解析 CES 资源 tags JSON 字符串（格式 {"key":"value"}，见 §6 resources[].tags）。
// 解析失败或空串返回 nil，tags 为 best-effort 字段，不阻断同步。
func parseCESResourceTagsJSON(raw string) []cesTagInput {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	tags := make([]cesTagInput, 0, len(m))
	for k, v := range m {
		tags = append(tags, cesTagInput{Key: k, Value: v})
	}
	// 按 key 排序后截断，避免 Go map 无序遍历导致每次保留的 20 个 tag 不确定（label 抖动）。
	sort.Slice(tags, func(i, j int) bool { return tags[i].Key < tags[j].Key })
	return tags
}

// applyCESTagsToLabels 将 CES tags 转为 tag.<key> label，附带数量/长度限制和敏感 key 过滤，见 §9.1。
func applyCESTagsToLabels(labels map[string]string, tags []cesTagInput) {
	count := 0
	for _, tag := range tags {
		if count >= maxCESTagsPerResource {
			break
		}
		key := strings.TrimSpace(tag.Key)
		if key == "" {
			continue
		}
		if cesTagSensitiveKeyPattern.MatchString(key) {
			continue
		}
		if utf8.RuneCountInString(key) > maxCESTagKeyLen {
			key = string([]rune(key)[:maxCESTagKeyLen])
		}
		value := strings.TrimSpace(tag.Value)
		if utf8.RuneCountInString(value) > maxCESTagValueLen {
			value = string([]rune(value)[:maxCESTagValueLen])
		}
		labels["tag."+key] = value
		count++
	}
}

// effectiveDimensionNames 计算映射用的维度序列（effective）与 labels.dim_name 用的实际维度名（actual）。
// product 路径（查询维度 dimNames 非空）：两者均为查询维度，行为与现状完全一致。
// dimension 路径（dimNames 为空，查询不带 dim_name）：actual 取资源实际返回的 dimensions[]
// 维度名（按返回顺序、去空）；effective 在优选维度命中实际维度时将其前置，使
// selectPrimaryDimension / resolveNamespaceMappingByDim 优先按优选维度取值与拆分子类型，
// 保证同一物理资源在 product 组与 dimension 组得到一致的主维度与 cloud_resource_type，
// 见 dimension 组支持方案 §4.3。
func effectiveDimensionNames(service string, dimNames []string, in cesResourceInput) (effective, actual []string) {
	if len(dimNames) > 0 {
		return dimNames, dimNames
	}
	for _, dim := range in.Dimensions {
		if name := strings.TrimSpace(dim.Name); name != "" {
			actual = append(actual, name)
		}
	}
	effective = actual
	preferred := preferredCESDimension(service)
	if preferred == "" {
		return effective, actual
	}
	for i, name := range actual {
		if strings.EqualFold(name, preferred) {
			// 优选维度前置，其余实际维度保持返回顺序（去掉重复的优选维度项）。
			effective = append([]string{name}, actual[:i]...)
			effective = append(effective, actual[i+1:]...)
			break
		}
	}
	return effective, actual
}

// mapCESResource 将单个 CES 资源映射为平台 CloudResource，见 §9.1。
// dimNames 为查询维度：product 路径非空；dimension 路径为空，此时按资源实际 dimensions
// 推导维度并用每服务优选维度表规范化主维度，见 effectiveDimensionNames。
// ok=false 表示资源缺少主维度，调用方应丢弃并计入 invalid_resource_count。
func mapCESResource(region, service string, dimNames []string, in cesResourceInput, groupID, groupName string) (domain.CloudResource, bool) {
	effectiveDims, actualDims := effectiveDimensionNames(service, dimNames, in)
	primary := selectPrimaryDimension(effectiveDims, in)
	if primary == "" {
		return domain.CloudResource{}, false
	}
	mapping := resolveNamespaceMappingByDim(service, effectiveDims)
	name := strings.TrimSpace(in.ResourceName)
	if name == "" {
		name = primary
	}
	status := strings.TrimSpace(in.Status)
	eventStatus := strings.TrimSpace(in.EventStatus)
	cloudStatus := status
	if cloudStatus == "" {
		cloudStatus = eventStatus
	}
	labels := map[string]string{
		"namespace": service,
		"dim_name":  strings.Join(actualDims, ","),
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
	// CES 告警状态写入 labels，使 asset_resource 能持久化（CloudResource.Status 不落库），见 §9.1。
	if status != "" {
		labels["ces_status"] = status
	}
	if eventStatus != "" {
		labels["ces_event_status"] = eventStatus
	}
	if len(in.Tags) > 0 {
		applyCESTagsToLabels(labels, in.Tags)
	}
	return domain.CloudResource{
		ResourceID:  "ces:" + region + ":" + service + ":" + primary,
		Name:        name,
		Type:        mapping.CloudResourceType,
		Region:      region,
		Status:      cloudStatus,
		ProviderRef: primary,
		Labels:      labels,
	}, true
}
