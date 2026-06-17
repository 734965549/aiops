package auth

import "testing"

func TestIPAllowlistAllowsExactAndCIDR(t *testing.T) {
	list, err := NewIPAllowlist([]string{"192.0.2.10", "198.51.100.0/24"})
	if err != nil {
		t.Fatalf("NewIPAllowlist: %v", err)
	}
	if !list.Enabled() {
		t.Fatal("expected allowlist to be enabled")
	}
	if !list.Allows("192.0.2.10") {
		t.Fatal("expected exact IP to be allowed")
	}
	if !list.Allows("198.51.100.42") {
		t.Fatal("expected CIDR IP to be allowed")
	}
	if list.Allows("203.0.113.10") {
		t.Fatal("expected outside IP to be denied")
	}
}

func TestIPAllowlistEmptyDisabled(t *testing.T) {
	list, err := NewIPAllowlist(nil)
	if err != nil {
		t.Fatalf("NewIPAllowlist: %v", err)
	}
	if list.Enabled() {
		t.Fatal("expected empty allowlist to be disabled")
	}
	if !list.Allows("not-an-ip") {
		t.Fatal("disabled allowlist should allow all clients")
	}
}

func TestIPAllowlistRejectsInvalidEntry(t *testing.T) {
	if _, err := NewIPAllowlist([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected invalid entry error")
	}
}
