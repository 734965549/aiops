package identity

import (
	"context"
	"errors"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func scopeTestAlert() *aiapp.AlertContext {
	return &aiapp.AlertContext{
		ID: "a1", Name: "HighCPU", Severity: "p1",
		BusinessLine: "payments", Environment: "prod",
		OwnerUserID: "user-9",
		Labels: map[string]string{
			"team":       "sre",
			"department": "platform",
			"region":     "cn-north-4",
		},
	}
}

func TestAlertScopeAuthorizerPassesAlertObjectContext(t *testing.T) {
	checker := &fakeAuthorizationChecker{result: &identityapp.AuthorizationResult{Allowed: true}}
	guard := NewAlertScopeAuthorizer(checker)

	if err := guard.AuthorizeAlert(context.Background(), "user-1", scopeTestAlert()); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	in := checker.input
	if in.UserID != "user-1" {
		t.Fatalf("user_id = %q", in.UserID)
	}
	if in.ObjectOwner != "user-9" || in.ObjectTeam != "sre" || in.ObjectDept != "platform" || in.ObjectRegion != "cn-north-4" {
		t.Fatalf("unexpected object context: %+v", in)
	}
	// tag 维度取 env/business_line；不得把告警正文塞进授权入参。
	if len(in.ObjectTags) != 2 || in.ObjectTags[0] != "prod" || in.ObjectTags[1] != "payments" {
		t.Fatalf("object tags = %+v", in.ObjectTags)
	}
	// 数据范围守卫只做对象级判断，不应拼 resource/action 或工具权限。
	if in.Resource != "" || in.Action != "" || in.ToolCode != "" || in.RequiredPermission != "" {
		t.Fatalf("scope guard must not re-check RBAC: %+v", in)
	}
}

func TestAlertScopeAuthorizerDenies(t *testing.T) {
	checker := &fakeAuthorizationChecker{err: apperr.New(apperr.CodePermissionDenied, "data scope denied")}
	guard := NewAlertScopeAuthorizer(checker)

	err := guard.AuthorizeAlert(context.Background(), "user-1", scopeTestAlert())
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAlertScopeAuthorizerDeniesWhenResultNotAllowed(t *testing.T) {
	checker := &fakeAuthorizationChecker{result: &identityapp.AuthorizationResult{Allowed: false, Reason: "data scope denied: missing object context"}}
	guard := NewAlertScopeAuthorizer(checker)

	err := guard.AuthorizeAlert(context.Background(), "user-1", &aiapp.AlertContext{ID: "a1"})
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if ae := apperr.FromError(err); ae == nil || ae.Message != "data scope denied: missing object context" {
		t.Fatalf("expected upstream reason to be preserved, got %v", err)
	}
}

func TestAlertScopeAuthorizerPropagatesServiceError(t *testing.T) {
	checker := &fakeAuthorizationChecker{err: errors.New("authorization unavailable")}
	guard := NewAlertScopeAuthorizer(checker)

	if err := guard.AuthorizeAlert(context.Background(), "user-1", scopeTestAlert()); err == nil {
		t.Fatal("expected service error")
	}
}

func TestAlertScopeAuthorizerRequiresChecker(t *testing.T) {
	guard := NewAlertScopeAuthorizer(nil)
	if err := guard.AuthorizeAlert(context.Background(), "user-1", scopeTestAlert()); apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}
