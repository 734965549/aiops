package huawei

import (
	"context"
	"errors"
	"testing"
)

func TestHistoryTotalCountUnknown(t *testing.T) {
	if got := historyTotalCount(nil); got != -1 {
		t.Fatalf("unknown total = %d, want -1", got)
	}
	value := int32(7)
	if got := historyTotalCount(&value); got != 7 {
		t.Fatalf("total = %d, want 7", got)
	}
}

func TestNewHistoryRDSClientRejectsExpiredContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newHistoryRDSClient(ctx, AKSKCredential{}, "p-1", "cn-north-4")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
