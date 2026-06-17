package errors

import (
	"errors"
	"testing"
)

var errSentinel = errors.New("sentinel")

func TestMapSentinels(t *testing.T) {
	mapped := MapSentinels(errSentinel, "internal", Sentinel{Err: errSentinel, Code: CodeNotFound})
	if mapped == nil {
		t.Fatal("expected mapped error")
	}
	e := FromError(mapped)
	if e.Code != CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %s", e.Code)
	}

	wrapped := MapSentinels(errors.New("db down"), "load failed")
	e = FromError(wrapped)
	if e.Code != CodeInternal {
		t.Fatalf("expected INTERNAL, got %s", e.Code)
	}
	if e.Message != "load failed" {
		t.Fatalf("unexpected message: %q", e.Message)
	}
}
