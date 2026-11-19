package domain

import (
	"strings"
	"testing"
)

func TestSessionStatusAndRoleValidity(t *testing.T) {
	for _, s := range []SessionStatus{SessionStatusActive, SessionStatusArchived} {
		if !s.IsValid() {
			t.Fatalf("expected valid session status %q", s)
		}
	}
	if SessionStatus("deleted").IsValid() {
		t.Fatal("unexpected valid session status")
	}
	for _, r := range []MessageRole{RoleUser, RoleAssistant} {
		if !r.IsValid() {
			t.Fatalf("expected valid role %q", r)
		}
	}
	if MessageRole("system").IsValid() {
		t.Fatal("unexpected valid role")
	}
	for _, f := range []Feedback{FeedbackNone, FeedbackValid, FeedbackInvalid} {
		if !f.IsValid() {
			t.Fatalf("expected valid feedback %q", f)
		}
	}
	if Feedback("maybe").IsValid() {
		t.Fatal("unexpected valid feedback")
	}
}

func TestNewSessionTitle(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "collapse whitespace", content: "  分析   payment-service \n 告警 ", want: "分析 payment-service 告警"},
		{name: "empty", content: "   ", want: "新会话"},
		{name: "truncate to rune limit", content: strings.Repeat("告警", 40), want: strings.Repeat("告警", 30)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewSessionTitle(tc.content); got != tc.want {
				t.Fatalf("NewSessionTitle(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestSessionContextNormalizeAndIsEmpty(t *testing.T) {
	empty := SessionContext{Page: "  ", PageTitle: " AI 助手 "}
	if !empty.IsEmpty() {
		t.Fatal("expected empty context")
	}
	got := SessionContext{
		Page:          "  alerts ",
		PageTitle:     strings.Repeat("a", 200),
		AlertID:       " alert-1 ",
		ApplicationID: " app-1 ",
		ResourceID:    " res-1 ",
		ResourceType:  " pod ",
		ExecutionID:   " exec-1 ",
		TimeRange:     " 30m ",
		ProviderID:    " provider-b1 ",
	}.Normalize()
	if got.Page != "alerts" || got.AlertID != "alert-1" || got.ApplicationID != "app-1" ||
		got.ResourceID != "res-1" || got.ResourceType != "pod" || got.TimeRange != "30m" {
		t.Fatalf("unexpected normalized context: %+v", got)
	}
	if got.ProviderID != "provider-b1" {
		t.Fatalf("provider_id not normalized: %+v", got)
	}
	if got.ExecutionID != "exec-1" {
		t.Fatalf("execution_id not normalized: %+v", got)
	}
	if len([]rune(got.PageTitle)) != 128 {
		t.Fatalf("expected page title truncated to 128 runes, got %d", len([]rune(got.PageTitle)))
	}
	if got.IsEmpty() {
		t.Fatal("expected non-empty context")
	}
	// 仅执行任务上下文的会话也必须被视为非空（执行页场景）。
	if (SessionContext{ExecutionID: "exec-1"}).IsEmpty() {
		t.Fatal("execution_id alone must make the context non-empty")
	}
	if len([]rune((SessionContext{ExecutionID: strings.Repeat("e", 100)}).Normalize().ExecutionID)) != 64 {
		t.Fatal("execution_id must be truncated to 64 runes")
	}
	if len([]rune((SessionContext{ProviderID: strings.Repeat("p", 100)}).Normalize().ProviderID)) != 64 {
		t.Fatal("provider_id must be truncated to 64 runes")
	}
	// 仅模型选择不算资源上下文，避免前端误判“已带入告警/资源”。
	if !(SessionContext{ProviderID: "provider-b1"}).IsEmpty() {
		t.Fatal("provider_id alone must not make the context non-empty")
	}
}
