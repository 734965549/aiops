package application

import (
	"testing"
)

func TestAlertObjectContextOfMapsAlertFields(t *testing.T) {
	obj := AlertObjectContextOf(&AlertContext{
		OwnerUserID:  " user-9 ",
		Environment:  "prod",
		BusinessLine: "payments",
		Labels: map[string]string{
			"dept":      "platform",
			"team":      "sre",
			"region":    "cn-north-4",
			"tags":      "core, database",
			"alertname": "HighCPU",
		},
	})
	if obj.Owner != "user-9" || obj.Dept != "platform" || obj.Team != "sre" || obj.Region != "cn-north-4" {
		t.Fatalf("unexpected object context: %+v", obj)
	}
	want := []string{"core", "database", "prod", "payments"}
	if len(obj.Tags) != len(want) {
		t.Fatalf("tags = %+v, want %+v", obj.Tags, want)
	}
	for i, value := range want {
		if obj.Tags[i] != value {
			t.Fatalf("tags[%d] = %q, want %q", i, obj.Tags[i], value)
		}
	}
}

func TestAlertObjectContextOfEmptyAlert(t *testing.T) {
	obj := AlertObjectContextOf(nil)
	if obj.Owner != "" || obj.Dept != "" || obj.Team != "" || obj.Region != "" || len(obj.Tags) != 0 {
		t.Fatalf("unexpected object context: %+v", obj)
	}
}

func TestAlertObjectContextOfIgnoresUnrelatedLabels(t *testing.T) {
	// 任意标签不应把 tag 数据范围放大：alertname / instance 等不参与 tag 匹配。
	obj := AlertObjectContextOf(&AlertContext{
		Labels: map[string]string{
			"alertname": "HighCPU", "instance": "10.0.0.1", "severity": "p1",
		},
	})
	if len(obj.Tags) != 0 {
		t.Fatalf("unexpected tags: %+v", obj.Tags)
	}
}

func TestAlertObjectContextOfDedupesAndCapsTags(t *testing.T) {
	labels := map[string]string{"env": "prod", "environment": "PROD", "business_line": "payments"}
	raw := ""
	for i := 0; i < 40; i++ {
		raw += "t" + string(rune('a'+i%26)) + ","
	}
	labels["tags"] = raw
	obj := AlertObjectContextOf(&AlertContext{Labels: labels})
	if len(obj.Tags) > alertScopeMaxTags {
		t.Fatalf("tags must be capped at %d, got %d", alertScopeMaxTags, len(obj.Tags))
	}
	seen := map[string]struct{}{}
	for _, tag := range obj.Tags {
		if _, ok := seen[tag]; ok {
			t.Fatalf("duplicate tag %q", tag)
		}
		seen[tag] = struct{}{}
	}
	if obj.Tags[0] != "prod" || obj.Tags[1] != "payments" {
		t.Fatalf("unexpected tag order: %+v", obj.Tags)
	}
}
