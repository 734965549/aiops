package identityprovider

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/go-ldap/ldap/v3"
)

func TestEntryExternalSubjectUsesEntryUUID(t *testing.T) {
	p := &LDAPProvider{
		info: domain.ProviderInfo{Type: domain.ProviderTypeLDAP},
		cfg:  config.LDAPProviderConfig{AttrSubject: "entryUUID"},
	}
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
		"entryUUID": {"550e8400-e29b-41d4-a716-446655440000"},
	})
	got := p.entryExternalSubject(entry)
	want := "550e8400-e29b-41d4-a716-446655440000"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEntryExternalSubjectUsesObjectGUID(t *testing.T) {
	p := &LDAPProvider{
		info: domain.ProviderInfo{Type: domain.ProviderTypeAD},
		cfg:  config.LDAPProviderConfig{},
	}
	want := "550e8400-e29b-41d4-a716-446655440000"
	raw, err := guidStringToADBytes(want)
	if err != nil {
		t.Fatalf("guid bytes: %v", err)
	}
	entry := ldap.NewEntry("cn=alice,dc=corp,dc=local", nil)
	entry.Attributes = []*ldap.EntryAttribute{{
		Name: "objectGUID", ByteValues: [][]byte{raw},
	}}

	got := p.entryExternalSubject(entry)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEntryExternalSubjectFallsBackToDN(t *testing.T) {
	p := &LDAPProvider{
		info: domain.ProviderInfo{Type: domain.ProviderTypeLDAP},
	}
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
		"uid": {"alice"},
	})
	got := p.entryExternalSubject(entry)
	if got != entry.DN {
		t.Fatalf("got %q want dn %q", got, entry.DN)
	}
}

func TestLDAPProviderAuthenticateUsesStableSubject(t *testing.T) {
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
		"cn":        {"Alice"},
		"mail":      {"alice@example.com"},
		"entryUUID": {"550e8400-e29b-41d4-a716-446655440000"},
	})
	conn := &mockLDAPConn{
		searchRes: &ldap.SearchResult{Entries: []*ldap.Entry{entry}},
	}
	p, err := NewLDAPProvider(domain.ProviderInfo{
		ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true,
	}, config.LDAPProviderConfig{
		ServerURL:   "ldap://127.0.0.1:389",
		BaseDN:      "ou=users,dc=example,dc=com",
		UserFilter:  "(uid={username})",
		AttrSubject: "entryUUID",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	p.dialer = &mockDialer{conn: conn}

	user, err := p.Authenticate(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user.ExternalSubject != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected subject: %q", user.ExternalSubject)
	}
}

func TestLDAPProviderListDirectoryUsersUsesStableSubject(t *testing.T) {
	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=com", map[string][]string{
		"uid":       {"alice"},
		"cn":        {"Alice"},
		"mail":      {"alice@example.com"},
		"entryUUID": {"550e8400-e29b-41d4-a716-446655440000"},
	})
	conn := &mockLDAPConn{
		searchRes: &ldap.SearchResult{Entries: []*ldap.Entry{entry}},
	}
	p, err := NewLDAPProvider(domain.ProviderInfo{
		ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true,
	}, config.LDAPProviderConfig{
		ServerURL:   "ldap://127.0.0.1:389",
		BaseDN:      "ou=users,dc=example,dc=com",
		AttrSubject: "entryUUID",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	p.dialer = &mockDialer{conn: conn}

	rows, err := p.ListDirectoryUsers(context.Background(), "ou=users,dc=example,dc=com", 50)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if rows[0].ExternalSubject != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected subject: %q", rows[0].ExternalSubject)
	}
	if rows[0].DN != entry.DN {
		t.Fatalf("unexpected dn: %q", rows[0].DN)
	}
}

func TestObjectSIDRoundTrip(t *testing.T) {
	want := "S-1-5-21-0-500"
	raw, err := parseObjectSIDString(want)
	if err != nil {
		t.Fatalf("parse sid: %v", err)
	}
	if got := formatObjectSID(raw); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
