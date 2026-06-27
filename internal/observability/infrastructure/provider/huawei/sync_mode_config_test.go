package huawei

import (
	"strings"
	"testing"
)

func TestParseSyncModeConfig_Defaults(t *testing.T) {
	cases := map[string][]byte{
		"nil":          nil,
		"empty":        {},
		"blank":        []byte("   "),
		"empty object": []byte("{}"),
		"invalid json": []byte("{not json"),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := ParseSyncModeConfig(raw)
			if cfg.Mode != SyncModeCES {
				t.Fatalf("Mode = %q, want %q", cfg.Mode, SyncModeCES)
			}
			if cfg.ResourceGroupName != defaultResourceGroupName {
				t.Fatalf("ResourceGroupName = %q, want %q", cfg.ResourceGroupName, defaultResourceGroupName)
			}
			if cfg.MaxResources != defaultMaxResources {
				t.Fatalf("MaxResources = %d, want %d", cfg.MaxResources, defaultMaxResources)
			}
		})
	}
}

func TestParseSyncModeConfig_ExplicitValues(t *testing.T) {
	raw := []byte(`{
		"sync_mode": "hybrid",
		"resource_group_name": "All Resources",
		"resource_group_id": "rg123",
		"enterprise_project_id": "all_granted_eps",
		"max_resources": 5000
	}`)
	cfg := ParseSyncModeConfig(raw)
	if cfg.Mode != SyncModeHybrid {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, SyncModeHybrid)
	}
	if cfg.ResourceGroupName != "All Resources" {
		t.Fatalf("ResourceGroupName = %q, want %q", cfg.ResourceGroupName, "All Resources")
	}
	if cfg.ResourceGroupID != "rg123" {
		t.Fatalf("ResourceGroupID = %q, want %q", cfg.ResourceGroupID, "rg123")
	}
	if cfg.EnterpriseProjectID != "all_granted_eps" {
		t.Fatalf("EnterpriseProjectID = %q, want %q", cfg.EnterpriseProjectID, "all_granted_eps")
	}
	if cfg.MaxResources != 5000 {
		t.Fatalf("MaxResources = %d, want %d", cfg.MaxResources, 5000)
	}
}

func TestParseSyncModeConfig_UnknownModeFallsBackToCES(t *testing.T) {
	cases := map[string]string{
		"native upper":  "NATIVE",
		"unknown value": "totally_unknown",
		"empty":         "",
	}
	for name, mode := range cases {
		t.Run(name, func(t *testing.T) {
			raw := []byte(`{"sync_mode":"` + mode + `"}`)
			cfg := ParseSyncModeConfig(raw)
			// NATIVE 大小写归一化为 native；其余未知值回落 ces。
			want := SyncModeCES
			if strings.EqualFold(mode, SyncModeNative) {
				want = SyncModeNative
			}
			if cfg.Mode != want {
				t.Fatalf("Mode = %q, want %q", cfg.Mode, want)
			}
		})
	}
}

func TestParseSyncModeConfig_InvalidMaxResourcesKeepsDefault(t *testing.T) {
	raw := []byte(`{"max_resources": -1}`)
	cfg := ParseSyncModeConfig(raw)
	if cfg.MaxResources != defaultMaxResources {
		t.Fatalf("MaxResources = %d, want %d", cfg.MaxResources, defaultMaxResources)
	}
}

func TestParseSyncModeConfig_BlankResourceGroupNameKeepsDefault(t *testing.T) {
	raw := []byte(`{"resource_group_name": "  "}`)
	cfg := ParseSyncModeConfig(raw)
	if cfg.ResourceGroupName != defaultResourceGroupName {
		t.Fatalf("ResourceGroupName = %q, want %q", cfg.ResourceGroupName, defaultResourceGroupName)
	}
}

func TestParseSyncModeConfig_RegionProjects(t *testing.T) {
	raw := []byte(`{
		"region_projects": [
			{"region": "cn-south-1", "project_id": "pid-south"},
			{"region": " cn-north-4 ", "project_id": " pid-north "},
			{"region": "", "project_id": "skip-empty-region"},
			{"region": "skip-empty-pid", "project_id": ""},
			{"region": "CN-SOUTH-1", "project_id": "should-be-dedup"},
			"not-an-object"
		]
	}`)
	cfg := ParseSyncModeConfig(raw)
	if len(cfg.RegionProjects) != 2 {
		t.Fatalf("RegionProjects len = %d, want 2 (empty/非法项应过滤，重复 region 去重)", len(cfg.RegionProjects))
	}
	if cfg.RegionProjects[0].Region != "cn-south-1" || cfg.RegionProjects[0].ProjectID != "pid-south" {
		t.Fatalf("RegionProjects[0] = %+v", cfg.RegionProjects[0])
	}
	// 第二项 region/project_id 被 trim。
	if cfg.RegionProjects[1].Region != "cn-north-4" || cfg.RegionProjects[1].ProjectID != "pid-north" {
		t.Fatalf("RegionProjects[1] = %+v", cfg.RegionProjects[1])
	}
}

func TestResolveProjectID(t *testing.T) {
	cfg := SyncModeConfig{
		RegionProjects: []RegionProject{
			{Region: "cn-south-1", ProjectID: "pid-south"},
			{Region: "cn-north-4", ProjectID: "pid-north"},
		},
	}
	cases := map[string]struct {
		region   string
		fallback string
		want     string
	}{
		"hit south":         {"cn-south-1", "fallback", "pid-south"},
		"hit north":         {"cn-north-4", "fallback", "pid-north"},
		"case insensitive":  {"CN-SOUTH-1", "fallback", "pid-south"},
		"miss falls back":   {"cn-east-3", "fallback", "fallback"},
		"empty region":      {"", "fallback", "fallback"},
		"empty fallback":    {"cn-east-3", "", ""},
		"hit but empty pid": {"cn-south-1", "fallback", "pid-south"}, // 解析阶段已过滤空 pid，命中即非空
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := cfg.ResolveProjectID(c.region, c.fallback)
			if got != c.want {
				t.Fatalf("ResolveProjectID(%q, %q) = %q, want %q", c.region, c.fallback, got, c.want)
			}
		})
	}
}
