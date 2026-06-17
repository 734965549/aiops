package domain

import "testing"

func TestMatchesTemplate(t *testing.T) {
	tpl := Template{
		Enabled:           true,
		MatchAlertName:    "HighCPU",
		MatchResourceType: "pod",
		MatchEnvironment:  "prod",
	}
	alert := AlertMatchContext{
		Name:         "HighCPU",
		ResourceType: "pod",
		Environment:  "prod",
	}
	ok, score := MatchesTemplate(tpl, alert)
	if !ok {
		t.Fatal("expected match")
	}
	if !score.EnvExact || !score.ResourceExact || !score.NameExact {
		t.Fatalf("score: %+v", score)
	}
}

func TestMatchesTemplateFuzzyName(t *testing.T) {
	tpl := Template{Enabled: true, MatchAlertName: "CPU"}
	ok, score := MatchesTemplate(tpl, AlertMatchContext{Name: "HighCPUAlert"})
	if !ok || !score.NameFuzzy {
		t.Fatalf("expected fuzzy match, ok=%v score=%+v", ok, score)
	}
}

func TestMatchesTemplateWildcard(t *testing.T) {
	tpl := Template{Enabled: true}
	ok, _ := MatchesTemplate(tpl, AlertMatchContext{Name: "anything"})
	if !ok {
		t.Fatal("wildcard should match")
	}
}

func TestResolveTaskRiskCannotLower(t *testing.T) {
	_, err := ResolveTaskRisk(OpRunbook, "prod", RiskMedium, []Step{{RiskLevel: RiskHigh}}, "low")
	if err != ErrInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestResolveTaskRiskProdCommand(t *testing.T) {
	risk, err := ResolveTaskRisk(OpRunbook, "prod", RiskLow, []Step{{ActionType: ActionCommand, RiskLevel: RiskLow}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if risk != RiskMedium {
		t.Fatalf("expected medium, got %s", risk)
	}
}
