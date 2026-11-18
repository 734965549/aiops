package domain

import (
	"testing"
	"time"
)

func TestSessionShareUsable(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	share := SessionShare{Status: ShareStatusActive, ExpiresAt: now.Add(time.Hour)}
	if err := share.Usable(now); err != nil {
		t.Fatalf("active share should be usable: %v", err)
	}

	share.Status = ShareStatusRevoked
	if err := share.Usable(now); err != ErrShareRevoked {
		t.Fatalf("revoked share err = %v", err)
	}

	share.Status = ShareStatusActive
	share.ExpiresAt = now
	if err := share.Usable(now); err != ErrShareExpired {
		t.Fatalf("expired share err = %v", err)
	}
}

func TestShareTokenHashIsStableAndHidesToken(t *testing.T) {
	token, err := NewShareToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if len(token) < 40 {
		t.Fatalf("token too short: %q", token)
	}
	first := ShareTokenHash(token)
	if first != ShareTokenHash(token) {
		t.Fatal("hash must be stable")
	}
	if len(first) != 64 {
		t.Fatalf("hash length = %d", len(first))
	}
	if first == token || first == "" {
		t.Fatal("hash must not equal the token")
	}
	if ShareTokenHash("  "+token+"  ") != first {
		t.Fatal("hash must trim surrounding spaces")
	}
	if ShareTokenHash(token+"x") == first {
		t.Fatal("different tokens must hash differently")
	}
}
