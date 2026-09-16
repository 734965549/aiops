package auth

import (
	"fmt"
	"net"
	"strings"
)

// IPAllowlist 保存登录入口的 IP 白名单，支持单个 IP 和 CIDR 网段。
type IPAllowlist struct {
	ips  map[string]struct{}
	nets []*net.IPNet
}

// NewIPAllowlist 解析配置中的白名单；同一个配置项可以用逗号分隔多个地址。
func NewIPAllowlist(entries []string) (*IPAllowlist, error) {
	list := &IPAllowlist{ips: make(map[string]struct{})}
	for _, raw := range entries {
		for _, part := range strings.Split(raw, ",") {
			entry := strings.TrimSpace(part)
			if entry == "" {
				continue
			}
			if strings.Contains(entry, "/") {
				_, network, err := net.ParseCIDR(entry)
				if err != nil {
					return nil, fmt.Errorf("invalid ip allowlist cidr %q: %w", entry, err)
				}
				list.nets = append(list.nets, network)
				continue
			}
			ip := net.ParseIP(entry)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip allowlist entry %q", entry)
			}
			list.ips[ip.String()] = struct{}{}
		}
	}
	return list, nil
}

// Enabled 表示白名单是否真正启用；空配置会视为未限制。
func (l *IPAllowlist) Enabled() bool {
	return l != nil && (len(l.ips) > 0 || len(l.nets) > 0)
}

// Allows 判断客户端 IP 是否命中白名单；未启用时默认放行。
func (l *IPAllowlist) Allows(rawIP string) bool {
	if l == nil || !l.Enabled() {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(rawIP))
	if ip == nil {
		return false
	}
	if _, ok := l.ips[ip.String()]; ok {
		return true
	}
	for _, network := range l.nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
