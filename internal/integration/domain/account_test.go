package domain

import "testing"

func TestProviderType_IsValid(t *testing.T) {
	for _, p := range []ProviderType{ProviderHuaweiCloud, ProviderSigNoz, ProviderPrometheus} {
		if !p.IsValid() {
			t.Fatalf("expected %q to be valid", p)
		}
	}
	for _, p := range []ProviderType{"custom", "aws"} {
		if ProviderType(p).IsValid() {
			t.Fatalf("%q should not be valid in phase 1", p)
		}
	}
}

func TestAuthType_IsValid(t *testing.T) {
	for _, a := range []AuthType{AuthAKSK, AuthAgency, AuthAPIToken, AuthNone} {
		if !a.IsValid() {
			t.Fatalf("expected %q to be valid", a)
		}
	}
}

func TestProviderType_SupportsAuthType(t *testing.T) {
	if !ProviderHuaweiCloud.SupportsAuthType(AuthAKSK) {
		t.Fatal("expected huawei_cloud to support ak_sk")
	}
	if ProviderHuaweiCloud.SupportsAuthType(AuthAPIToken) {
		t.Fatal("expected huawei_cloud to reject api_token")
	}
	if !ProviderSigNoz.SupportsAuthType(AuthAPIToken) {
		t.Fatal("expected signoz to support api_token")
	}
	if ProviderSigNoz.SupportsAuthType(AuthAKSK) {
		t.Fatal("expected signoz to reject ak_sk")
	}
	if !ProviderPrometheus.SupportsAuthType(AuthAKSK) {
		t.Fatal("expected prometheus to support ak_sk")
	}
}
