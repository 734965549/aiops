package domain

import (
	"strings"
	"testing"
)

func TestMemoryCaseStatusAndSourceValidity(t *testing.T) {
	for _, s := range []MemoryCaseStatus{MemoryCaseStatusActive, MemoryCaseStatusRetired} {
		if !s.IsValid() {
			t.Fatalf("expected valid memory status %q", s)
		}
	}
	if MemoryCaseStatus("archived").IsValid() {
		t.Fatal("unexpected valid memory status")
	}
	for _, s := range []MemoryCaseSource{MemoryCaseSourceChatMessage, MemoryCaseSourceAlertAnalysis, MemoryCaseSourceKnowledgeItem} {
		if !s.IsValid() {
			t.Fatalf("expected valid memory source %q", s)
		}
	}
	if MemoryCaseSource("manual").IsValid() {
		t.Fatal("unexpected valid memory source")
	}
}

func TestMemoryCaseApplyFeedback(t *testing.T) {
	// 案例由有效反馈创建，初始质量分为 +1。
	fresh := MemoryCase{QualityScore: MemoryCaseValidDelta, Status: MemoryCaseStatusActive}
	if score, status := fresh.ApplyFeedback(true); score != 2*MemoryCaseValidDelta || status != MemoryCaseStatusActive {
		t.Fatalf("valid feedback => (%d, %s)", score, status)
	}
	// 单次无效反馈降分但不淘汰（+1 -> -1）。
	score, status := fresh.ApplyFeedback(false)
	if score != MemoryCaseValidDelta+MemoryCaseInvalidDelta || status != MemoryCaseStatusActive {
		t.Fatalf("first invalid feedback must keep active => (%d, %s)", score, status)
	}
	// 再次无效反馈达到阈值，转 retired。
	next := MemoryCase{QualityScore: score, Status: status}
	if score, status := next.ApplyFeedback(false); score > MemoryCaseRetireScore || status != MemoryCaseStatusRetired {
		t.Fatalf("second invalid feedback must retire => (%d, %s)", score, status)
	}
	// retired 不被有效反馈复活（阶段 1 不做复活，避免噪声回流）。
	retired := MemoryCase{QualityScore: -10, Status: MemoryCaseStatusRetired}
	if _, status := retired.ApplyFeedback(true); status != MemoryCaseStatusRetired {
		t.Fatalf("retired case must stay retired, got %s", status)
	}
}

func TestBuildAlertSignature(t *testing.T) {
	dims := map[string]string{"instance_id": "i-1", "zone": "cn-north-4a"}
	a := BuildAlertSignature("HighCPU", "cpu_util", dims)
	if a == "" || !strings.HasPrefix(a, "highcpu|cpu_util|") {
		t.Fatalf("unexpected signature: %q", a)
	}
	// 维度顺序不影响结果（按 key 排序后哈希）。
	b := BuildAlertSignature("  HighCPU ", "CPU_UTIL", map[string]string{"zone": "cn-north-4a", "instance_id": "i-1"})
	if a != b {
		t.Fatalf("signature must be stable: %q vs %q", a, b)
	}
	// 维度值不同则签名不同。
	c := BuildAlertSignature("HighCPU", "cpu_util", map[string]string{"instance_id": "i-2", "zone": "cn-north-4a"})
	if a == c {
		t.Fatal("different dimensions must produce different signatures")
	}
	if BuildAlertSignature("", "", nil) != "" {
		t.Fatal("empty input must produce empty signature")
	}
}

func TestBuildMemoryKeywords(t *testing.T) {
	got := BuildMemoryKeywords("payment-service CPU 持续升高，建议先查发布记录！发布", "", "payment-service")
	tokens := strings.Fields(got)
	if len(tokens) == 0 {
		t.Fatalf("expected keywords, got %q", got)
	}
	seen := map[string]struct{}{}
	for _, token := range tokens {
		if _, ok := seen[token]; ok {
			t.Fatalf("duplicate keyword %q in %q", token, got)
		}
		seen[token] = struct{}{}
		if strings.ContainsAny(token, "，。！") {
			t.Fatalf("punctuation must be stripped: %q", token)
		}
	}
	// 单字符词被过滤。
	for _, token := range tokens {
		if len([]rune(token)) < 2 {
			t.Fatalf("too-short keyword kept: %q", token)
		}
	}
	if len([]rune(BuildMemoryKeywords(strings.Repeat("告警", 400)))) > MemoryCaseKeywordsRunes {
		t.Fatal("keywords must be truncated")
	}
}

func TestMemoryCaseFilterMatchPriority(t *testing.T) {
	c := MemoryCase{AlertSignature: "sig-1", ApplicationID: "app-1", ResourceID: "i-1", Keywords: "cpu 告警"}
	cases := []struct {
		name   string
		filter MemoryCaseFilter
		want   int
	}{
		{"signature wins", MemoryCaseFilter{AlertSignature: "SIG-1", ApplicationID: "app-1"}, 3},
		{"application", MemoryCaseFilter{ApplicationID: "app-1"}, 2},
		{"resource", MemoryCaseFilter{ResourceID: "i-1"}, 2},
		{"keywords", MemoryCaseFilter{Keywords: "cpu"}, 1},
		{"no match", MemoryCaseFilter{ApplicationID: "other", Keywords: "memory"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.MatchPriority(c); got != tc.want {
				t.Fatalf("priority = %d, want %d", got, tc.want)
			}
		})
	}
}
