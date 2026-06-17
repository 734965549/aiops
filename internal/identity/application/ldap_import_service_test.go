package application

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestImportLDAPUsersRequiresProvider(t *testing.T) {
	svc := NewAuthService(newFakeAuthUserRepo(), mustTestJWT(t), nil, nil, newFakeExternalIDRepo(), nil, nil, nil, "")
	_, err := svc.ImportLDAPUsers(context.Background(), ImportLDAPUsersInput{
		ProviderID: "corp-ldap", ExternalSubjects: []string{"uid=alice,dc=example,dc=com"},
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestResolveLDAPImportSubjectsDedup(t *testing.T) {
	svc := &AuthService{}
	subjects, err := svc.resolveLDAPImportSubjects(context.Background(), nil, ImportLDAPUsersInput{
		ExternalSubjects: []string{" dn1 ", "dn1", "dn2"},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(subjects) != 2 || subjects[0] != "dn1" || subjects[1] != "dn2" {
		t.Fatalf("unexpected subjects: %+v", subjects)
	}
}

func TestValidateImportRoleCodesRejectsUnknownRole(t *testing.T) {
	ac := &fakeAccessControlRepo{roles: []domain.Role{{ID: "r1", Code: "viewer"}}}
	svc := NewAuthService(newFakeAuthUserRepo(), mustTestJWT(t), nil, ac, nil, nil, nil, nil, "")
	err := svc.validateImportRoleCodes(context.Background(), []string{"viewer", "missing"})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestPublicErrorMessageUsesAppMessage(t *testing.T) {
	err := apperr.Wrap(errors.New("duplicate key value violates unique constraint"), apperr.CodeInternal, "create user failed")
	if got := publicErrorMessage(err); got != "create user failed" {
		t.Fatalf("expected public app message, got %q", got)
	}
}
