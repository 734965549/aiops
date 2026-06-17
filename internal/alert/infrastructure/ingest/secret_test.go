package ingest

import "testing"

func TestVerifyWebhookSecret(t *testing.T) {
	secret := "my-webhook-secret-value"
	hash := HashWebhookSecret(secret)
	if !VerifyWebhookSecret(secret, hash) {
		t.Fatal("expected valid secret to match")
	}
	if VerifyWebhookSecret("wrong", hash) {
		t.Fatal("expected wrong secret to fail")
	}
	if VerifyWebhookSecret("", hash) || VerifyWebhookSecret(secret, "") {
		t.Fatal("expected empty inputs to fail")
	}
	if VerifyWebhookSecret(secret, "not-hex") {
		t.Fatal("expected invalid hash to fail")
	}
}
