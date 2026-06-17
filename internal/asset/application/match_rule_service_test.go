package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
)

func TestMatchRuleService_CreateWritesAudit(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	svc := NewMatchRuleService(&fakeRuleRepo{}, apps, &fakeResRepo{}, audit)
	enabled := true
	out, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateMatchRuleInput{
		Name: "payment rule", Enabled: &enabled, Priority: 10,
		TargetType: "application", SourceType: "all",
		LabelKey: "service", LabelValuePattern: "payment-*", ApplicationID: "app-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.ID == "" || out.Name != "payment rule" {
		t.Fatalf("unexpected dto: %+v", out)
	}
	if len(audit.rows) != 1 || audit.rows[0].ResourceType != "match_rule" || audit.rows[0].Action != AuditCreateMatchRule {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}

func TestMatchRuleService_DeleteWritesAudit(t *testing.T) {
	audit := &capturingAssetAudit{}
	rules := &fakeRuleRepo{rules: []domain.MatchRule{
		{ID: "rule-1", Name: "r1", ApplicationID: "app-1", LabelKey: "service", LabelValuePattern: "*"},
	}}
	svc := NewMatchRuleService(rules, &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}, &fakeResRepo{}, audit)
	if err := svc.Delete(context.Background(), "rule-1", Actor{UserID: "u1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditDeleteMatchRule {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}
