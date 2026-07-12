package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestAssetService_UpdateApplication(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service", Environment: "prod"},
	}}
	svc := NewAssetService(apps, &fakeResRepo{}, nil, nil, audit)
	out, err := svc.UpdateApplication(context.Background(), "app-1", Actor{UserID: "u1"}, UpdateApplicationInput{
		Name: "payment-v2", Environment: "staging", Namespace: "pay", Description: "updated",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Name != "payment-v2" || out.Environment != "staging" {
		t.Fatalf("unexpected dto: %+v", out)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditUpdateApplication {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}

func TestAssetService_DeleteApplicationBlockedWhenHasResources(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Pod: "p1"},
	}}
	svc := NewAssetService(apps, resources, nil, nil, NoopAuditRecorder{})
	err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestAssetService_DeleteApplicationAfterResourcesRemoved(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	svc := NewAssetService(apps, &fakeResRepo{}, nil, nil, audit)
	if err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := apps.apps["payment-service"]; ok {
		t.Fatal("application should be deleted")
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditDeleteApplication {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}

func TestAssetService_UpdateAndDeleteResource(t *testing.T) {
	audit := &capturingAssetAudit{}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Name: "old", Pod: "p1"},
	}}
	svc := NewAssetService(&fakeAppRepo{apps: map[string]domain.Application{}}, resources, nil, nil, audit)
	out, err := svc.UpdateResource(context.Background(), "res-1", Actor{UserID: "u1"}, UpdateResourceInput{
		Name: "new-name", Pod: "p2", ResourceType: "pod",
	})
	if err != nil {
		t.Fatalf("update resource: %v", err)
	}
	if out.Name != "new-name" || out.Pod != "p2" {
		t.Fatalf("unexpected dto: %+v", out)
	}
	if err := svc.DeleteResource(context.Background(), "res-1", Actor{UserID: "u1"}); err != nil {
		t.Fatalf("delete resource: %v", err)
	}
	if len(resources.rows) != 0 {
		t.Fatal("resource should be deleted")
	}
	if len(audit.rows) != 2 {
		t.Fatalf("expected 2 audit rows, got %d", len(audit.rows))
	}
}

func TestAssetService_DeleteApplicationBlockedWhenHasMatchRules(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	rules := &fakeRuleRepo{rules: []domain.MatchRule{
		{ID: "rule-1", ApplicationID: "app-1", LabelKey: "service", LabelValuePattern: "payment-*"},
	}}
	svc := NewAssetService(apps, &fakeResRepo{}, rules, nil, NoopAuditRecorder{})
	err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestAssetService_DeleteResourceBlockedWhenHasMatchRules(t *testing.T) {
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Pod: "p1"},
	}}
	rules := &fakeRuleRepo{rules: []domain.MatchRule{
		{ID: "rule-1", ApplicationID: "app-1", ResourceID: "res-1", LabelKey: "service", LabelValuePattern: "payment-*"},
	}}
	svc := NewAssetService(&fakeAppRepo{apps: map[string]domain.Application{}}, resources, rules, nil, NoopAuditRecorder{})
	err := svc.DeleteResource(context.Background(), "res-1", Actor{UserID: "u1"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

// fakeRefChecker 用于测试的跨上下文引用检查器。
type fakeRefChecker struct {
	alertCount  int64
	policyCount int64
	alertErr    error
	policyErr   error
}

func (f *fakeRefChecker) CountAlertsByApplicationID(_ context.Context, _ string) (int64, error) {
	return f.alertCount, f.alertErr
}

func (f *fakeRefChecker) CountInspectionPoliciesByApplicationID(_ context.Context, _ string) (int64, error) {
	return f.policyCount, f.policyErr
}

func TestAssetService_DeleteApplicationBlockedWhenHasAlertReferences(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	refChecker := &fakeRefChecker{alertCount: 2}
	svc := NewAssetService(apps, &fakeResRepo{}, nil, refChecker, NoopAuditRecorder{})
	err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
	// 确保应用未被删除
	if _, ok := apps.apps["payment-service"]; !ok {
		t.Fatal("application should not be deleted when alerts reference it")
	}
}

func TestAssetService_DeleteApplicationBlockedWhenHasInspectionPolicyReferences(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	refChecker := &fakeRefChecker{policyCount: 1}
	svc := NewAssetService(apps, &fakeResRepo{}, nil, refChecker, NoopAuditRecorder{})
	err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
	if _, ok := apps.apps["payment-service"]; !ok {
		t.Fatal("application should not be deleted when inspection policies reference it")
	}
}

func TestAssetService_DeleteApplicationSucceedsWithRefCheckerNil(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	// refChecker 为 nil 时应向后兼容，不检查跨上下文引用
	svc := NewAssetService(apps, &fakeResRepo{}, nil, nil, audit)
	if err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"}); err != nil {
		t.Fatalf("delete should succeed with nil refChecker: %v", err)
	}
	if _, ok := apps.apps["payment-service"]; ok {
		t.Fatal("application should be deleted")
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditDeleteApplication {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}

func TestAssetService_DeleteApplicationSucceedsWhenNoCrossContextReferences(t *testing.T) {
	audit := &capturingAssetAudit{}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service"},
	}}
	// refChecker 报告 0 条引用时，删除应成功
	refChecker := &fakeRefChecker{alertCount: 0, policyCount: 0}
	svc := NewAssetService(apps, &fakeResRepo{}, nil, refChecker, audit)
	if err := svc.DeleteApplication(context.Background(), "app-1", Actor{UserID: "u1"}); err != nil {
		t.Fatalf("delete should succeed with no references: %v", err)
	}
	if _, ok := apps.apps["payment-service"]; ok {
		t.Fatal("application should be deleted")
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditDeleteApplication {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}
