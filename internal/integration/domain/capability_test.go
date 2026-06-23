package domain

import "testing"

func TestCapability_IsValid(t *testing.T) {
	valid := []Capability{
		CapabilityMetrics, CapabilityLogs, CapabilityTraces, CapabilityTopology, CapabilityAlerts, CapabilityAssets,
	}
	for _, c := range valid {
		if !c.IsValid() {
			t.Fatalf("expected %q to be valid", c)
		}
	}
	if Capability("unknown").IsValid() {
		t.Fatal("unknown capability should be invalid")
	}
}

func TestDefaultCapabilitiesForProvider(t *testing.T) {
	huawei := DefaultCapabilitiesForProvider(ProviderHuaweiCloud)
	if len(huawei) != 6 {
		t.Fatalf("expected 6 huawei capabilities, got %d", len(huawei))
	}
	signoz := DefaultCapabilitiesForProvider(ProviderSigNoz)
	if len(signoz) != 5 {
		t.Fatalf("expected 5 signoz capabilities, got %d", len(signoz))
	}
	prom := DefaultCapabilitiesForProvider(ProviderPrometheus)
	if len(prom) != 2 {
		t.Fatalf("expected 2 prometheus capabilities, got %d", len(prom))
	}
}
