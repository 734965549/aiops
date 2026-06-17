package oauthstate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
)

func TestMemoryStoreIssueAndConsume(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	state, err := store.Issue(ctx, "corp-oauth", Binding{}, time.Minute)
	if err != nil || state == "" {
		t.Fatalf("issue: state=%q err=%v", state, err)
	}
	if err := store.Consume(ctx, state, "corp-oauth", Binding{}); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := store.Consume(ctx, state, "corp-oauth", Binding{}); !errors.Is(err, domain.ErrOAuthStateNotFound) {
		t.Fatalf("expected one-time consume, got %v", err)
	}
}

func TestMemoryStoreRejectProviderMismatch(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	state, err := store.Issue(ctx, "corp-oauth", Binding{}, time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := store.Consume(ctx, state, "other-oauth", Binding{}); !errors.Is(err, domain.ErrOAuthStateInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
	if err := store.Consume(ctx, state, "corp-oauth", Binding{}); err != nil {
		t.Fatalf("state should remain consumable after mismatch, got %v", err)
	}
}

func TestMemoryStoreExpire(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	state, err := store.Issue(ctx, "corp-oauth", Binding{}, time.Millisecond)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.Consume(ctx, state, "corp-oauth", Binding{}); !errors.Is(err, domain.ErrOAuthStateNotFound) {
		t.Fatalf("expected expired state, got %v", err)
	}
}

func TestMemoryStoreRejectBindingMismatch(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	binding := Binding{ClientIP: "192.0.2.1", UserAgent: "browser-a"}
	state, err := store.Issue(ctx, "corp-oauth", binding, time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := store.Consume(ctx, state, "corp-oauth", Binding{ClientIP: "192.0.2.2", UserAgent: "browser-a"}); !errors.Is(err, domain.ErrOAuthStateInvalid) {
		t.Fatalf("expected invalid for binding mismatch, got %v", err)
	}
	if err := store.Consume(ctx, state, "corp-oauth", binding); err != nil {
		t.Fatalf("state should remain consumable after binding mismatch, got %v", err)
	}
}
