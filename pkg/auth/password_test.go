package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestHashPasswordAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$2a$") {
		t.Fatalf("expected bcrypt prefix, got %q", hash)
	}
	if err := VerifyPassword(hash, "correct-horse-battery"); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyPasswordMismatch(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	err = VerifyPassword(hash, "wrong-password")
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}

func TestVerifyPasswordRejectsEmptyInputs(t *testing.T) {
	hash, _ := HashPassword("secret123")
	for _, tc := range []struct {
		name  string
		hash  string
		plain string
	}{
		{name: "empty hash", hash: "", plain: "secret123"},
		{name: "empty plain", hash: hash, plain: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyPassword(tc.hash, tc.plain); !errors.Is(err, ErrPasswordMismatch) {
				t.Fatalf("expected ErrPasswordMismatch, got %v", err)
			}
		})
	}
}
