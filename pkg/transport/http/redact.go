package http

import "strings"

// sensitivePathPrefixes 是 URL 路径中携带一次性凭证的前缀；访问/错误日志必须脱敏。
var sensitivePathPrefixes = []string{
	"/api/ai/chat/shares/",
}

// RedactSensitivePath 把路径中的一次性凭证替换为 <redacted>，避免 token 进入日志。
// 例如 /api/ai/chat/shares/abc -> /api/ai/chat/shares/<redacted>。
func RedactSensitivePath(path string) string {
	for _, prefix := range sensitivePathPrefixes {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		if len(path) <= len(prefix) {
			return path
		}
		return prefix + "<redacted>"
	}
	return path
}
