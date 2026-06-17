package application

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestCreateLocalUserUniqueUsername(t *testing.T) {
	repo := newFakeAuthUserRepo(&domain.User{ID: "u1", Username: "alice", Status: domain.UserStatusActive})
	svc := NewAuthService(repo, mustTestJWT(t), auth.NoopRefreshTokenStore{}, nil, nil, nil, nil, nil, "")

	_, err := svc.CreateLocalUser(context.Background(), CreateLocalUserInput{
		Username: "alice", Password: "password123",
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeAlreadyExists {
		t.Fatalf("expected already exists, got %v", err)
	}

	dto, err := svc.CreateLocalUser(context.Background(), CreateLocalUserInput{
		Username: "bob", Password: "password123", DisplayName: "Bob",
	})
	if err != nil || dto == nil || dto.Username != "bob" {
		t.Fatalf("create bob: dto=%+v err=%v", dto, err)
	}
}

func TestProvisionExternalIdentityCreatesNamespacedUser(t *testing.T) {
	repo := newFakeAuthUserRepo()
	extRepo := newFakeExternalIDRepo()
	extUser := &domain.AuthenticatedExternalUser{
		ProviderID: "corp-ldap", ExternalSubject: "uid=alice,dc=example,dc=com",
		Username: "alice@example.com", DisplayName: "Alice", Email: "alice@example.com",
	}
	reg := identityprovider.NewRegistry()
	reg.RegisterPassword(&fakePasswordProvider{
		info: domain.ProviderInfo{ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true},
		user: extUser,
	})

	svc := NewAuthService(repo, mustTestJWT(t), auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, nil, "")
	dto, err := svc.ProvisionExternalIdentity(context.Background(), ProvisionExternalIdentityInput{
		ProviderID: "corp-ldap", ExternalSubject: "uid=alice,dc=example,dc=com",
		ExternalUsername: "alice@example.com", DisplayName: "Alice", Email: "alice@example.com",
	})
	if err != nil || dto == nil {
		t.Fatalf("provision: dto=%+v err=%v", dto, err)
	}
	if dto.Username != "corp-ldap:alice@example.com" {
		t.Fatalf("unexpected username: %q", dto.Username)
	}

	pair, err := svc.LoginExternal(context.Background(), ExternalLoginInput{
		ProviderID: "corp-ldap", Username: "alice@example.com", Password: "any",
	})
	if err != nil || pair == nil || pair.User.Username != dto.Username {
		t.Fatalf("login after provision: pair=%+v err=%v", pair, err)
	}
}

func TestProvisionExternalIdentityBindExistingUser(t *testing.T) {
	repo := newFakeAuthUserRepo(&domain.User{
		ID: "local-1", Username: "alice", Status: domain.UserStatusActive,
	})
	extRepo := newFakeExternalIDRepo()
	reg := identityprovider.NewRegistry()
	reg.RegisterPassword(&fakePasswordProvider{
		info: domain.ProviderInfo{ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true},
	})

	svc := NewAuthService(repo, mustTestJWT(t), auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, nil, "")
	dto, err := svc.ProvisionExternalIdentity(context.Background(), ProvisionExternalIdentityInput{
		ProviderID: "corp-ldap", ExternalSubject: "uid=alice,dc=example,dc=com",
		ExternalUsername: "alice@example.com", UserID: "local-1",
	})
	if err != nil || dto == nil || dto.Username != "alice" {
		t.Fatalf("bind existing: dto=%+v err=%v", dto, err)
	}
}

type failingExternalIDRepo struct {
	*fakeExternalIDRepo
}

func (f *failingExternalIDRepo) Create(_ context.Context, ext *domain.ExternalIdentity) error {
	return errors.New("simulated binding failure")
}

func TestProvisionExternalIdentityRollsBackUserOnBindingFailure(t *testing.T) {
	repo := newFakeAuthUserRepo()
	extRepo := &failingExternalIDRepo{fakeExternalIDRepo: newFakeExternalIDRepo()}
	svc := NewAuthService(repo, mustTestJWT(t), auth.NoopRefreshTokenStore{}, nil, extRepo, nil, nil, nil, "")

	_, err := svc.ProvisionExternalIdentity(context.Background(), ProvisionExternalIdentityInput{
		ProviderID: "corp-ldap", ExternalSubject: "uid=alice,dc=example,dc=com",
		ExternalUsername: "alice@example.com",
	})
	if err == nil {
		t.Fatal("expected binding failure")
	}
	if len(repo.byID) != 0 {
		t.Fatalf("expected orphan user rolled back, still have %d users", len(repo.byID))
	}
}

func mustTestJWT(t *testing.T) *auth.JWTManager {
	t.Helper()
	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "provisioning-test-secret-with-length", Issuer: "test",
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	return jwtMgr
}
