package domain

import "testing"

func TestPreserveUserRoleSource(t *testing.T) {
	tests := []struct {
		current  UserRoleSource
		incoming UserRoleSource
		want     UserRoleSource
	}{
		{UserRoleSourceManual, UserRoleSourceExternalGroup, UserRoleSourceManual},
		{UserRoleSourceLDAPImport, UserRoleSourceExternalGroup, UserRoleSourceLDAPImport},
		{UserRoleSourceExternalGroup, UserRoleSourceExternalGroup, UserRoleSourceExternalGroup},
		{UserRoleSourceExternalGroup, UserRoleSourceLDAPImport, UserRoleSourceLDAPImport},
		{"", UserRoleSourceExternalGroup, UserRoleSourceManual},
	}
	for _, tc := range tests {
		got := PreserveUserRoleSource(tc.current, tc.incoming)
		if got != tc.want {
			t.Fatalf("PreserveUserRoleSource(%q, %q) = %q, want %q", tc.current, tc.incoming, got, tc.want)
		}
	}
}
