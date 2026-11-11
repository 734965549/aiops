package domain

import (
	"errors"
	"strings"
	"time"
)

// AlertSourceDeliveryMode 区分接入源投递方式：平台主动拉取或外部 Webhook 推送。
// 存量 huawei_ces Webhook 源保持 webhook 语义，见 ops/huawei-ces-alert-sync-contract.md §4。
type AlertSourceDeliveryMode string

const (
	DeliveryModePull    AlertSourceDeliveryMode = "pull"
	DeliveryModeWebhook AlertSourceDeliveryMode = "webhook"
)

// ErrInvalidPullConfig 拉取配置非法；application 层映射为 INVALID_ARGUMENT。
var ErrInvalidPullConfig = errors.New("invalid alert source pull config")

const (
	// DefaultPullIntervalSeconds 拉取间隔默认值；配置为 0 或缺省时使用。
	DefaultPullIntervalSeconds = 300
	// MinPullIntervalSeconds 后台定时轮询下限；低于下限按 60 秒执行。
	MinPullIntervalSeconds = 60
)

// PullConfig 是拉取式接入源配置，落库为 alert_source.pull_config JSONB。
// 字段语义见 ops/huawei-ces-alert-sync-contract.md §4.2。
type PullConfig struct {
	DeliveryMode        AlertSourceDeliveryMode `json:"delivery_mode,omitempty"`
	PullIntervalSeconds int                     `json:"pull_interval_seconds,omitempty"`
	SeverityMap         map[string]string       `json:"severity_map,omitempty"`
	RegionFilter        []string                `json:"region_filter,omitempty"`
}

// DefaultSeverityMap 返回契约 §4.2 的 CES severity 默认映射。
func DefaultSeverityMap() map[string]string {
	return map[string]string{
		"critical": "p0",
		"major":    "p1",
		"minor":    "p2",
		"info":     "p3",
	}
}

// NormalizePullConfig 规范化拉取配置：去空白、补默认、去重 region_filter。
// 非法间隔按默认值处理（契约 §4.3）；severity_map 非法目标值返回 ErrInvalidPullConfig。
func NormalizePullConfig(in PullConfig) (PullConfig, error) {
	out := PullConfig{
		DeliveryMode:        in.DeliveryMode,
		PullIntervalSeconds: in.PullIntervalSeconds,
		SeverityMap:         map[string]string{},
		RegionFilter:        []string{},
	}
	if out.DeliveryMode == "" {
		out.DeliveryMode = DeliveryModePull
	}
	if out.DeliveryMode != DeliveryModePull && out.DeliveryMode != DeliveryModeWebhook {
		return PullConfig{}, ErrInvalidPullConfig
	}
	if out.PullIntervalSeconds <= 0 {
		out.PullIntervalSeconds = DefaultPullIntervalSeconds
	}
	base := DefaultSeverityMap()
	for k, v := range base {
		out.SeverityMap[k] = v
	}
	for k, v := range in.SeverityMap {
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.ToLower(strings.TrimSpace(v))
		if key == "" {
			continue
		}
		if !isValidSeverityName(val) {
			return PullConfig{}, ErrInvalidPullConfig
		}
		out.SeverityMap[key] = val
	}
	seen := map[string]struct{}{}
	for _, r := range in.RegionFilter {
		region := strings.TrimSpace(r)
		if region == "" {
			continue
		}
		key := strings.ToLower(region)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out.RegionFilter = append(out.RegionFilter, region)
	}
	return out, nil
}

// EffectivePullInterval 返回后台轮询实际间隔：低于下限按 60 秒执行。
func (c PullConfig) EffectivePullInterval() time.Duration {
	sec := c.PullIntervalSeconds
	if sec <= 0 {
		sec = DefaultPullIntervalSeconds
	}
	if sec < MinPullIntervalSeconds {
		sec = MinPullIntervalSeconds
	}
	return time.Duration(sec) * time.Second
}

// EffectiveRegionFilter 返回去空白后的 region 过滤集合。
func (c PullConfig) EffectiveRegionFilter() []string {
	out := make([]string, 0, len(c.RegionFilter))
	for _, r := range c.RegionFilter {
		if v := strings.TrimSpace(r); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// SeverityFor 按 CES 原始 severity 名称映射平台级别；未命中按 info 兜底。
func (c PullConfig) SeverityFor(cesSeverity string) (AlertSeverity, bool) {
	key := strings.ToLower(strings.TrimSpace(cesSeverity))
	if key == "" {
		return SeverityInfo, false
	}
	if v, ok := c.SeverityMap[key]; ok && isValidSeverityName(v) {
		return AlertSeverity(v), true
	}
	return SeverityInfo, false
}

func isValidSeverityName(v string) bool {
	switch AlertSeverity(v) {
	case SeverityP0, SeverityP1, SeverityP2, SeverityP3, SeverityInfo:
		return true
	default:
		return false
	}
}
