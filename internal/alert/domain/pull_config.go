package domain

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AlertSourceDeliveryMode 区分接入源投递方式：平台主动拉取或外部 Webhook 推送。
// 存量源保留 webhook；delivery_mode 创建后不可修改，见 CES 拉取契约 §4.3。
type AlertSourceDeliveryMode string

const (
	DeliveryModePull    AlertSourceDeliveryMode = "pull"
	DeliveryModeWebhook AlertSourceDeliveryMode = "webhook"
)

// ErrInvalidPullConfig 拉取配置非法；application 层映射为 INVALID_ARGUMENT。
var ErrInvalidPullConfig = errors.New("invalid alert source pull config")

const (
	// DefaultPullIntervalSeconds 拉取间隔默认值；0 表示仅手动。
	DefaultPullIntervalSeconds = 300
	// MinPullIntervalSeconds 后台定时轮询下限。
	MinPullIntervalSeconds = 60
	// MaxPullIntervalSeconds 后台定时轮询上限。
	MaxPullIntervalSeconds = 86400
)

// PullConfig 是拉取式接入源配置，落库为 alert_source.pull_config JSONB。
// 字段语义见 ops/huawei-ces-alert-sync-contract.md §4.2。
type PullConfig struct {
	PullIntervalSeconds int               `json:"pull_interval_seconds,omitempty"`
	SeverityMap         map[string]string `json:"severity_map,omitempty"`
	RegionFilter        []string          `json:"region_filter,omitempty"`
}

// DefaultSeverityMap 返回契约 §4.2 的 CES level -> 平台级别默认映射。
// 键为 CES 整数级别的字符串形式（"1".."4"）。
func DefaultSeverityMap() map[string]string {
	return map[string]string{
		"1": "p0",
		"2": "p1",
		"3": "p2",
		"4": "p3",
	}
}

// NormalizePullConfig 规范化拉取配置：
//   - 间隔缺省按 300；0 保持 0（仅手动）；1..59、负数、超上限返回 ErrInvalidPullConfig
//   - severity_map 只允许键 "1".."4"，值限 p0/p1/p2/p3/info；部分覆盖默认映射
//   - region_filter 去空白、转小写并去重
func NormalizePullConfig(in PullConfig) (PullConfig, error) {
	out := PullConfig{
		PullIntervalSeconds: in.PullIntervalSeconds,
		SeverityMap:         map[string]string{},
		RegionFilter:        []string{},
	}
	if out.PullIntervalSeconds == 0 {
		// 0 是合法的“仅手动”，缺省值由调用方在创建时写入 300。
	} else if out.PullIntervalSeconds < MinPullIntervalSeconds || out.PullIntervalSeconds > MaxPullIntervalSeconds {
		return PullConfig{}, ErrInvalidPullConfig
	}
	for k, v := range DefaultSeverityMap() {
		out.SeverityMap[k] = v
	}
	for k, v := range in.SeverityMap {
		key := strings.TrimSpace(k)
		val := strings.ToLower(strings.TrimSpace(v))
		if key == "" {
			return PullConfig{}, ErrInvalidPullConfig
		}
		level, err := strconv.Atoi(key)
		if err != nil || level < 1 || level > 4 {
			return PullConfig{}, ErrInvalidPullConfig
		}
		if !isValidSeverityName(val) {
			return PullConfig{}, ErrInvalidPullConfig
		}
		out.SeverityMap[strconv.Itoa(level)] = val
	}
	seen := map[string]struct{}{}
	for _, r := range in.RegionFilter {
		region := strings.ToLower(strings.TrimSpace(r))
		if region == "" {
			continue
		}
		if _, dup := seen[region]; dup {
			continue
		}
		seen[region] = struct{}{}
		out.RegionFilter = append(out.RegionFilter, region)
	}
	sort.Strings(out.RegionFilter)
	return out, nil
}

// WithDefaults 补齐 severity_map/region_filter 默认值；不改变 interval，
// 因为 0 是合法的“仅手动”，仅在请求缺省时由 HTTP 层写入 300。
func (c PullConfig) WithDefaults() PullConfig {
	out := c
	if len(out.SeverityMap) == 0 {
		out.SeverityMap = DefaultSeverityMap()
	}
	if out.RegionFilter == nil {
		out.RegionFilter = []string{}
	}
	return out
}

// EffectivePullInterval 返回后台轮询实际间隔；0 表示仅手动触发。
func (c PullConfig) EffectivePullInterval() time.Duration {
	if c.PullIntervalSeconds <= 0 {
		return 0
	}
	return time.Duration(c.PullIntervalSeconds) * time.Second
}

// EffectiveRegionFilter 返回去空白后的 region 过滤集合。
func (c PullConfig) EffectiveRegionFilter() []string {
	out := make([]string, 0, len(c.RegionFilter))
	for _, r := range c.RegionFilter {
		if v := strings.TrimSpace(r); v != "" {
			out = append(out, strings.ToLower(v))
		}
	}
	return out
}

// SeverityFor 按 CES 整数级别（字符串形式）映射平台级别；未命中按 info 兜底。
func (c PullConfig) SeverityFor(cesLevel string) (AlertSeverity, bool) {
	key := strings.TrimSpace(cesLevel)
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
