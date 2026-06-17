package toolgateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMaxOutboundBodyBytes = 1 << 20 // 1 MiB

// OutboundPolicy 控制 AI 工具出站 HTTP 的目标地址策略。
type OutboundPolicy struct {
	// AllowedHosts 非空时仅允许列表内主机（精确或后缀匹配，如 ".example.com"）。
	AllowedHosts []string
	// AllowLoopback 是否允许 loopback（仅建议 dev 开启）。
	AllowLoopback bool
}

// DefaultOutboundPolicy 返回默认策略：拦截私有/链路本地/metadata 地址。
func DefaultOutboundPolicy() OutboundPolicy {
	return OutboundPolicy{AllowLoopback: false}
}

// ValidateOutboundURL 校验出站 URL；在注册 provider 与发起请求前均应调用。
func ValidateOutboundURL(ctx context.Context, raw string, policy OutboundPolicy) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if isBlockedMetadataHost(host) {
		return fmt.Errorf("url host %q is not allowed", host)
	}
	if len(policy.AllowedHosts) > 0 && !hostMatchesAllowlist(host, policy.AllowedHosts) {
		return fmt.Errorf("url host %q is not in outbound allowlist", host)
	}
	return validateHostResolves(ctx, host, policy)
}

func validateHostResolves(ctx context.Context, host string, policy OutboundPolicy) error {
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip, policy) {
			return fmt.Errorf("url host resolves to blocked address")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve url host: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("url host has no addresses")
	}
	for _, addr := range ips {
		if isBlockedIP(addr.IP, policy) {
			return fmt.Errorf("url host resolves to blocked address")
		}
	}
	return nil
}

func hostMatchesAllowlist(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, item := range allowed {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, ".") {
			if host == strings.TrimPrefix(item, ".") || strings.HasSuffix(host, item) {
				return true
			}
			continue
		}
		if host == item {
			return true
		}
	}
	return false
}

func isBlockedMetadataHost(host string) bool {
	switch host {
	case "169.254.169.254", "metadata.google.internal", "metadata", "localhost.localdomain":
		return true
	default:
		return strings.HasSuffix(host, ".metadata.google.internal")
	}
}

func isBlockedIP(ip net.IP, policy OutboundPolicy) bool {
	if ip == nil {
		return true
	}
	if ip.IsMulticast() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip.IsLoopback() && !policy.AllowLoopback {
		return true
	}
	// AWS/GCP/Azure 等 metadata 链路本地
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	return false
}

func joinProviderURL(baseURL, requestPath string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return "", fmt.Errorf("base_url is required")
	}
	base = strings.TrimRight(base, "/")
	path := strings.TrimLeft(requestPath, "/")
	if path == "" {
		return base, nil
	}
	return base + "/" + path, nil
}

func newOutboundHTTPClient(policy OutboundPolicy, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("redirect not allowed")
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, fmt.Errorf("invalid dial address: %w", err)
				}
				if err := validateHostResolves(ctx, host, policy); err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, addr)
			},
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
	}
}
