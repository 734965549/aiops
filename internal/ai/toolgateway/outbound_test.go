package toolgateway

import (
	"context"
	"testing"
)

func TestValidateOutboundURL_BlocksMetadataIP(t *testing.T) {
	err := ValidateOutboundURL(context.Background(), "http://169.254.169.254/latest/meta-data/", DefaultOutboundPolicy())
	if err == nil {
		t.Fatal("expected metadata IP to be blocked")
	}
}

func TestValidateOutboundURL_BlocksPrivateWithoutLoopback(t *testing.T) {
	err := ValidateOutboundURL(context.Background(), "http://10.0.0.1/", DefaultOutboundPolicy())
	if err == nil {
		t.Fatal("expected private IP to be blocked")
	}
}

func TestValidateOutboundURL_AllowsPublicHost(t *testing.T) {
	if err := ValidateOutboundURL(context.Background(), "http://example.com/", DefaultOutboundPolicy()); err != nil {
		t.Fatalf("expected public host allowed: %v", err)
	}
}

func TestValidateOutboundURL_Allowlist(t *testing.T) {
	policy := OutboundPolicy{AllowedHosts: []string{"example.com"}}
	if err := ValidateOutboundURL(context.Background(), "https://example.com/v1", policy); err != nil {
		t.Fatalf("expected allowlisted host: %v", err)
	}
	if err := ValidateOutboundURL(context.Background(), "https://evil.example.com/v1", policy); err == nil {
		t.Fatal("expected non-allowlisted host rejected")
	}
}

func TestValidateOutboundURL_RejectsRedirectScheme(t *testing.T) {
	if err := ValidateOutboundURL(context.Background(), "file:///etc/passwd", DefaultOutboundPolicy()); err == nil {
		t.Fatal("expected non-http scheme rejected")
	}
}
