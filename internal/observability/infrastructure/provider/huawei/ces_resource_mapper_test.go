package huawei

import (
	"reflect"
	"testing"

	"github.com/734965549/aiops/internal/observability/domain"
)

func TestParseProductNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []cesProduct
	}{
		{"empty", "", nil},
		{"blank", "   ", nil},
		{"single", "SYS.ECS,instance_id", []cesProduct{{Service: "SYS.ECS", DimName: "instance_id"}}},
		{"multi", "SYS.ECS,instance_id;SYS.EVS,disk_name", []cesProduct{
			{Service: "SYS.ECS", DimName: "instance_id"},
			{Service: "SYS.EVS", DimName: "disk_name"},
		}},
		{"dedup", "SYS.ECS,instance_id;SYS.ECS,instance_id", []cesProduct{{Service: "SYS.ECS", DimName: "instance_id"}}},
		{"skip empty parts", ";;SYS.ECS,instance_id;;", []cesProduct{{Service: "SYS.ECS", DimName: "instance_id"}}},
		{"trim spaces", " SYS.ECS , instance_id ; SYS.EVS , disk_name ", []cesProduct{
			{Service: "SYS.ECS", DimName: "instance_id"},
			{Service: "SYS.EVS", DimName: "disk_name"},
		}},
		{"missing dim_name", "SYS.ECS", []cesProduct{{Service: "SYS.ECS", DimName: ""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseProductNames(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseProductNames(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveNamespaceMapping_Known(t *testing.T) {
	want := map[string]namespaceMapping{
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

func TestSelectPrimaryDimension(t *testing.T) {
	in := cesResourceInput{
		ResourceName: "fallback-name",
		Dimensions: []cesDimInput{
			{Name: "other_dim", Value: "other-val"},
			{Name: "instance_id", Value: "i-primary"},
			{Name: "empty", Value: ""},
		},
	}
	if got := selectPrimaryDimension("instance_id", in); got != "i-primary" {
		t.Fatalf("dim_name match = %q, want %q", got, "i-primary")
	}
	// dim_name 不匹配时取第一个非空 dimension。
	if got := selectPrimaryDimension("not_exist", in); got != "other-val" {
		t.Fatalf("first non-empty = %q, want %q", got, "other-val")
	}
	// 无 dimension 时取 resource_name。
	inNoDim := cesResourceInput{ResourceName: "name-only"}
	if got := selectPrimaryDimension("instance_id", inNoDim); got != "name-only" {
		t.Fatalf("resource_name fallback = %q, want %q", got, "name-only")
	}
	// 全空返回空串。
	inEmpty := cesResourceInput{}
	if got := selectPrimaryDimension("instance_id", inEmpty); got != "" {
		t.Fatalf("empty = %q, want empty", got)
	}
}

func TestMapCESResource(t *testing.T) {
	in := cesResourceInput{
		Status:             "health",
		EventStatus:        "unhealthy",
		ResourceName:       "my-ecs",
		EnterpriseProjectID: "eps-001",
		Dimensions:         []cesDimInput{{Name: "instance_id", Value: "i-12345"}},
	}
	cloud, ok := mapCESResource("cn-south-1", "SYS.ECS", "instance_id", in, "rg001", "全部资源")
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
			"namespace":              "SYS.ECS",
			"dim_name":               "instance_id",
			"enterprise_project_id":  "eps-001",
			"resource_group_id":      "rg001",
			"resource_group_name":    "全部资源",
		},
	}
	if !reflect.DeepEqual(cloud, want) {
		t.Fatalf("mapCESResource = %+v, want %+v", cloud, want)
	}
}

func TestMapCESResource_NoPrimaryDimReturnsFalse(t *testing.T) {
	in := cesResourceInput{ResourceName: "", Dimensions: nil}
	_, ok := mapCESResource("cn-south-1", "SYS.ECS", "instance_id", in, "rg001", "g")
	if ok {
		t.Fatalf("expected ok=false when no primary dimension")
	}
}

func TestMapCESResource_UnknownNamespace(t *testing.T) {
	in := cesResourceInput{
		ResourceName: "x",
		Dimensions:   []cesDimInput{{Name: "res_id", Value: "v-1"}},
	}
	cloud, ok := mapCESResource("cn-north-4", "SYS.NEW_SERVICE", "res_id", in, "", "")
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

func TestMapCESResource_NameFallbackToPrimary(t *testing.T) {
	in := cesResourceInput{
		Dimensions: []cesDimInput{{Name: "instance_id", Value: "i-1"}},
	}
	cloud, ok := mapCESResource("r", "SYS.ECS", "instance_id", in, "", "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if cloud.Name != "i-1" {
		t.Fatalf("Name = %q, want %q", cloud.Name, "i-1")
	}
}
