package huawei

import (
	"strings"
	"testing"

	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestValidateRegion(t *testing.T) {
	cases := []struct {
		name    string
		region  string
		wantErr bool
	}{
		{"valid cn-north-4", "cn-north-4", false},
		{"valid ap-southeast-1", "ap-southeast-1", false},
		{"valid single segment", "cn", false},
		{"empty", "", true},
		{"only whitespace", "   ", true},
		{"uppercase", "CN-North-4", true},
		{"slash payload", "evil.com/", true},
		{"question payload", "evil.com?", true},
		{"hash payload", "evil.com#", true},
		{"at payload", "evil.com@", true},
		{"colon payload", "cn:north", true},
		{"space inside", "cn north", true},
		{"underscore", "cn_north", true},
		{"leading hyphen", "-cn-north-4", true},
		{"trailing hyphen", "cn-north-4-", true},
		{"double hyphen", "cn--north", true},
		{"too long", strings.Repeat("a", huaweiRegionMaxLen+1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRegion(tc.region)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for region %q, got nil", tc.region)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for region %q: %v", tc.region, err)
			}
			if tc.wantErr && err != nil && apperr.CodeOf(err) != apperr.CodeInvalidArgument {
				t.Fatalf("expected CodeInvalidArgument, got %s", apperr.CodeOf(err))
			}
		})
	}
}

func TestBuildEndpoint(t *testing.T) {
	t.Run("valid ces endpoint", func(t *testing.T) {
		got, err := buildEndpoint("ces", "cn-north-4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "https://ces.cn-north-4.myhuaweicloud.com"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("valid ecs endpoint", func(t *testing.T) {
		got, err := buildEndpoint("ecs", "ap-southeast-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://ecs.ap-southeast-1.myhuaweicloud.com" {
			t.Fatalf("got %q", got)
		}
	})

	rrPayloads := []string{
		"evil.com/",
		"evil.com?",
		"evil.com#",
		"evil.com@x",
		"cn:north",
		"cn north",
		"CN-North",
	}
	for _, region := range rrPayloads {
		t.Run("reject region "+region, func(t *testing.T) {
			if _, err := buildEndpoint("ces", region); err == nil {
				t.Fatalf("expected error for region %q, got nil", region)
			}
		})
	}

	t.Run("reject empty service", func(t *testing.T) {
		if _, err := buildEndpoint("", "cn-north-4"); err == nil {
			t.Fatalf("expected error for empty service")
		}
	})

	t.Run("reject invalid service", func(t *testing.T) {
		if _, err := buildEndpoint("ces2", "cn-north-4"); err == nil {
			t.Fatalf("expected error for service ces2")
		}
		if _, err := buildEndpoint("c/s", "cn-north-4"); err == nil {
			t.Fatalf("expected error for service c/s")
		}
	})
}
