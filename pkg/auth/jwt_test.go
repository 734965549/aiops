package auth

import (
	"errors"
	"testing"
	"time"
)

const testJWTSecret = "unit-test-jwt-secret-with-enough-length"

func newTestJWTManager(t *testing.T, accessTTL, refreshTTL time.Duration) *JWTManager {
	t.Helper()
	mgr, err := NewJWTManager(Options{
		Secret:     testJWTSecret,
		Issuer:     "aiops-test",
		AccessTTL:  accessTTL,
		RefreshTTL: refreshTTL,
		ClockSkew:  time.Second,
	})
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	return mgr
}

func TestNewJWTManagerRejectsEmptySecret(t *testing.T) {
	if _, err := NewJWTManager(Options{Secret: ""}); err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestJWTManagerIssueAndVerifyAccess(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	token, exp, err := mgr.IssueAccess(IssueOptions{
		UserID:   "user-1",
		Username: "alice",
		Roles:    []string{"admin"},
	})
	if err != nil || token == "" || exp.Before(time.Now()) {
		t.Fatalf("issue access: token=%q exp=%v err=%v", token, exp, err)
	}

	claims, err := mgr.Verify(token, TokenTypeAccess)
	if err != nil {
		t.Fatalf("verify access: %v", err)
	}
	if claims.Subject != "user-1" || claims.Username != "alice" || claims.Type != TokenTypeAccess {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Fatalf("unexpected roles: %+v", claims.Roles)
	}
}

func TestJWTManagerIssueAndVerifyRefresh(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	token, exp, jti, err := mgr.IssueRefresh(IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil || token == "" || jti == "" || exp.Before(time.Now()) {
		t.Fatalf("issue refresh: token=%q jti=%q err=%v", token, jti, err)
	}

	claims, err := mgr.Verify(token, TokenTypeRefresh)
	if err != nil {
		t.Fatalf("verify refresh: %v", err)
	}
	if claims.ID != jti || claims.Type != TokenTypeRefresh {
		t.Fatalf("unexpected refresh claims: %+v", claims)
	}
}

func TestJWTManagerRejectsWrongTokenType(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	access, _, _ := mgr.IssueAccess(IssueOptions{UserID: "user-1"})
	refresh, _, _, _ := mgr.IssueRefresh(IssueOptions{UserID: "user-1"})

	if _, err := mgr.Verify(refresh, TokenTypeAccess); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected refresh-as-access to fail, got %v", err)
	}
	if _, err := mgr.Verify(access, TokenTypeRefresh); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected access-as-refresh to fail, got %v", err)
	}
}

func TestJWTManagerRejectsExpiredToken(t *testing.T) {
	mgr, err := NewJWTManager(Options{
		Secret:     testJWTSecret,
		Issuer:     "aiops-test",
		AccessTTL:  time.Millisecond,
		RefreshTTL: time.Hour,
		ClockSkew:  time.Second,
	})
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	token, _, err := mgr.IssueAccess(IssueOptions{UserID: "user-1"})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// leeway 为 1s，需等待 TTL + leeway 后才应判定过期。
	time.Sleep(1100 * time.Millisecond)
	if _, err := mgr.Verify(token, TokenTypeAccess); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected expired token to fail, got %v", err)
	}
}

func TestJWTManagerRejectsTamperedToken(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	other, _ := NewJWTManager(Options{Secret: "another-secret-with-enough-length-here", Issuer: "aiops-test"})
	token, _, _ := mgr.IssueAccess(IssueOptions{UserID: "user-1"})
	if _, err := other.Verify(token, TokenTypeAccess); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected wrong secret to fail, got %v", err)
	}
}

func TestJWTManagerVerifyRejectsEmptyToken(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	if _, err := mgr.Verify("", TokenTypeAccess); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected empty token to fail, got %v", err)
	}
}

func TestJWTAuthenticatorRejectsRefreshToken(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	refresh, _, _, _ := mgr.IssueRefresh(IssueOptions{UserID: "user-1", Username: "alice"})
	authn := NewJWTAuthenticator(mgr)
	if _, err := authn.Authenticate(refresh); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected refresh token rejected as access, got %v", err)
	}
}

func TestJWTAuthenticatorAcceptsAccessToken(t *testing.T) {
	mgr := newTestJWTManager(t, time.Hour, time.Hour)
	access, _, _ := mgr.IssueAccess(IssueOptions{UserID: "user-1", Username: "alice", Roles: []string{"admin"}})
	id, err := NewJWTAuthenticator(mgr).Authenticate(access)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if id.UserID != "user-1" || id.Username != "alice" || id.Roles[0] != "admin" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}
