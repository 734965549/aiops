package http

import "testing"

func TestRedactSensitivePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"share token", "/api/ai/chat/shares/abc123", "/api/ai/chat/shares/<redacted>"},
		{"share token with suffix", "/api/ai/chat/shares/abc123/extra", "/api/ai/chat/shares/<redacted>"},
		{"share prefix without token", "/api/ai/chat/shares/", "/api/ai/chat/shares/"},
		{"session path untouched", "/api/ai/chat/sessions/s-1", "/api/ai/chat/sessions/s-1"},
		{"providers path untouched", "/api/ai/providers", "/api/ai/providers"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactSensitivePath(tc.path); got != tc.want {
				t.Fatalf("RedactSensitivePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
