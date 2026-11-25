package toolgateway

import "strings"

// BuiltinToolSpec 描述一个由 AI application 层执行的内置只读工具。
// 这些工具不经 provider 转发，但仍需在统一清单中登记，供 Agent Loop 白名单与提示词复用。
type BuiltinToolSpec struct {
	Code string
	Name string
	Mode string
}

var builtinChatToolSpecs = []BuiltinToolSpec{
	{Code: "alarm.analyze", Name: "读取告警快照", Mode: "read_only"},
	{Code: "metrics.query", Name: "查询指标证据", Mode: "read_only"},
	{Code: "postgres.activity.query", Name: "查询 PostgreSQL 历史会话", Mode: "read_only"},
	{Code: "alert.rules.list", Name: "查询告警规则", Mode: "read_only"},
	{Code: "alert.rules.health", Name: "批量规则体检", Mode: "read_only"},
	{Code: "rds.backup.query", Name: "查询 RDS 备份策略与记录", Mode: "read_only"},
	{Code: "accounts.list", Name: "列出可用集成账号", Mode: "read_only"},
	{Code: "resource.analyze", Name: "读取资源上下文", Mode: "read_only"},
	{Code: "resources.list", Name: "列出云同步资源", Mode: "read_only"},
}

// BuiltinChatToolSpecs 返回内置对话工具清单副本。
func BuiltinChatToolSpecs() []BuiltinToolSpec {
	out := make([]BuiltinToolSpec, len(builtinChatToolSpecs))
	copy(out, builtinChatToolSpecs)
	return out
}

// BuiltinChatToolCodes 返回内置对话工具编码（保持稳定顺序）。
func BuiltinChatToolCodes() []string {
	out := make([]string, 0, len(builtinChatToolSpecs))
	for _, spec := range builtinChatToolSpecs {
		out = append(out, spec.Code)
	}
	return out
}

// BuiltinChatToolSet 返回白名单集合，供 Agent Loop 判断可申请工具。
func BuiltinChatToolSet() map[string]struct{} {
	out := make(map[string]struct{}, len(builtinChatToolSpecs))
	for _, spec := range builtinChatToolSpecs {
		out[spec.Code] = struct{}{}
	}
	return out
}

// LookupBuiltinChatTool 按工具编码查找内置工具定义。
func LookupBuiltinChatTool(code string) (BuiltinToolSpec, bool) {
	code = strings.TrimSpace(code)
	for _, spec := range builtinChatToolSpecs {
		if spec.Code == code {
			return spec, true
		}
	}
	return BuiltinToolSpec{}, false
}
