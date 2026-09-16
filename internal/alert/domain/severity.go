package domain

import "strings"

// AlertSeverity 告警级别；API 与持久化使用小写枚举（ops/alert-contract.md §4.1）。
type AlertSeverity string

const (
	// SeverityP0 严重故障，核心业务不可用或大范围影响。
	SeverityP0 AlertSeverity = "p0"
	// SeverityP1 高危问题，关键服务明显异常。
	SeverityP1 AlertSeverity = "p1"
	// SeverityP2 中等问题，局部影响或有劣化趋势。
	SeverityP2 AlertSeverity = "p2"
	// SeverityP3 低优先级问题，需要跟进但不紧急。
	SeverityP3 AlertSeverity = "p3"
	// SeverityInfo 信息类提醒。
	SeverityInfo AlertSeverity = "info"
)

// IsValid 判断是否为平台定义的告警级别。
func (s AlertSeverity) IsValid() bool {
	switch s {
	case SeverityP0, SeverityP1, SeverityP2, SeverityP3, SeverityInfo:
		return true
	default:
		return false
	}
}

// DisplayLabel 返回前端展示文案（P0、P1、Info 等）；未知值原样返回。
func (s AlertSeverity) DisplayLabel() string {
	switch s {
	case SeverityP0:
		return "P0"
	case SeverityP1:
		return "P1"
	case SeverityP2:
		return "P2"
	case SeverityP3:
		return "P3"
	case SeverityInfo:
		return "Info"
	default:
		return string(s)
	}
}

// NormalizeSeverity 将 critical/warning 等外部级别归一化为平台 p0-p3/info（§4.1 级别归一化表）。
//
// 未知或空值默认回落为 info，避免接入源差异导致写入失败。
func NormalizeSeverity(raw string) AlertSeverity {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "critical", "fatal", "emergency", "p0":
		return SeverityP0
	case "high", "major", "warning", "p1":
		return SeverityP1
	case "medium", "minor", "p2":
		return SeverityP2
	case "low", "notice", "p3":
		return SeverityP3
	case "info", "none", "":
		return SeverityInfo
	default:
		if s := AlertSeverity(v); s.IsValid() {
			return s
		}
		return SeverityInfo
	}
}
