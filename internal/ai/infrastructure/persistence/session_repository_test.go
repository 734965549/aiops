package persistence

import (
	"encoding/json"
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
)

func TestSessionTableNames(t *testing.T) {
	if got := (sessionModel{}).TableName(); got != "ai_session" {
		t.Fatalf("session table name = %q", got)
	}
	if got := (sessionMessageModel{}).TableName(); got != "ai_session_message" {
		t.Fatalf("message table name = %q", got)
	}
}

func TestSessionContextRoundTrip(t *testing.T) {
	in := domain.SessionContext{
		Page: "alerts", PageTitle: "告警中心", AlertID: "alert-1",
		ApplicationID: "app-1", ResourceID: "res-1", ResourceType: "pod", TimeRange: "30m",
	}
	raw, err := marshalContext(in)
	if err != nil {
		t.Fatalf("marshal context: %v", err)
	}
	out, err := unmarshalContext(raw)
	if err != nil {
		t.Fatalf("unmarshal context: %v", err)
	}
	if out != in {
		t.Fatalf("context round-trip = %+v, want %+v", out, in)
	}
	empty, err := unmarshalContext(nil)
	if err != nil || !empty.IsEmpty() {
		t.Fatalf("empty context = %+v, err=%v", empty, err)
	}
}

func TestMessageCollectionsRoundTrip(t *testing.T) {
	calls := []domain.ToolCall{{ToolCode: "alarm.analyze", Allowed: true, Mode: "read_only", Status: "ok"}}
	rawCalls, err := marshalToolCalls(calls)
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}
	gotCalls, err := unmarshalToolCalls(rawCalls)
	if err != nil {
		t.Fatalf("unmarshal tool calls: %v", err)
	}
	if len(gotCalls) != 1 || gotCalls[0].ToolCode != "alarm.analyze" || !gotCalls[0].Allowed {
		t.Fatalf("tool calls round-trip = %+v", gotCalls)
	}

	refs := []domain.Reference{{Type: "alert", ID: "alert-1", Label: "CPU 高", URL: "/alerts?alert_id=alert-1"}}
	rawRefs, err := marshalReferences(refs)
	if err != nil {
		t.Fatalf("marshal references: %v", err)
	}
	gotRefs, err := unmarshalReferences(rawRefs)
	if err != nil {
		t.Fatalf("unmarshal references: %v", err)
	}
	if len(gotRefs) != 1 || gotRefs[0].URL == "" {
		t.Fatalf("references round-trip = %+v", gotRefs)
	}

	emptyCalls, err := unmarshalToolCalls(nil)
	if err != nil || len(emptyCalls) != 0 {
		t.Fatalf("nil tool calls = %+v, err=%v", emptyCalls, err)
	}
	emptyRefs, err := unmarshalReferences([]byte("null"))
	if err != nil || len(emptyRefs) != 0 {
		t.Fatalf("null references = %+v, err=%v", emptyRefs, err)
	}

	if _, err := unmarshalToolCalls([]byte("{bad")); err == nil {
		t.Fatal("expected invalid tool calls json error")
	}
	var probe []map[string]any
	if err := json.Unmarshal([]byte("[]"), &probe); err != nil {
		t.Fatalf("sanity json: %v", err)
	}
}
