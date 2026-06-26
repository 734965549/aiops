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
