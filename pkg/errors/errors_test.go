package errors

import (
	"errors"
	"testing"
)

func TestFromError_PreservesTypedError(t *testing.T) {
	orig := New(CodeNotFound, "alert not found")
	got := FromError(orig)
	if got != orig {
		t.Fatalf("expected same pointer, got %p vs %p", got, orig)
	}
}

func TestFromError_SanitizesPlainError(t *testing.T) {
	raw := errors.New("redis: connection refused")
	got := FromError(raw)
	if got.Code != CodeInternal {
		t.Fatalf("expected INTERNAL, got %s", got.Code)
	}
	if got.Message != InternalMessage {
		t.Fatalf("expected sanitized message %q, got %q", InternalMessage, got.Message)
	}
	if got.Unwrap() != raw {
		t.Fatalf("expected cause preserved")
	}
}

func TestCodeAndMessageOf(t *testing.T) {
	if got := CodeOf(nil); got != CodeOK {
		t.Fatalf("nil code = %s, want OK", got)
	}
	if got := MessageOf(nil); got != "ok" {
		t.Fatalf("nil message = %q, want ok", got)
	}

	raw := errors.New("redis: connection refused")
	if got := CodeOf(raw); got != CodeInternal {
		t.Fatalf("plain code = %s, want INTERNAL", got)
	}
	if got := MessageOf(raw); got != InternalMessage {
		t.Fatalf("plain message = %q, want sanitized", got)
	}
}
