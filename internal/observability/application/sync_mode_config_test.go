package application

import (
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/observability/domain"
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
			if cfg.ResourceGroupName != "" {
				t.Fatalf("ResourceGroupName = %q, want empty (unset, default candidates handled at selection)", cfg.ResourceGroupName)
			}
			if cfg.RawFetchedCountBudget != DefaultRawRowBudget {
				t.Fatalf("RawFetchedCountBudget = %d, want %d", cfg.RawFetchedCountBudget, DefaultRawRowBudget)
			}
			if cfg.MaxResources != DefaultMaxResources {
				t.Fatalf("MaxResources = %d, want %d", cfg.MaxResources, DefaultMaxResources)
			}
		})
	}
}

func TestParseSyncModeConfig_ExplicitValues(t *testing.T) {
	raw := []byte(`{
		"sync_mode": "hybrid",
		"resource_group_name": "全部资源",
		"resource_group_id": "rg123",
		"enterprise_project_id": "all_granted_eps",
		"max_resources": 5000
	}`)
	cfg := ParseSyncModeConfig(raw)
	if cfg.Mode != SyncModeHybrid {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, SyncModeHybrid)
	}
	if cfg.ResourceGroupName != "" {
		t.Fatalf("ResourceGroupName = %q, want empty placeholder to be handled at selection time", cfg.ResourceGroupName)
	}
	if cfg.ResourceGroupID != "rg123" {
		t.Fatalf("ResourceGroupID = %q, want %q", cfg.ResourceGroupID, "rg123")
	}
	if cfg.EnterpriseProjectID != "all_granted_eps" {
		t.Fatalf("EnterpriseProjectID = %q, want %q", cfg.EnterpriseProjectID, "all_granted_eps")
	}
	if cfg.RawFetchedCountBudget != 5000 {
		t.Fatalf("RawFetchedCountBudget = %d, want %d", cfg.RawFetchedCountBudget, 5000)
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

func TestParseSyncModeConfigWithDiagnostics_UnknownModeReportsFallback(t *testing.T) {
	cfg, diag := ParseSyncModeConfigWithDiagnostics([]byte(`{"sync_mode":"hybird"}`))
	if cfg.Mode != SyncModeCES {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, SyncModeCES)
	}
	if !diag.FallbackToCES {
		t.Fatal("expected FallbackToCES to be true")
	}
	if diag.InvalidSyncMode != "hybird" {
		t.Fatalf("InvalidSyncMode = %q, want %q", diag.InvalidSyncMode, "hybird")
	}
	if diag.EffectiveMode != SyncModeCES {
		t.Fatalf("EffectiveMode = %q, want %q", diag.EffectiveMode, SyncModeCES)
	}
	if len(diag.Warnings) == 0 {
		t.Fatal("expected warnings for invalid sync_mode")
	}
}

func TestValidateSyncModeConfig_ReturnsWarnings(t *testing.T) {
	warnings := ValidateSyncModeConfig([]byte(`{"sync_mode":"hybird"}`))
	if len(warnings) == 0 {
		t.Fatal("expected warnings")
	}
}

func TestParseSyncModeConfig_InvalidMaxResourcesKeepsDefault(t *testing.T) {
	raw := []byte(`{"max_resources": -1}`)
	cfg := ParseSyncModeConfig(raw)
	if cfg.MaxResources != DefaultMaxResources {
		t.Fatalf("MaxResources = %d, want %d", cfg.MaxResources, DefaultMaxResources)
	}
}

func TestParseSyncModeConfig_BlankAndPlaceholderResourceGroupNameStayUnset(t *testing.T) {
	cases := map[string]string{
		"blank":       `{"resource_group_name": "  "}`,
		"placeholder": `{"resource_group_name": "All Resources"}`,
		"chinese":     `{"resource_group_name": "全部资源"}`,
	}
	for name, rawJSON := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := ParseSyncModeConfig([]byte(rawJSON))
			if cfg.ResourceGroupName != "" {
				t.Fatalf("ResourceGroupName = %q, want empty (placeholder must not be treated as specified)", cfg.ResourceGroupName)
			}
		})
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
		"hit but empty pid": {"cn-south-1", "fallback", "pid-south"},
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

func TestParseSyncModeConfig_RegionProjectResourceGroup(t *testing.T) {
	raw := []byte(`{
		"region_projects": [
			{"region":"cn-south-1","project_id":"pid-south","resource_group_id":"rg-south","resource_group_name":"南方全量"},
			{"region":" cn-north-4 ","project_id":" pid-north ","resource_group_id":" rg-north ","resource_group_name":" 北方全量 "},
			{"region":"cn-east-3","project_id":"pid-east"}
		]
	}`)
	cfg := ParseSyncModeConfig(raw)
	if len(cfg.RegionProjects) != 3 {
		t.Fatalf("RegionProjects len = %d, want 3", len(cfg.RegionProjects))
	}
	rp0 := cfg.RegionProjects[0]
	if rp0.ResourceGroupID != "rg-south" || rp0.ResourceGroupName != "南方全量" {
		t.Fatalf("RegionProjects[0] = %+v, want rg-south/南方全量", rp0)
	}
	rp1 := cfg.RegionProjects[1]
	if rp1.Region != "cn-north-4" || rp1.ProjectID != "pid-north" || rp1.ResourceGroupID != "rg-north" || rp1.ResourceGroupName != "北方全量" {
		t.Fatalf("RegionProjects[1] = %+v, want trimmed rg-north/北方全量", rp1)
	}
	rp2 := cfg.RegionProjects[2]
	if rp2.ResourceGroupID != "" || rp2.ResourceGroupName != "" {
		t.Fatalf("RegionProjects[2] = %+v, want empty resource group fields", rp2)
	}
}

func TestResolveResourceGroupID(t *testing.T) {
	cfg := SyncModeConfig{
		ResourceGroupID: "rg-global",
		RegionProjects: []RegionProject{
			{Region: "cn-south-1", ProjectID: "pid-south", ResourceGroupID: "rg-south"},
			{Region: "cn-north-4", ProjectID: "pid-north"},
		},
	}
	cases := map[string]struct {
		region string
		want   string
	}{
		"hit south":          {"cn-south-1", "rg-south"},
		"case insensitive":   {"CN-SOUTH-1", "rg-south"},
		"hit but empty fall": {"cn-north-4", "rg-global"},
		"miss falls back":    {"cn-east-3", "rg-global"},
		"empty region":       {"", "rg-global"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := cfg.ResolveResourceGroupID(c.region); got != c.want {
				t.Fatalf("ResolveResourceGroupID(%q) = %q, want %q", c.region, got, c.want)
			}
		})
	}
}

func TestResolveResourceGroupName(t *testing.T) {
	cfg := SyncModeConfig{
		ResourceGroupName: "全部资源",
		RegionProjects: []RegionProject{
			{Region: "cn-south-1", ProjectID: "pid-south", ResourceGroupName: "南方全量"},
			{Region: "cn-north-4", ProjectID: "pid-north"},
		},
	}
	cases := map[string]struct {
		region string
		want   string
	}{
		"hit south":          {"cn-south-1", "南方全量"},
		"case insensitive":   {"CN-SOUTH-1", "南方全量"},
		"hit but empty fall": {"cn-north-4", "全部资源"},
		"miss falls back":    {"cn-east-3", "全部资源"},
		"empty region":       {"", "全部资源"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := cfg.ResolveResourceGroupName(c.region); got != c.want {
				t.Fatalf("ResolveResourceGroupName(%q) = %q, want %q", c.region, got, c.want)
			}
		})
	}
}

func TestResolveSyncScopes(t *testing.T) {
	account := domain.AccountSnapshot{
		ProjectID:   "pid-global",
		ExtraConfig: []byte(`{"sync_mode":"hybrid","resource_group_name":"全部资源","resource_group_id":"rg-global","region_projects":[{"region":"cn-south-1","project_id":"pid-south","resource_group_id":"rg-south","resource_group_name":"南方全量"}]}`),
	}
	scopes, fallbackRegions := ResolveSyncScopes(account, []string{"cn-south-1", " cn-north-4 ", "", "cn-south-1"})
	if len(scopes) != 2 {
		t.Fatalf("scopes len = %d, want 2", len(scopes))
	}
	if len(fallbackRegions) != 1 || fallbackRegions[0] != "cn-north-4" {
		t.Fatalf("fallbackRegions = %+v, want [cn-north-4]", fallbackRegions)
	}
	if scopes[0].Region != "cn-south-1" || scopes[0].ProjectID != "pid-south" || scopes[0].SyncMode != SyncModeHybrid ||
		scopes[0].ResourceGroupID != "rg-south" || scopes[0].ResourceGroupName != "南方全量" || scopes[0].ResourceGroupSelection != "explicit" {
		t.Fatalf("scopes[0] = %+v", scopes[0])
	}
	if scopes[1].Region != "cn-north-4" || scopes[1].ProjectID != "pid-global" || scopes[1].SyncMode != SyncModeHybrid ||
		scopes[1].ResourceGroupID != "rg-global" || scopes[1].ResourceGroupName != "" || scopes[1].ResourceGroupSelection != "fallback" {
		t.Fatalf("scopes[1] = %+v", scopes[1])
	}
}
