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
