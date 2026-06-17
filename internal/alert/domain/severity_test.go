package domain

import "testing"

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		raw  string
		want AlertSeverity
	}{
		{"critical", SeverityP0},
		{"fatal", SeverityP0},
		{"emergency", SeverityP0},
		{"p0", SeverityP0},
		{"high", SeverityP1},
		{"major", SeverityP1},
		{"warning", SeverityP1},
		{"p1", SeverityP1},
		{"medium", SeverityP2},
		{"minor", SeverityP2},
		{"p2", SeverityP2},
		{"low", SeverityP3},
		{"notice", SeverityP3},
		{"p3", SeverityP3},
		{"info", SeverityInfo},
		{"none", SeverityInfo},
		{"", SeverityInfo},
		{"unknown", SeverityInfo},
		{" P1 ", SeverityP1},
	}
	for _, tc := range tests {
		got := NormalizeSeverity(tc.raw)
		if got != tc.want {
			t.Fatalf("NormalizeSeverity(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestAlertSeverityDisplayLabel(t *testing.T) {
	if SeverityP0.DisplayLabel() != "P0" {
		t.Fatalf("expected P0 label")
	}
	if SeverityInfo.DisplayLabel() != "Info" {
		t.Fatalf("expected Info label")
	}
}

func TestAlertSeverityIsValid(t *testing.T) {
	if !SeverityP1.IsValid() {
		t.Fatal("p1 should be valid")
	}
	if (AlertSeverity("invalid")).IsValid() {
		t.Fatal("invalid severity should not be valid")
	}
}
