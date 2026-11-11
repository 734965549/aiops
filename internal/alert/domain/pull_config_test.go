package domain

import (
	"testing"
	"time"
)

func TestNormalizePullConfig_DefaultsAndManualMode(t *testing.T) {
	cfg, err := NormalizePullConfig(PullConfig{})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.PullIntervalSeconds != 0 {
		t.Fatalf("0 must remain manual-only, got %d", cfg.PullIntervalSeconds)
	}
	if cfg.SeverityMap["1"] != "p0" || cfg.SeverityMap["4"] != "p3" {
		t.Fatalf("unexpected default severity map: %+v", cfg.SeverityMap)
	}
	if cfg.EffectivePullInterval() != 0 {
		t.Fatal("manual-only config must not schedule")
	}
	if cfg.WithDefaults().PullIntervalSeconds != 0 {
		t.Fatal("WithDefaults must not turn manual-only into periodic")
	}
}

func TestNormalizePullConfig_IntervalBounds(t *testing.T) {
	cases := []struct {
		seconds int
		ok      bool
	}{
		{0, true}, {59, false}, {60, true}, {86400, true}, {86401, false}, {-1, false},
	}
	for _, tc := range cases {
		_, err := NormalizePullConfig(PullConfig{PullIntervalSeconds: tc.seconds})
		if (err == nil) != tc.ok {
			t.Fatalf("interval %d: ok=%v err=%v", tc.seconds, tc.ok, err)
		}
	}
}

func TestNormalizePullConfig_SeverityMapAndRegion(t *testing.T) {
	cfg, err := NormalizePullConfig(PullConfig{
		PullIntervalSeconds: 300,
		SeverityMap:         map[string]string{"2": "info"},
		RegionFilter:        []string{" CN-North-4 ", "cn-north-4", "cn-south-1"},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.SeverityMap["2"] != "info" || cfg.SeverityMap["1"] != "p0" {
		t.Fatalf("unexpected severity override: %+v", cfg.SeverityMap)
	}
	if len(cfg.RegionFilter) != 2 || cfg.RegionFilter[0] != "cn-north-4" || cfg.RegionFilter[1] != "cn-south-1" {
		t.Fatalf("unexpected region filter: %+v", cfg.RegionFilter)
	}
	if got, _ := cfg.SeverityFor("2"); got != SeverityInfo {
		t.Fatalf("expected info, got %s", got)
	}
	if got, ok := cfg.SeverityFor("9"); ok || got != SeverityInfo {
		t.Fatalf("unknown level must fallback to info, got %s ok=%v", got, ok)
	}
	for _, bad := range []map[string]string{{"5": "p0"}, {"1": "critical"}, {"": "p0"}} {
		if _, err := NormalizePullConfig(PullConfig{SeverityMap: bad}); err == nil {
			t.Fatalf("expected invalid severity map %+v", bad)
		}
	}
}

func TestPullConfig_EffectiveInterval(t *testing.T) {
	if got := (PullConfig{PullIntervalSeconds: 60}).EffectivePullInterval(); got != time.Minute {
		t.Fatalf("got %v", got)
	}
	if got := (PullConfig{PullIntervalSeconds: 300}).EffectivePullInterval(); got != 5*time.Minute {
		t.Fatalf("got %v", got)
	}
}
