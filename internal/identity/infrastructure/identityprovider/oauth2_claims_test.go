package identityprovider

import (
	"reflect"
	"testing"
)

func TestPickGroupsFromStringArray(t *testing.T) {
	raw := map[string]any{
		"groups": []any{"Operators", "Admins", "Operators"},
	}
	got := pickGroups(raw, "", "groups")
	want := []string{"Operators", "Admins"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestPickGroupsFromCommaString(t *testing.T) {
	raw := map[string]any{
		"roles": "operator, admin ,viewer",
	}
	got := pickGroups(raw, "roles")
	want := []string{"operator", "admin", "viewer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestPickGroupsPrefersConfiguredClaim(t *testing.T) {
	raw := map[string]any{
		"groups":   []any{"ignored"},
		"memberOf": []any{"CN=Operators,OU=Groups,DC=example,DC=com"},
		"roles":    []any{"viewer"},
	}
	got := pickGroups(raw, "memberOf", "groups", "roles")
	want := []string{"CN=Operators,OU=Groups,DC=example,DC=com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestPickGroupsReturnsNilWhenMissing(t *testing.T) {
	if got := pickGroups(map[string]any{"email": "a@b.com"}, "", "groups"); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
