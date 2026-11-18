package persistence

import (
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
)

func TestMemoryCaseTableName(t *testing.T) {
	if got := (memoryCaseModel{}).TableName(); got != "ai_memory_case" {
		t.Fatalf("memory case table name = %q", got)
	}
}

func TestMemoryCaseRecommendationsRoundTrip(t *testing.T) {
	raw, err := marshalRecommendations([]string{"先确认发布记录", "扩容后观察 10 分钟"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := unmarshalRecommendations(raw)
	if len(got) != 2 || got[0] != "先确认发布记录" {
		t.Fatalf("round-trip = %+v", got)
	}
	if out := unmarshalRecommendations(""); len(out) != 0 {
		t.Fatalf("empty text must yield empty slice, got %+v", out)
	}
	if out := unmarshalRecommendations("not-json"); len(out) != 0 {
		t.Fatalf("invalid json must degrade to empty slice, got %+v", out)
	}
}

func TestToMemoryCaseDomainDefaults(t *testing.T) {
	row := &memoryCaseModel{
		CaseID: "case-1", UserID: "u-1",
		SourceType: "unknown", Status: "archived",
		Recommendations: "[]",
	}
	c, err := toMemoryCaseDomain(row)
	if err != nil {
		t.Fatalf("to domain: %v", err)
	}
	// 非法枚举回落到安全默认值，避免脏数据让检索报错。
	if c.SourceType != domain.MemoryCaseSourceChatMessage {
		t.Fatalf("source type = %q", c.SourceType)
	}
	if c.Status != domain.MemoryCaseStatusActive {
		t.Fatalf("status = %q", c.Status)
	}
	if c.Recommendations == nil {
		t.Fatal("recommendations must not be nil")
	}
}
