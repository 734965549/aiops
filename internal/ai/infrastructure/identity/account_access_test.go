package identity

import (
	"context"
	"errors"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAuthorizationChecker struct {
	result *identityapp.AuthorizationResult
	err    error
	called bool
	input  identityapp.AuthorizationInput
}

func (f *fakeAuthorizationChecker) Authorize(_ context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	f.called = true
	f.input = in
	return f.result, f.err
}

func TestAccountAccessAllows(t *testing.T) {
	checker := &fakeAuthorizationChecker{result: &identityapp.AuthorizationResult{Allowed: true}}
	access := NewAccountAccessAuthorizer(checker)

	err := access.Authorize(context.Background(), "user-1", aiapp.AccountRef{
		OwnerTeam: "sre", Regions: []string{"cn-north-4"},
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !checker.called || checker.input.ObjectTeam != "sre" || checker.input.ObjectRegion != "cn-north-4" {
		t.Fatalf("unexpected authorization input: %+v", checker.input)
	}
}

func TestAccountAccessDenied(t *testing.T) {
	checker := &fakeAuthorizationChecker{result: &identityapp.AuthorizationResult{Allowed: false}}
	access := NewAccountAccessAuthorizer(checker)

	err := access.Authorize(context.Background(), "user-1", aiapp.AccountRef{})
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestAccountAccessPropagatesServiceError(t *testing.T) {
	checker := &fakeAuthorizationChecker{err: errors.New("authorization unavailable")}
	access := NewAccountAccessAuthorizer(checker)

	if err := access.Authorize(context.Background(), "user-1", aiapp.AccountRef{}); err == nil {
		t.Fatal("expected service error")
	}
}

func TestAccountAccessSkipsEmptyUser(t *testing.T) {
	checker := &fakeAuthorizationChecker{}
	access := NewAccountAccessAuthorizer(checker)

	if err := access.Authorize(context.Background(), "", aiapp.AccountRef{}); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if checker.called {
		t.Fatal("empty user should bypass authorization check")
	}
}
