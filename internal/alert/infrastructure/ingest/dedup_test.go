package ingest

import "testing"

func TestComputeDedupKey_WithFingerprint(t *testing.T) {
	k1 := ComputeDedupKey("src1", "fp-abc", "", "", nil)
	k2 := ComputeDedupKey("src1", "fp-abc", "", "", nil)
	if k1 != k2 || k1 == "" {
		t.Fatalf("expected stable dedup key, got %q %q", k1, k2)
	}
	k3 := ComputeDedupKey("src2", "fp-abc", "", "", nil)
	if k3 == k1 {
		t.Fatal("different source should produce different key")
	}
}
