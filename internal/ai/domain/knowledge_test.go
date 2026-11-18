package domain

import (
	"strings"
	"testing"
	"time"
)

func TestKnowledgeEnumsValidity(t *testing.T) {
	for _, s := range []KnowledgeStatus{KnowledgeStatusDraft, KnowledgeStatusActive, KnowledgeStatusDisabled} {
		if !s.IsValid() {
			t.Fatalf("expected valid knowledge status %q", s)
		}
	}
	if KnowledgeStatus("archived").IsValid() {
		t.Fatal("unexpected valid status")
	}
	for _, s := range []KnowledgeScope{KnowledgeScopeGlobal, KnowledgeScopeApplication, KnowledgeScopeResourceType} {
		if !s.IsValid() {
			t.Fatalf("expected valid scope %q", s)
		}
	}
	if KnowledgeScope("team").IsValid() {
		t.Fatal("unexpected valid scope")
	}
	for _, o := range []KnowledgeOrigin{KnowledgeOriginFeedbackAggregate, KnowledgeOriginManual} {
		if !o.IsValid() {
			t.Fatalf("expected valid origin %q", o)
		}
	}
	if KnowledgeOrigin("import").IsValid() {
		t.Fatal("unexpected valid origin")
	}
}

func TestKnowledgeItemValidate(t *testing.T) {
	base := KnowledgeItem{
		Scope: KnowledgeScopeGlobal, Title: "标题", Content: "内容", Origin: KnowledgeOriginManual,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("global item should be valid: %v", err)
	}
	if err := (KnowledgeItem{Scope: KnowledgeScopeGlobal, ScopeRef: "app-1", Title: "t", Content: "c", Origin: KnowledgeOriginManual}).Validate(); err == nil {
		t.Fatal("global scope must not carry scope_ref")
	}
	if err := (KnowledgeItem{Scope: KnowledgeScopeApplication, Title: "t", Content: "c", Origin: KnowledgeOriginManual}).Validate(); err == nil {
		t.Fatal("non-global scope requires scope_ref")
	}
	if err := (KnowledgeItem{Scope: KnowledgeScopeApplication, ScopeRef: "app-1", Content: "c", Origin: KnowledgeOriginManual}).Validate(); err == nil {
		t.Fatal("title is required")
	}
	if err := (KnowledgeItem{Scope: KnowledgeScopeApplication, ScopeRef: "app-1", Title: "t", Origin: KnowledgeOriginManual}).Validate(); err == nil {
		t.Fatal("content is required")
	}
}

func TestKnowledgeItemNormalize(t *testing.T) {
	item := KnowledgeItem{
		Scope: KnowledgeScopeApplication, ScopeRef: " app-1 ", Title: strings.Repeat("标", 200),
		Content: strings.Repeat("内", 3000), Origin: KnowledgeOriginManual,
	}.Normalize()
	if item.ScopeRef != "app-1" {
		t.Fatalf("scope_ref = %q", item.ScopeRef)
	}
	if len([]rune(item.Title)) != KnowledgeMaxTitleRunes {
		t.Fatalf("title runes = %d", len([]rune(item.Title)))
	}
	if len([]rune(item.Content)) != KnowledgeMaxContentRunes {
		t.Fatalf("content runes = %d", len([]rune(item.Content)))
	}
}

func TestKnowledgeItemShouldAutoDisable(t *testing.T) {
	active := KnowledgeItem{Status: KnowledgeStatusActive, HitCount: KnowledgeAutoDisableMinHits, UsefulCount: 0}
	if !active.ShouldAutoDisable() {
		t.Fatal("active item with hits but no useful marks should auto disable")
	}
	useful := KnowledgeItem{Status: KnowledgeStatusActive, HitCount: KnowledgeAutoDisableMinHits, UsefulCount: 1}
	if useful.ShouldAutoDisable() {
		t.Fatal("useful item must not be auto disabled")
	}
	fewHits := KnowledgeItem{Status: KnowledgeStatusActive, HitCount: KnowledgeAutoDisableMinHits - 1}
	if fewHits.ShouldAutoDisable() {
		t.Fatal("item below threshold must stay active")
	}
	draft := KnowledgeItem{Status: KnowledgeStatusDraft, HitCount: 99}
	if draft.ShouldAutoDisable() {
		t.Fatal("draft items are not auto disabled")
	}
}

func TestKnowledgeItemMatchesTarget(t *testing.T) {
	global := KnowledgeItem{Scope: KnowledgeScopeGlobal}
	app := KnowledgeItem{Scope: KnowledgeScopeApplication, ScopeRef: "app-1"}
	res := KnowledgeItem{Scope: KnowledgeScopeResourceType, ScopeRef: "ecs"}
	cases := []struct {
		item           KnowledgeItem
		appID, resType string
		want           bool
	}{
		{global, "", "", true},
		{app, "app-1", "ecs", true},
		{app, "app-2", "ecs", false},
		{app, "", "ecs", false},
		{res, "app-1", "ecs", true},
		{res, "app-1", "rds", false},
	}
	for i, tc := range cases {
		if got := tc.item.MatchesTarget(tc.appID, tc.resType); got != tc.want {
			t.Fatalf("case %d: got %v want %v", i, got, tc.want)
		}
	}
}

func TestSelectKnowledgeForInjection(t *testing.T) {
	now := time.Now()
	items := []KnowledgeItem{
		{ItemID: "draft", Status: KnowledgeStatusDraft, Scope: KnowledgeScopeGlobal, Title: "d", Content: "d"},
		{ItemID: "disabled", Status: KnowledgeStatusDisabled, Scope: KnowledgeScopeGlobal, Title: "x", Content: "x"},
		{ItemID: "hot", Status: KnowledgeStatusActive, Scope: KnowledgeScopeGlobal, Title: "热", Content: "热知识", HitCount: 9, UpdatedAt: now},
		{ItemID: "cold", Status: KnowledgeStatusActive, Scope: KnowledgeScopeGlobal, Title: "冷", Content: "冷知识", HitCount: 1, UpdatedAt: now},
		{ItemID: "app", Status: KnowledgeStatusActive, Scope: KnowledgeScopeApplication, ScopeRef: "app-1", Title: "a", Content: "a", HitCount: 5, UpdatedAt: now},
	}
	got := SelectKnowledgeForInjection(items, "app-1", "")
	if len(got) != 3 {
		t.Fatalf("selected %d items: %+v", len(got), got)
	}
	// 命中次数降序：hot(9) → app(5) → cold(1)，且 draft/disabled 不参与。
	if got[0].ItemID != "hot" || got[1].ItemID != "app" || got[2].ItemID != "cold" {
		t.Fatalf("unexpected order: %+v", got)
	}
	// 其他应用只命中 global。
	other := SelectKnowledgeForInjection(items, "app-2", "")
	if len(other) != 2 || other[0].ItemID != "hot" {
		t.Fatalf("unexpected selection for other app: %+v", other)
	}
}

func TestSelectKnowledgeForInjectionCaps(t *testing.T) {
	now := time.Now()
	items := make([]KnowledgeItem, 0, 8)
	// 每条内容都很长，确保总长上限生效（3000 runes）。
	long := strings.Repeat("知", 900)
	for i := 0; i < 8; i++ {
		items = append(items, KnowledgeItem{
			ItemID: "k" + string(rune('a'+i)), Status: KnowledgeStatusActive, Scope: KnowledgeScopeGlobal,
			Title: "t", Content: long, HitCount: 10 - i, UpdatedAt: now,
		})
	}
	got := SelectKnowledgeForInjection(items, "", "")
	if len(got) == 0 || len(got) >= 5 {
		t.Fatalf("expected length cap by runes, got %d", len(got))
	}
	total := 0
	for _, item := range got {
		total += len([]rune(item.Title)) + len([]rune(item.Content))
	}
	if total > KnowledgeMaxInjectedRunes {
		t.Fatalf("appendix exceeds rune budget: %d", total)
	}
}

func TestKnowledgeAppendix(t *testing.T) {
	if KnowledgeAppendix(nil) != "" {
		t.Fatal("empty items must yield empty appendix")
	}
	appendix := KnowledgeAppendix([]KnowledgeItem{{ItemID: "item-1", Title: "发布导致 CPU 升高", Content: "先查发布记录"}})
	if !strings.Contains(appendix, "item-1") || !strings.Contains(appendix, "先查发布记录") {
		t.Fatalf("unexpected appendix: %s", appendix)
	}
	if !strings.Contains(appendix, "不得照搬结论") {
		t.Fatalf("appendix must keep the reference-only constraint: %s", appendix)
	}
}
