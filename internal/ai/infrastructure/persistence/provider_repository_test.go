package persistence

import (
	"reflect"
	"testing"
)

func TestMarshalHeaders(t *testing.T) {
	raw, err := marshalHeaders(nil)
	if err != nil || string(raw) != "{}" {
		t.Fatalf("nil headers = %q, err=%v", string(raw), err)
	}
	raw, err = marshalHeaders(map[string]string{"X-Client": "aiops"})
	if err != nil {
		t.Fatalf("marshal headers: %v", err)
	}
	got, err := unmarshalHeaders(raw)
	if err != nil {
		t.Fatalf("unmarshal headers: %v", err)
	}
	if !reflect.DeepEqual(got, map[string]string{"X-Client": "aiops"}) {
		t.Fatalf("headers round-trip = %#v", got)
	}
}

func TestUnmarshalHeadersEmpty(t *testing.T) {
	got, err := unmarshalHeaders(nil)
	if err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("expected empty map, got %#v", got)
	}
}

func TestProviderModelTableName(t *testing.T) {
	if got := (providerModel{}).TableName(); got != "ai_provider" {
		t.Fatalf("table name = %q", got)
	}
}
