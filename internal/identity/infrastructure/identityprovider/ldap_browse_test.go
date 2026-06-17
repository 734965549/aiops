package identityprovider

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/go-ldap/ldap/v3"
)

func TestLDAPProviderBrowseOrganizations(t *testing.T) {
	entry := ldap.NewEntry("ou=operators,dc=example,dc=com", map[string][]string{"ou": {"Operators"}})
	conn := &mockLDAPConn{
		searchRes: &ldap.SearchResult{Entries: []*ldap.Entry{entry}},
	}
	p, err := NewLDAPProvider(domain.ProviderInfo{
		ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true,
	}, config.LDAPProviderConfig{
		ServerURL: "ldap://127.0.0.1:389",
		BindDN:    "cn=svc,dc=example,dc=com",
		BaseDN:    "dc=example,dc=com",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	p.dialer = &mockDialer{conn: conn}

	rows, err := p.BrowseOrganizations(context.Background(), "")
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "Operators" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestLDAPProviderListDirectoryUsers(t *testing.T) {
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
		"uid":  {"alice"},
		"cn":   {"Alice"},
		"mail": {"alice@example.com"},
	})
	conn := &mockLDAPConn{
		searchRes: &ldap.SearchResult{Entries: []*ldap.Entry{entry}},
	}
	p, err := NewLDAPProvider(domain.ProviderInfo{
		ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true,
	}, config.LDAPProviderConfig{
		ServerURL: "ldap://127.0.0.1:389",
		BaseDN:    "ou=users,dc=example,dc=com",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	p.dialer = &mockDialer{conn: conn}

	rows, err := p.ListDirectoryUsers(context.Background(), "ou=users,dc=example,dc=com", 50)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(rows) != 1 || rows[0].ExternalUsername != "alice" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestRegistryLDAPProvider(t *testing.T) {
	reg := NewRegistry()
	ldapProvider, err := NewLDAPProvider(domain.ProviderInfo{
		ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true,
	}, config.LDAPProviderConfig{
		ServerURL: "ldap://127.0.0.1:389",
		BaseDN:    "dc=example,dc=com",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	reg.RegisterPassword(ldapProvider)
	got, ok := reg.LDAPProvider("corp-ldap")
	if !ok || got == nil {
		t.Fatal("expected ldap provider")
	}
	if _, ok := reg.LDAPProvider("corp-oauth2"); ok {
		t.Fatal("oauth provider should not be returned as ldap")
	}
}
