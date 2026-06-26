package huawei

import (
	"encoding/json"
	"strings"
)

// 同步模式，见 docs/huawei-ces-asset-sync-plan.md §4。
const (
	SyncModeCES    = "ces"    // 默认模式：仅依赖 CES 资源分组/资源列表发现资源
	SyncModeHybrid = "hybrid" // 增强模式：先 ces 全量发现并入库，再用原生 API 增强详情
	SyncModeNative = "native" // 兼容旧模式：沿用 ECS/CCE/RDS/ELB 原生 API 发现
)

const (
	defaultSyncMode            = SyncModeCES
	defaultResourceGroupName   = "全部资源"
	defaultMaxResources   int  = 20000
	defaultCESPageLimit   int32 = 100
)

// SyncModeConfig 解析自 integration_account.extra_config 的华为云同步配置，
// 见 docs/huawei-ces-asset-sync-plan.md §11。禁止存放密钥或凭据。
type SyncModeConfig struct {
	Mode                string `json:"sync_mode"`
	ResourceGroupName   string `json:"resource_group_name"`
	ResourceGroupID     string `json:"resource_group_id"`
	EnterpriseProjectID string `json:"enterprise_project_id"`
	MaxResources        int    `json:"max_resources"`
}

// ParseSyncModeConfig 从 extra_config 原始 JSON 解析华为云同步配置。
// 解析失败或字段缺失时回落到默认值：Mode=ces、ResourceGroupName=全部资源、MaxResources=20000。
// 未知 Mode 同样回落为 ces，避免误启用 native。
func ParseSyncModeConfig(raw []byte) SyncModeConfig {
	cfg := SyncModeConfig{
		Mode:              defaultSyncMode,
		ResourceGroupName: defaultResourceGroupName,
		MaxResources:      defaultMaxResources,
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return cfg
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return cfg
	}
	if v, ok := parsed["sync_mode"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case SyncModeHybrid:
			cfg.Mode = SyncModeHybrid
		case SyncModeNative:
			cfg.Mode = SyncModeNative
		default:
			cfg.Mode = SyncModeCES
		}
	}
	if v, ok := parsed["resource_group_name"].(string); ok {
		if name := strings.TrimSpace(v); name != "" {
			cfg.ResourceGroupName = name
		}
	}
	if v, ok := parsed["resource_group_id"].(string); ok {
		cfg.ResourceGroupID = strings.TrimSpace(v)
	}
	if v, ok := parsed["enterprise_project_id"].(string); ok {
		cfg.EnterpriseProjectID = strings.TrimSpace(v)
	}
	if v, ok := parsed["max_resources"].(float64); ok {
		if n := int(v); n > 0 {
			cfg.MaxResources = n
		}
	}
	return cfg
}
