package huawei

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/734965549/aiops/internal/observability/domain"
)

func TestParseProductNames(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     []cesProduct
		multiDim bool
	}{
		{"empty", "", nil, false},
		{"blank", "   ", nil, false},
		{"single", "SYS.ECS,instance_id", []cesProduct{{Service: "SYS.ECS", DimNames: []string{"instance_id"}}}, false},
		{"multi", "SYS.ECS,instance_id;SYS.EVS,disk_name", []cesProduct{
			{Service: "SYS.ECS", DimNames: []string{"instance_id"}},
			{Service: "SYS.EVS", DimNames: []string{"disk_name"}},
		}, false},
		{"dedup", "SYS.ECS,instance_id;SYS.ECS,instance_id", []cesProduct{{Service: "SYS.ECS", DimNames: []string{"instance_id"}}}, false},
		{"skip empty parts", ";;SYS.ECS,instance_id;;", []cesProduct{{Service: "SYS.ECS", DimNames: []string{"instance_id"}}}, false},
		{"trim spaces", " SYS.ECS , instance_id ; SYS.EVS , disk_name ", []cesProduct{
			{Service: "SYS.ECS", DimNames: []string{"instance_id"}},
			{Service: "SYS.EVS", DimNames: []string{"disk_name"}},
		}, false},
		{"missing dim_name", "SYS.ECS", []cesProduct{{Service: "SYS.ECS"}}, false},
		// CES 不返回单项多 dim；若遇到则按单项单 dim 拆成多个 product 项，见 §8.5。
		{"multi dim splits", "SYS.VPC,publicip_id,bandwidth_id", []cesProduct{
			{Service: "SYS.VPC", DimNames: []string{"publicip_id"}},
			{Service: "SYS.VPC", DimNames: []string{"bandwidth_id"}},
		}, true},
		{"multi dim dedup", "SYS.ECS,instance_id,instance_id", []cesProduct{{Service: "SYS.ECS", DimNames: []string{"instance_id"}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, multiDim := parseProductNames(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseProductNames(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
			if multiDim != tc.multiDim {
				t.Fatalf("parseProductNames(%q) multiDim = %v, want %v", tc.in, multiDim, tc.multiDim)
			}
		})
	}
}

func TestResolveNamespaceMapping_Known(t *testing.T) {
	want := map[string]namespaceMapping{
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
	for ns, w := range want {
		got := resolveNamespaceMapping(ns)
		if got != w {
			t.Fatalf("resolveNamespaceMapping(%q) = %+v, want %+v", ns, got, w)
		}
		if isUnknownNamespace(ns) {
			t.Fatalf("isUnknownNamespace(%q) = true, want false", ns)
		}
	}
}

func TestResolveNamespaceMapping_Unknown(t *testing.T) {
	got := resolveNamespaceMapping("SYS.NEW_SERVICE")
	if got.CloudResourceType != "new_service" {
		t.Fatalf("CloudResourceType = %q, want %q", got.CloudResourceType, "new_service")
	}
	if got.ResourceType != "service" {
		t.Fatalf("ResourceType = %q, want %q", got.ResourceType, "service")
	}
	if !isUnknownNamespace("SYS.NEW_SERVICE") {
		t.Fatalf("isUnknownNamespace = false, want true")
	}
}

func TestResolveNamespaceMappingByDim_VPCSplit(t *testing.T) {
	cases := []struct {
		dimName string
		want    namespaceMapping
	}{
		{"publicip_id", namespaceMapping{CloudResourceType: "eip", ResourceType: "network"}},
		{"bandwidth_id", namespaceMapping{CloudResourceType: "bandwidth", ResourceType: "network"}},
		{"subnet_id", namespaceMapping{CloudResourceType: "subnet", ResourceType: "network"}},
		{"peering_id", namespaceMapping{CloudResourceType: "peering", ResourceType: "network"}},
		{"vpc_id", namespaceMapping{CloudResourceType: "vpc", ResourceType: "network"}},
		{"VPC_ID", namespaceMapping{CloudResourceType: "vpc", ResourceType: "network"}},
		// 未知 dim 兜底为 vpc，保守保留实体语义。
		{"unknown_dim", namespaceMapping{CloudResourceType: "vpc", ResourceType: "network"}},
		{"", namespaceMapping{CloudResourceType: "vpc", ResourceType: "network"}},
	}
	for _, tc := range cases {
		got := resolveNamespaceMappingByDim("SYS.VPC", []string{tc.dimName})
		if got != tc.want {
			t.Fatalf("resolveNamespaceMappingByDim(SYS.VPC,%q) = %+v, want %+v", tc.dimName, got, tc.want)
		}
	}
}

func TestResolveNamespaceMappingByDim_NonVPCIgnoresDim(t *testing.T) {
	// 非 SYS.VPC namespace 忽略 dim_name，走原映射。
	got := resolveNamespaceMappingByDim("SYS.ECS", []string{"instance_id"})
	if got.CloudResourceType != "ecs" || got.ResourceType != "host" {
		t.Fatalf("ECS mapping = %+v, want ecs/host", got)
	}
	// dim_name 不影响非 VPC namespace。
	got2 := resolveNamespaceMappingByDim("SYS.ECS", []string{"publicip_id"})
	if got2.CloudResourceType != "ecs" {
		t.Fatalf("ECS mapping with unrelated dim = %+v, want ecs", got2)
	}
}

func TestMapCESResource_VPCSubtypeSplit(t *testing.T) {
	in := cesResourceInput{
		ResourceName: "my-eip",
		Dimensions:   []cesDimInput{{Name: "publicip_id", Value: "pub-uuid-1"}},
	}
	cloud, ok := mapCESResource("cn-south-1", "SYS.VPC", []string{"publicip_id"}, in, "rg001", "g")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Type != "eip" {
		t.Fatalf("Type = %q, want eip", cloud.Type)
	}
	if cloud.ProviderRef != "pub-uuid-1" {
		t.Fatalf("ProviderRef = %q, want pub-uuid-1", cloud.ProviderRef)
	}
	if cloud.Labels["namespace"] != "SYS.VPC" || cloud.Labels["dim_name"] != "publicip_id" {
		t.Fatalf("labels = %+v", cloud.Labels)
	}
	// 子网 dim → subnet。
	inSub := cesResourceInput{Dimensions: []cesDimInput{{Name: "subnet_id", Value: "sub-1"}}}
	cloudSub, _ := mapCESResource("r", "SYS.VPC", []string{"subnet_id"}, inSub, "", "")
	if cloudSub.Type != "subnet" {
		t.Fatalf("subnet Type = %q, want subnet", cloudSub.Type)
	}
}

func TestSelectPrimaryDimension(t *testing.T) {
	in := cesResourceInput{
		ResourceName: "fallback-name",
		Dimensions: []cesDimInput{
			{Name: "other_dim", Value: "other-val"},
			{Name: "instance_id", Value: "i-primary"},
			{Name: "empty", Value: ""},
		},
	}
	if got := selectPrimaryDimension([]string{"instance_id"}, in); got != "i-primary" {
		t.Fatalf("dim_name match = %q, want %q", got, "i-primary")
	}
	// dim_name 不匹配时取第一个非空 dimension。
	if got := selectPrimaryDimension([]string{"not_exist"}, in); got != "other-val" {
		t.Fatalf("first non-empty = %q, want %q", got, "other-val")
	}
	// 无 dimension 时取 resource_name。
	inNoDim := cesResourceInput{ResourceName: "name-only"}
	if got := selectPrimaryDimension([]string{"instance_id"}, inNoDim); got != "name-only" {
		t.Fatalf("resource_name fallback = %q, want %q", got, "name-only")
	}
	// 全空返回空串。
	inEmpty := cesResourceInput{}
	if got := selectPrimaryDimension([]string{"instance_id"}, inEmpty); got != "" {
		t.Fatalf("empty = %q, want empty", got)
	}
}

func TestMapCESResource(t *testing.T) {
	in := cesResourceInput{
		Status:              "health",
		EventStatus:         "unhealthy",
		ResourceName:        "my-ecs",
		EnterpriseProjectID: "eps-001",
		Dimensions:          []cesDimInput{{Name: "instance_id", Value: "i-12345"}},
	}
	cloud, ok := mapCESResource("cn-south-1", "SYS.ECS", []string{"instance_id"}, in, "rg001", "全部资源")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	want := domain.CloudResource{
		ResourceID:  "ces:cn-south-1:SYS.ECS:i-12345",
		Name:        "my-ecs",
		Type:        "ecs",
		Region:      "cn-south-1",
		Status:      "health",
		ProviderRef: "i-12345",
		Labels: map[string]string{
			"namespace":             "SYS.ECS",
			"dim_name":              "instance_id",
			"enterprise_project_id": "eps-001",
			"resource_group_id":     "rg001",
			"resource_group_name":   "全部资源",
			"ces_status":            "health",
			"ces_event_status":      "unhealthy",
		},
	}
	if !reflect.DeepEqual(cloud, want) {
		t.Fatalf("mapCESResource = %+v, want %+v", cloud, want)
	}
}

func TestMapCESResource_NoPrimaryDimReturnsFalse(t *testing.T) {
	in := cesResourceInput{ResourceName: "", Dimensions: nil}
	_, ok := mapCESResource("cn-south-1", "SYS.ECS", []string{"instance_id"}, in, "rg001", "g")
	if ok {
		t.Fatalf("expected ok=false when no primary dimension")
	}
}

// TestPreferredCESDimensions 验证每服务优选维度表初始值取 fallbackCESProducts 该服务首维度，
// 见 dimension 组支持方案 §4.3。
func TestPreferredCESDimensions(t *testing.T) {
	want := map[string]string{
		"SYS.ECS":               "instance_id",
		"SYS.EVS":               "disk_name",
		"SYS.VPC":               "publicip_id",
		"SYS.ELB":               "loadbalancer_id",
		"SYS.RDS":               "rds_cluster_id",
		"SYS.RDS_MYSQL_CLUSTER": "rds_cluster_id",
		"SYS.OBS":               "bucket_name",
		"SYS.DCS":               "dcs_instance_id",
		"SYS.DMS":               "dms_instance_id",
		"SYS.CCE":               "cluster_id",
		"SYS.CBR":               "instance_id",
		"SYS.VPCEP":             "vpcep_id",
		"SYS.NAT":               "nat_gateway_id",
		"SYS.SFS":               "share_id",
	}
	for svc, dim := range want {
		if got := preferredCESDimension(svc); got != dim {
			t.Fatalf("preferredCESDimension(%q) = %q, want %q", svc, got, dim)
		}
	}
	// 无兜底维度的已知 namespace 没有优选维度，回落首个非空实际维度。
	if got := preferredCESDimension("SYS.APM"); got != "" {
		t.Fatalf("preferredCESDimension(SYS.APM) = %q, want empty", got)
	}
	if got := preferredCESDimension("SYS.CES"); got != "" {
		t.Fatalf("preferredCESDimension(SYS.CES) = %q, want empty", got)
	}
}

// TestDimensionServiceProducts 验证 dimension 组服务枚举工作项：覆盖 cesNamespaceMappings
// 全部已知 namespace、不带维度、按 service 排序保证顺序确定，见 dimension 组支持方案 §4.2。
func TestDimensionServiceProducts(t *testing.T) {
	got := dimensionServiceProducts()
	if len(got) != len(cesNamespaceMappings) {
		t.Fatalf("dimensionServiceProducts count = %d, want %d", len(got), len(cesNamespaceMappings))
	}
	seen := map[string]bool{}
	for i, p := range got {
		if p.Service == "" {
			t.Fatalf("item %d has empty service", i)
		}
		if len(p.DimNames) != 0 {
			t.Fatalf("service %s has DimNames %v, want empty (dimension enumeration does not pass dim_name)", p.Service, p.DimNames)
		}
		if isUnknownNamespace(p.Service) {
			t.Fatalf("service %s is not a known namespace (enumeration must stay within the whitelist)", p.Service)
		}
		if seen[p.Service] {
			t.Fatalf("duplicate service %s", p.Service)
		}
		seen[p.Service] = true
		if i > 0 && got[i-1].Service >= p.Service {
			t.Fatalf("services not sorted: %s before %s", got[i-1].Service, p.Service)
		}
	}
}

// TestEffectiveDimensionNames_ProductPathUnchanged 验证 product 路径（查询维度非空）行为不变：
// effective 与 actual 均为查询维度，与现状完全一致。
func TestEffectiveDimensionNames_ProductPathUnchanged(t *testing.T) {
	in := cesResourceInput{Dimensions: []cesDimInput{
		{Name: "instance_id", Value: "i-1"},
		{Name: "az", Value: "az-1"},
	}}
	effective, actual := effectiveDimensionNames("SYS.ECS", []string{"instance_id"}, in)
	if !reflect.DeepEqual(effective, []string{"instance_id"}) || !reflect.DeepEqual(actual, []string{"instance_id"}) {
		t.Fatalf("effective=%v actual=%v, want [instance_id]/[instance_id]", effective, actual)
	}
}

// TestEffectiveDimensionNames_DimensionPreferredHoisted 验证 dimension 路径：优选维度命中
// 实际维度时前置，其余实际维度保持返回顺序；未命中时 effective 等于实际维度序列，
// 见 dimension 组支持方案 §4.3。
func TestEffectiveDimensionNames_DimensionPreferredHoisted(t *testing.T) {
	in := cesResourceInput{Dimensions: []cesDimInput{
		{Name: "lbaas_instance_id", Value: "x"},
		{Name: "loadbalancer_id", Value: "lb-1"},
		{Name: "az", Value: "az-1"},
	}}
	effective, actual := effectiveDimensionNames("SYS.ELB", nil, in)
	if !reflect.DeepEqual(effective, []string{"loadbalancer_id", "lbaas_instance_id", "az"}) {
		t.Fatalf("effective = %v, want [loadbalancer_id lbaas_instance_id az] (preferred hoisted)", effective)
	}
	if !reflect.DeepEqual(actual, []string{"lbaas_instance_id", "loadbalancer_id", "az"}) {
		t.Fatalf("actual = %v, want [lbaas_instance_id loadbalancer_id az] (return order)", actual)
	}
	// 优选维度缺失：effective 等于实际维度序列。
	inNoPreferred := cesResourceInput{Dimensions: []cesDimInput{{Name: "lbaas_instance_id", Value: "x"}}}
	effective2, actual2 := effectiveDimensionNames("SYS.ELB", nil, inNoPreferred)
	if !reflect.DeepEqual(effective2, []string{"lbaas_instance_id"}) || !reflect.DeepEqual(actual2, []string{"lbaas_instance_id"}) {
		t.Fatalf("effective=%v actual=%v, want [lbaas_instance_id]/[lbaas_instance_id]", effective2, actual2)
	}
	// 无维度：均为空。
	effective3, actual3 := effectiveDimensionNames("SYS.ELB", nil, cesResourceInput{})
	if len(effective3) != 0 || len(actual3) != 0 {
		t.Fatalf("effective=%v actual=%v, want empty/empty", effective3, actual3)
	}
	// 空维度名被忽略。
	inBlank := cesResourceInput{Dimensions: []cesDimInput{{Name: "  ", Value: "v"}, {Name: "instance_id", Value: "i-1"}}}
	effective4, actual4 := effectiveDimensionNames("SYS.ECS", nil, inBlank)
	if !reflect.DeepEqual(effective4, []string{"instance_id"}) || !reflect.DeepEqual(actual4, []string{"instance_id"}) {
		t.Fatalf("effective=%v actual=%v, want [instance_id]/[instance_id] (blank dim name ignored)", effective4, actual4)
	}
}

// TestMapCESResource_DimensionActualDims 验证 dimension 路径（查询维度为空）按资源实际
// dimensions 映射：主维度来自实际维度，labels.dim_name 记录实际维度名（返回顺序），
// 见 dimension 组支持方案 §4.3。
func TestMapCESResource_DimensionActualDims(t *testing.T) {
	in := cesResourceInput{
		Status:       "health",
		ResourceName: "bucket-prod",
		Dimensions: []cesDimInput{
			{Name: "bucket_name", Value: "b-1"},
			{Name: "az", Value: "az-1"},
		},
	}
	cloud, ok := mapCESResource("cn-south-1", "SYS.OBS", nil, in, "rg-dim", "大数据")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Type != "obs" {
		t.Fatalf("Type = %q, want obs", cloud.Type)
	}
	if cloud.ProviderRef != "b-1" {
		t.Fatalf("ProviderRef = %q, want b-1 (preferred bucket_name)", cloud.ProviderRef)
	}
	if cloud.ResourceID != "ces:cn-south-1:SYS.OBS:b-1" {
		t.Fatalf("ResourceID = %q", cloud.ResourceID)
	}
	if cloud.Labels["dim_name"] != "bucket_name,az" {
		t.Fatalf("dim_name label = %q, want bucket_name,az (actual dimensions in return order)", cloud.Labels["dim_name"])
	}
	if cloud.Labels["resource_group_id"] != "rg-dim" || cloud.Labels["resource_group_name"] != "大数据" {
		t.Fatalf("group labels = %+v", cloud.Labels)
	}
}

// TestMapCESResource_DimensionVPCCSubtypeFromActualDim 验证 dimension 路径 SYS.VPC 资源
// 按实际维度拆分子类型：subnet_id→subnet、bandwidth_id→bandwidth，见方案 §4.3。
func TestMapCESResource_DimensionVPCCSubtypeFromActualDim(t *testing.T) {
	sub, ok := mapCESResource("r", "SYS.VPC", nil, cesResourceInput{
		Dimensions: []cesDimInput{{Name: "subnet_id", Value: "sub-1"}},
	}, "", "")
	if !ok || sub.Type != "subnet" || sub.ProviderRef != "sub-1" {
		t.Fatalf("subnet mapping = %+v ok=%v, want subnet/sub-1", sub, ok)
	}
	bw, ok := mapCESResource("r", "SYS.VPC", nil, cesResourceInput{
		Dimensions: []cesDimInput{{Name: "bandwidth_id", Value: "bw-1"}},
	}, "", "")
	if !ok || bw.Type != "bandwidth" || bw.ProviderRef != "bw-1" {
		t.Fatalf("bandwidth mapping = %+v ok=%v, want bandwidth/bw-1", bw, ok)
	}
	// 优选维度 publicip_id 命中时映射 eip，即使返回维度里还有其它维度。
	eip, ok := mapCESResource("r", "SYS.VPC", nil, cesResourceInput{
		Dimensions: []cesDimInput{{Name: "bandwidth_id", Value: "bw-2"}, {Name: "publicip_id", Value: "pub-1"}},
	}, "", "")
	if !ok || eip.Type != "eip" || eip.ProviderRef != "pub-1" {
		t.Fatalf("eip mapping = %+v ok=%v, want eip/pub-1 (preferred dimension wins)", eip, ok)
	}
}

// TestMapCESResource_DimensionUnknownServiceDim 验证 dimension 路径对未知维度的资源仍按
// 实际维度取名（SYS.VPC 未知 dim 兜底 vpc 实体语义，非 VPC namespace 忽略 dim）。
func TestMapCESResource_DimensionUnknownServiceDim(t *testing.T) {
	cloud, ok := mapCESResource("r", "SYS.VPC", nil, cesResourceInput{
		Dimensions: []cesDimInput{{Name: "some_unknown_dim", Value: "u-1"}},
	}, "", "")
	if !ok || cloud.Type != "vpc" || cloud.ProviderRef != "u-1" {
		t.Fatalf("mapping = %+v ok=%v, want vpc/u-1", cloud, ok)
	}
}

func TestMapCESResource_UnknownNamespace(t *testing.T) {
	in := cesResourceInput{
		ResourceName: "x",
		Dimensions:   []cesDimInput{{Name: "res_id", Value: "v-1"}},
	}
	cloud, ok := mapCESResource("cn-north-4", "SYS.NEW_SERVICE", []string{"res_id"}, in, "", "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Type != "new_service" {
		t.Fatalf("Type = %q, want %q", cloud.Type, "new_service")
	}
	if cloud.Labels["namespace"] != "SYS.NEW_SERVICE" {
		t.Fatalf("namespace label not preserved")
	}
}

// TestMapCESResource_RDSMysqlCluster 验证 SYS.RDS_MYSQL_CLUSTER（RDS for MySQL 集群版）
// 正确映射为 rds/database，而非小写兜底的 rds_mysql_cluster/service。
// 官方维度：rds_cluster_id（level 0）+ rds_instance_id（level 1），见
// https://support.huaweicloud.com/usermanual-rds-mysql/rds_06_0001.html
func TestMapCESResource_RDSMysqlCluster(t *testing.T) {
	// 集群版主维度 rds_cluster_id —— 资产发现以此为主，对齐原生 RDS ListInstances 的 inst.Id。
	in := cesResourceInput{
		Status:              "health",
		ResourceName:        "mysql-cluster-01",
		EnterpriseProjectID: "eps-rds",
		Dimensions: []cesDimInput{
			{Name: "rds_cluster_id", Value: "cluster-uuid-123"},
			{Name: "rds_instance_id", Value: "node-uuid-456"},
		},
	}
	cloud, ok := mapCESResource("cn-south-1", "SYS.RDS_MYSQL_CLUSTER", []string{"rds_cluster_id"}, in, "rg001", "全部资源")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Type != "rds" {
		t.Fatalf("Type = %q, want rds", cloud.Type)
	}
	if cloud.ProviderRef != "cluster-uuid-123" {
		t.Fatalf("ProviderRef = %q, want cluster-uuid-123", cloud.ProviderRef)
	}
	if cloud.ResourceID != "ces:cn-south-1:SYS.RDS_MYSQL_CLUSTER:cluster-uuid-123" {
		t.Fatalf("ResourceID = %q", cloud.ResourceID)
	}
	if cloud.Labels["namespace"] != "SYS.RDS_MYSQL_CLUSTER" {
		t.Fatalf("namespace label = %q", cloud.Labels["namespace"])
	}
	if cloud.Labels["dim_name"] != "rds_cluster_id" {
		t.Fatalf("dim_name label = %q", cloud.Labels["dim_name"])
	}
	// 确认不是未知 namespace。
	if isUnknownNamespace("SYS.RDS_MYSQL_CLUSTER") {
		t.Fatalf("SYS.RDS_MYSQL_CLUSTER should be a known namespace")
	}
}

// TestMapCESResource_RDSMysqlCluster_InstanceDim 验证集群版子维度 rds_instance_id
// 仍映射为 rds/database（namespace 映射不因 dim_name 变化），
// 且主维度选择优先匹配 dim_name=rds_instance_id。
func TestMapCESResource_RDSMysqlCluster_InstanceDim(t *testing.T) {
	in := cesResourceInput{
		ResourceName: "mysql-cluster-node-0",
		Dimensions: []cesDimInput{
			{Name: "rds_cluster_id", Value: "cluster-uuid-123"},
			{Name: "rds_instance_id", Value: "node-uuid-456"},
		},
	}
	cloud, ok := mapCESResource("cn-south-1", "SYS.RDS_MYSQL_CLUSTER", []string{"rds_instance_id"}, in, "", "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Type != "rds" {
		t.Fatalf("Type = %q, want rds", cloud.Type)
	}
	// dim_name=rds_instance_id 时应优先选择该维度值。
	if cloud.ProviderRef != "node-uuid-456" {
		t.Fatalf("ProviderRef = %q, want node-uuid-456", cloud.ProviderRef)
	}
}

func TestMapCESResource_NameFallbackToPrimary(t *testing.T) {
	in := cesResourceInput{
		Dimensions: []cesDimInput{{Name: "instance_id", Value: "i-1"}},
	}
	cloud, ok := mapCESResource("r", "SYS.ECS", []string{"instance_id"}, in, "", "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Name != "i-1" {
		t.Fatalf("Name = %q, want %q", cloud.Name, "i-1")
	}
}

func TestParseCESResourceTagsJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []cesTagInput
	}{
		{"empty", "", nil},
		{"blank", "  ", nil},
		{"invalid json", "{not json}", nil},
		{"empty object", "{}", nil},
		{"single", `{"env":"prod"}`, []cesTagInput{{Key: "env", Value: "prod"}}},
		{"multi", `{"env":"prod","team":"ops"}`, []cesTagInput{
			{Key: "env", Value: "prod"},
			{Key: "team", Value: "ops"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCESResourceTagsJSON(tc.raw)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("parseCESResourceTagsJSON(%q) = %+v, want nil", tc.raw, got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			gotMap := map[string]string{}
			for _, tag := range got {
				gotMap[tag.Key] = tag.Value
			}
			for _, w := range tc.want {
				if gotMap[w.Key] != w.Value {
					t.Fatalf("tag %q = %q, want %q", w.Key, gotMap[w.Key], w.Value)
				}
			}
		})
	}
}

func TestApplyCESTagsToLabels(t *testing.T) {
	t.Run("normal tags", func(t *testing.T) {
		labels := map[string]string{}
		applyCESTagsToLabels(labels, []cesTagInput{
			{Key: "env", Value: "prod"},
			{Key: "team", Value: "ops"},
		})
		if labels["tag.env"] != "prod" {
			t.Fatalf("tag.env = %q", labels["tag.env"])
		}
		if labels["tag.team"] != "ops" {
			t.Fatalf("tag.team = %q", labels["tag.team"])
		}
	})

	t.Run("sensitive keys filtered", func(t *testing.T) {
		labels := map[string]string{}
		applyCESTagsToLabels(labels, []cesTagInput{
			{Key: "env", Value: "prod"},
			{Key: "password", Value: "s3cr3t"},
			{Key: "api_key", Value: "AKIDxxx"},
			{Key: "token", Value: "tk-xxx"},
			{Key: "authorization", Value: "Bearer x"},
			{Key: "secret", Value: "s"},
			{Key: "credential", Value: "c"},
		})
		if labels["tag.env"] != "prod" {
			t.Fatalf("tag.env should be set")
		}
		for _, sensitive := range []string{"password", "api_key", "token", "authorization", "secret", "credential"} {
			if _, ok := labels["tag."+sensitive]; ok {
				t.Fatalf("sensitive tag %q should be filtered", sensitive)
			}
		}
	})

	t.Run("count limit", func(t *testing.T) {
		labels := map[string]string{}
		tags := make([]cesTagInput, 0, maxCESTagsPerResource+5)
		for i := 0; i < maxCESTagsPerResource+5; i++ {
			tags = append(tags, cesTagInput{Key: "k" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Value: "v"})
		}
		applyCESTagsToLabels(labels, tags)
		count := 0
		for k := range labels {
			if strings.HasPrefix(k, "tag.") {
				count++
			}
		}
		if count != maxCESTagsPerResource {
			t.Fatalf("tag count = %d, want %d", count, maxCESTagsPerResource)
		}
	})

	t.Run("length truncation", func(t *testing.T) {
		labels := map[string]string{}
		longKey := strings.Repeat("k", maxCESTagKeyLen+50)
		longValue := strings.Repeat("v", maxCESTagValueLen+50)
		applyCESTagsToLabels(labels, []cesTagInput{{Key: longKey, Value: longValue}})
		for k, v := range labels {
			if !strings.HasPrefix(k, "tag.") {
				continue
			}
			keyPart := k[len("tag."):]
			if len(keyPart) != maxCESTagKeyLen {
				t.Fatalf("key length = %d, want %d", len(keyPart), maxCESTagKeyLen)
			}
			if len(v) != maxCESTagValueLen {
				t.Fatalf("value length = %d, want %d", len(v), maxCESTagValueLen)
			}
		}
	})

	t.Run("rune-safe truncation for multibyte UTF-8", func(t *testing.T) {
		labels := map[string]string{}
		// 中文每个字符 3 字节，按字节截断会在字符中间切断产生非法 UTF-8。
		longKey := strings.Repeat("测", maxCESTagKeyLen+10)
		longValue := strings.Repeat("值", maxCESTagValueLen+10)
		applyCESTagsToLabels(labels, []cesTagInput{{Key: longKey, Value: longValue}})
		for k, v := range labels {
			if !strings.HasPrefix(k, "tag.") {
				continue
			}
			keyPart := k[len("tag."):]
			if utf8.RuneCountInString(keyPart) != maxCESTagKeyLen {
				t.Fatalf("key rune count = %d, want %d", utf8.RuneCountInString(keyPart), maxCESTagKeyLen)
			}
			if !utf8.ValidString(keyPart) {
				t.Fatalf("truncated key is not valid UTF-8: %q", keyPart)
			}
			if utf8.RuneCountInString(v) != maxCESTagValueLen {
				t.Fatalf("value rune count = %d, want %d", utf8.RuneCountInString(v), maxCESTagValueLen)
			}
			if !utf8.ValidString(v) {
				t.Fatalf("truncated value is not valid UTF-8: %q", v)
			}
		}
	})

	t.Run("empty key skipped", func(t *testing.T) {
		labels := map[string]string{}
		applyCESTagsToLabels(labels, []cesTagInput{
			{Key: "", Value: "v"},
			{Key: "  ", Value: "v"},
			{Key: "env", Value: "prod"},
		})
		if len(labels) != 1 || labels["tag.env"] != "prod" {
			t.Fatalf("labels = %+v, want only tag.env", labels)
		}
	})
}

func TestMapCESResource_StatusLabels(t *testing.T) {
	t.Run("both status and event_status", func(t *testing.T) {
		in := cesResourceInput{
			Status:      "health",
			EventStatus: "unhealthy",
			Dimensions:  []cesDimInput{{Name: "instance_id", Value: "i-1"}},
		}
		cloud, _ := mapCESResource("r", "SYS.ECS", []string{"instance_id"}, in, "", "")
		if cloud.Labels["ces_status"] != "health" {
			t.Fatalf("ces_status = %q, want health", cloud.Labels["ces_status"])
		}
		if cloud.Labels["ces_event_status"] != "unhealthy" {
			t.Fatalf("ces_event_status = %q, want unhealthy", cloud.Labels["ces_event_status"])
		}
		// CloudResource.Status uses fallback: status first.
		if cloud.Status != "health" {
			t.Fatalf("Status = %q, want health", cloud.Status)
		}
	})

	t.Run("only event_status", func(t *testing.T) {
		in := cesResourceInput{
			EventStatus: "unhealthy",
			Dimensions:  []cesDimInput{{Name: "instance_id", Value: "i-1"}},
		}
		cloud, _ := mapCESResource("r", "SYS.ECS", []string{"instance_id"}, in, "", "")
		if _, ok := cloud.Labels["ces_status"]; ok {
			t.Fatalf("ces_status should not be set when status is empty")
		}
		if cloud.Labels["ces_event_status"] != "unhealthy" {
			t.Fatalf("ces_event_status = %q", cloud.Labels["ces_event_status"])
		}
		// Status fallback to event_status.
		if cloud.Status != "unhealthy" {
			t.Fatalf("Status = %q, want unhealthy (fallback)", cloud.Status)
		}
	})

	t.Run("no status", func(t *testing.T) {
		in := cesResourceInput{
			Dimensions: []cesDimInput{{Name: "instance_id", Value: "i-1"}},
		}
		cloud, _ := mapCESResource("r", "SYS.ECS", []string{"instance_id"}, in, "", "")
		if _, ok := cloud.Labels["ces_status"]; ok {
			t.Fatalf("ces_status should not be set")
		}
		if _, ok := cloud.Labels["ces_event_status"]; ok {
			t.Fatalf("ces_event_status should not be set")
		}
		if cloud.Status != "" {
			t.Fatalf("Status = %q, want empty", cloud.Status)
		}
	})
}

func TestMapCESResource_TagsToLabels(t *testing.T) {
	in := cesResourceInput{
		Status:     "health",
		Dimensions: []cesDimInput{{Name: "instance_id", Value: "i-1"}},
		Tags: []cesTagInput{
			{Key: "env", Value: "prod"},
			{Key: "team", Value: "ops"},
			{Key: "password", Value: "s3cr3t"},
		},
	}
	cloud, ok := mapCESResource("r", "SYS.ECS", []string{"instance_id"}, in, "", "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Labels["tag.env"] != "prod" {
		t.Fatalf("tag.env = %q", cloud.Labels["tag.env"])
	}
	if cloud.Labels["tag.team"] != "ops" {
		t.Fatalf("tag.team = %q", cloud.Labels["tag.team"])
	}
	if _, ok := cloud.Labels["tag.password"]; ok {
		t.Fatalf("sensitive tag.password should be filtered")
	}
}
