package domain

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

func TestComputeDedupKey_UsesSelectedLabelsWithoutExternalID(t *testing.T) {
	labels := map[string]string{"alertname": "HighCPU", "instance": "i-1", "noise": "x"}
	k1 := ComputeDedupKey("src1", "", "HighCPU", "host-1", labels)
	labels["noise"] = "y"
	k2 := ComputeDedupKey("src1", "", "HighCPU", "host-1", labels)
	if k1 != k2 {
		t.Fatal("unstable labels must not affect dedup key")
	}
	if ComputeDedupKey("src1", "", "HighCPU", "host-2", labels) == k1 {
		t.Fatal("different resource should produce different key")
	}
}

func TestComputeCESDedupKey_DistinguishesScopeAndRecord(t *testing.T) {
	base := ComputeCESDedupKey("src", "acc", "cn-north-4", "proj-1", "ah-1")
	cases := map[string]string{
		"source":  ComputeCESDedupKey("src2", "acc", "cn-north-4", "proj-1", "ah-1"),
		"account": ComputeCESDedupKey("src", "acc2", "cn-north-4", "proj-1", "ah-1"),
		"region":  ComputeCESDedupKey("src", "acc", "cn-south-1", "proj-1", "ah-1"),
		"project": ComputeCESDedupKey("src", "acc", "cn-north-4", "proj-2", "ah-1"),
		"record":  ComputeCESDedupKey("src", "acc", "cn-north-4", "proj-1", "ah-2"),
	}
	for name, got := range cases {
		if got == base {
			t.Fatalf("%s change must produce a different dedup key", name)
		}
	}
	if base != ComputeCESDedupKey(" src ", " acc ", "CN-NORTH-4", " proj-1 ", " ah-1 ") {
		t.Fatal("expected normalization to keep key stable")
	}
}
