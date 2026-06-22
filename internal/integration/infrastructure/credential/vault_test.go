package credential

import (
	"testing"

	"github.com/734965549/aiops/internal/integration/domain"
)

func TestVaultEncryptDecryptWithKeyVersion(t *testing.T) {
	v, err := NewVault("test-credential-encryption-key", 2)
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	material := domain.CredentialMaterial{"access_key": "AK", "secret_key": "SK"}
	ciphertext, fp, err := v.Encrypt(material)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ciphertext) == 0 || ciphertext[0] != 2 {
		t.Fatalf("expected version byte 2, got %v", ciphertext[0])
	}
	if fp == "" {
		t.Fatal("expected fingerprint")
	}
	out, err := v.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out["access_key"] != "AK" || out["secret_key"] != "SK" {
		t.Fatalf("unexpected material: %+v", out)
	}
}

func TestVaultDecryptRejectsWrongKeyVersion(t *testing.T) {
	v1, err := NewVault("key-one-for-version-test", 1)
	if err != nil {
		t.Fatalf("new vault v1: %v", err)
	}
	v2, err := NewVault("key-two-for-version-test", 2)
	if err != nil {
		t.Fatalf("new vault v2: %v", err)
	}
	material := domain.CredentialMaterial{"api_token": "tok"}
	ciphertext, _, err := v1.Encrypt(material)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := v2.Decrypt(ciphertext); err == nil {
		t.Fatal("expected version mismatch error")
	}
}

func TestVaultEncryptRejectsEmptyMaterial(t *testing.T) {
	v, err := NewVault("test-credential-encryption-key", 1)
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	if _, _, err := v.Encrypt(nil); err != domain.ErrCredentialRequired {
		t.Fatalf("expected ErrCredentialRequired, got %v", err)
	}
}

func TestNewVaultRejectsEmptyKey(t *testing.T) {
	if _, err := NewVault(" ", 1); err == nil {
		t.Fatal("expected empty key error")
	}
}
