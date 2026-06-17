package domain

import "testing"

func TestDefaultRiskForOperation_ProdScriptIsMedium(t *testing.T) {
	got := DefaultRiskForOperation(OpScript, "prod")
	if got != RiskMedium {
		t.Fatalf("expected medium for prod script, got %s", got)
	}
}

func TestResolveRiskLevel_RejectsLowerOverride(t *testing.T) {
	_, err := ResolveRiskLevel(OpRestart, "prod", string(RiskLow))
	if err == nil {
		t.Fatal("expected error when lowering risk")
	}
}

func TestResolveRiskLevel_AllowsHigherOverride(t *testing.T) {
	got, err := ResolveRiskLevel(OpRestart, "prod", string(RiskHigh))
	if err != nil {
		t.Fatal(err)
	}
	if got != RiskHigh {
		t.Fatalf("expected high, got %s", got)
	}
}
