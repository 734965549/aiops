package identityprovider

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/go-ldap/ldap/v3"
)

type mockLDAPConn struct {
	bindCalls []string
	searchRes *ldap.SearchResult
	searchErr error
	bindErr   map[string]error
}

func (m *mockLDAPConn) Bind(username, _ string) error {
	m.bindCalls = append(m.bindCalls, username)
	if m.bindErr != nil {
		if err, ok := m.bindErr[username]; ok {
			return err
		}
	}
	return nil
}

func (m *mockLDAPConn) Search(_ *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchRes, nil
}

func (m *mockLDAPConn) StartTLS(_ *tls.Config) error { return nil }
func (m *mockLDAPConn) Close() error                 { return nil }

type mockDialer struct {
	conn *mockLDAPConn
}

func (d *mockDialer) DialURL(_ string, _ ...ldap.DialOpt) (ldapConn, error) {
	return d.conn, nil
}

func TestLDAPProviderAuthenticateSuccess(t *testing.T) {
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
		"cn":       {"Alice"},
		"mail":     {"alice@example.com"},
		"memberOf": {"CN=Operators,OU=Groups,DC=example,DC=com"},
	})
	conn := &mockLDAPConn{
		searchRes: &ldap.SearchResult{Entries: []*ldap.Entry{entry}},
	}
	p, err := NewLDAPProvider(domain.ProviderInfo{
		ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Name: "LDAP", Enabled: true,
	}, config.LDAPProviderConfig{
		ServerURL:  "ldap://127.0.0.1:389",
		BindDN:     "cn=svc,dc=example,dc=com",
		BaseDN:     "ou=users,dc=example,dc=com",
		UserFilter: "(uid={username})",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	p.dialer = &mockDialer{conn: conn}

	user, err := p.Authenticate(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user.Username != "alice" || user.Email != "alice@example.com" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if len(user.Groups) != 1 {
		t.Fatalf("expected groups, got %+v", user.Groups)
	}
	if len(conn.bindCalls) != 2 {
		t.Fatalf("expected service bind + user bind, got %v", conn.bindCalls)
	}
}

func TestLDAPProviderAuthenticateWrongPassword(t *testing.T) {
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{"cn": {"Alice"}})
	conn := &mockLDAPConn{
		searchRes: &ldap.SearchResult{Entries: []*ldap.Entry{entry}},
		bindErr: map[string]error{
			"uid=alice,ou=users,dc=example,dc=com": errors.New("invalid credentials"),
		},
	}
	p, err := NewLDAPProvider(domain.ProviderInfo{ID: "corp-ldap", Type: domain.ProviderTypeLDAP}, config.LDAPProviderConfig{
		ServerURL: "ldap://127.0.0.1:389",
		BaseDN:    "ou=users,dc=example,dc=com",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	p.dialer = &mockDialer{conn: conn}
	if _, err := p.Authenticate(context.Background(), "alice", "bad"); err == nil {
		t.Fatal("expected auth failure")
	}
}

func TestBuildRegistryFromConfig(t *testing.T) {
	reg, err := BuildRegistryFromConfig("dev", []config.IdentityProviderConfig{
		{
			ID: "corp-ldap", Type: "ldap", Name: "LDAP", Enabled: true, Priority: 1,
			LDAP: config.LDAPProviderConfig{
				ServerURL: "ldap://127.0.0.1:389",
				BaseDN:    "dc=example,dc=com",
			},
		},
		{
			ID: "corp-oidc", Type: "oidc", Name: "OIDC", Enabled: true, Priority: 2,
			OIDC: config.OIDCProviderConfig{
				Issuer: "https://oidc.example.com", ClientID: "cid", RedirectURI: "http://localhost/cb",
			},
		},
	})
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	if len(reg.ListProviders()) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(reg.ListProviders()))
	}
}

func TestBuildRegistryProdRequiresTLS(t *testing.T) {
	_, err := BuildRegistryFromConfig("prod", []config.IdentityProviderConfig{
		{
			ID: "corp-ldap", Type: "ldap", Enabled: true,
			LDAP: config.LDAPProviderConfig{
				ServerURL: "ldap://127.0.0.1:389",
				BaseDN:    "dc=example,dc=com",
			},
		},
	})
	if err == nil {
		t.Fatal("expected prod ldap tls validation error")
	}
}

func TestBuildRegistryProdRejectsInsecureSkipVerify(t *testing.T) {
	_, err := BuildRegistryFromConfig("prod", []config.IdentityProviderConfig{
		{
			ID: "corp-ldap", Type: "ldap", Enabled: true,
			LDAP: config.LDAPProviderConfig{
				ServerURL:          "ldaps://127.0.0.1:636",
				BaseDN:             "dc=example,dc=com",
				InsecureSkipVerify: true,
			},
		},
	})
	if err == nil {
		t.Fatal("expected prod ldap insecure_skip_verify validation error")
	}
}
