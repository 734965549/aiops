package application

import (
	"encoding/json"
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
)

// 同步模式，见 ops/huawei-ces-sync-contract.md §4。
const (
	SyncModeCES    = "ces"    // 默认模式：仅依赖 CES 资源分组/资源列表发现资源
	SyncModeHybrid = "hybrid" // 增强模式：先 ces 全量发现并入库，再用原生 API 增强详情
	SyncModeNative = "native" // 兼容旧模式：沿用 ECS/CCE/RDS/ELB 原生 API 发现
	// SyncModeFake 标识 auth_type=none 的 fake 路径：scope 非权威，不填充 SuccessfulTypes，
	// 不参与权威反向 stale，见 ops/huawei-ces-sync-contract.md §13.1。
	// SyncModeFake 仅测试用途，不作为业务配置模式；业务决策不得依赖 fake。
	SyncModeFake = "fake"
)

const (
	defaultSyncMode = SyncModeCES
	// DefaultRawRowBudget 单区域单次同步的原始行预算，按 region 独立计额，按原始行计数，见 ops/huawei-ces-sync-contract.md §11。
	DefaultRawRowBudget int = 20000
	// DefaultMaxResources 保持向后兼容，等价于 DefaultRawRowBudget。
	DefaultMaxResources = DefaultRawRowBudget
	// MaxConfiguredResources 允许配置的最大资源数上限，防止用户误配过大值。
	MaxConfiguredResources int = 20000
	// DefaultCESPageLimit CES SDK 默认分页大小。
	DefaultCESPageLimit int32 = 100
)

// SyncModeConfig 解析自 integration_account.extra_config 的华为云同步配置，
// 见 ops/huawei-ces-sync-contract.md §11。禁止存放密钥或凭据。
type SyncModeConfig struct {
	Mode                string `json:"sync_mode"`
	ResourceGroupName   string `json:"resource_group_name"`
	ResourceGroupID     string `json:"resource_group_id"`
	EnterpriseProjectID string `json:"enterprise_project_id"`
	// RawFetchedCountBudget 表示华为云 CES/资源组接口允许拉取的原始行预算；
	// 它按“原始行数”计额，不是 unique 资源数，重复/无效行同样消耗预算。
	RawFetchedCountBudget int `json:"max_resources"`
	// MaxResources 保持旧测试与调用兼容，语义上等同 RawFetchedCountBudget，
	// 仅作为历史别名存在，后续代码应优先使用 RawFetchedCountBudget。
	MaxResources   int             `json:"-"`
	RegionProjects []RegionProject `json:"region_projects"`
}

type SyncModeConfigDiagnostics struct {
	RawMode                 string
	EffectiveMode           string
	FallbackToCES           bool
	InvalidSyncMode         string
	ResourceGroupNameSource string
	Warnings                []string
}

// RegionProject 表达 region -> project_id 映射，见 ops/huawei-ces-sync-contract.md §5.3。
// 华为云 project_id 与 region 相关；多区域账号需为每个 region 指定对应 project_id，
// 否则不同区域的 CES 资源可能查不到。
// ResourceGroupID / ResourceGroupName 可选：资源组是 project 作用域的，多区域通常需要
// 不同的资源组 ID/名称；未填则按 region 解析时回落到全局 ResourceGroupID / ResourceGroupName。
type RegionProject struct {
	Region            string `json:"region"`
	ProjectID         string `json:"project_id"`
	ResourceGroupID   string `json:"resource_group_id"`
	ResourceGroupName string `json:"resource_group_name"`
}

// ResolveProjectID 按 region 选择对应 project_id；未命中时回落 fallback（通常是 Account.ProjectID），
// 保证旧的单 project_id 账号零行为变化。region 大小写不敏感。
func (cfg SyncModeConfig) ResolveProjectID(region, fallback string) string {
	projectID, _ := cfg.ResolveProjectIDWithFallback(region, fallback)
	return projectID
}

// ResolveProjectIDWithFallback 返回 project_id 及是否命中 fallback。
func (cfg SyncModeConfig) ResolveProjectIDWithFallback(region, fallback string) (string, bool) {
	region = strings.TrimSpace(region)
	if region == "" {
		return fallback, true
	}
	for _, rp := range cfg.RegionProjects {
		if strings.EqualFold(strings.TrimSpace(rp.Region), region) {
			if pid := strings.TrimSpace(rp.ProjectID); pid != "" {
				return pid, false
			}
		}
	}
	return fallback, true
}

// ResolveResourceGroupID 按 region 选择对应 resource_group_id；未命中或对应项为空时回落
// 全局 ResourceGroupID，保证旧的单资源组账号零行为变化。region 大小写不敏感。
// 资源组是 project 作用域的，多区域账号需为每个 region 指定对应 resource_group_id。
func (cfg SyncModeConfig) ResolveResourceGroupID(region string) string {
	value, _ := cfg.ResolveResourceGroupIDWithFallback(region)
	return value
}

// ResolveResourceGroupIDWithFallback 返回 resource_group_id 及是否命中 fallback。
func (cfg SyncModeConfig) ResolveResourceGroupIDWithFallback(region string) (string, bool) {
	region = strings.TrimSpace(region)
	if region == "" {
		return cfg.ResourceGroupID, true
	}
	for _, rp := range cfg.RegionProjects {
		if strings.EqualFold(strings.TrimSpace(rp.Region), region) {
			if id := strings.TrimSpace(rp.ResourceGroupID); id != "" {
				return id, false
			}
		}
	}
	return cfg.ResourceGroupID, true
}

// ResolveResourceGroupName 按 region 选择对应 resource_group_name；未命中或对应项为空时
// 回落全局 ResourceGroupName。region 大小写不敏感。
func (cfg SyncModeConfig) ResolveResourceGroupName(region string) string {
	value, _ := cfg.ResolveResourceGroupNameWithFallback(region)
	return value
}

// ResolveResourceGroupNameWithFallback 返回 resource_group_name 及是否命中 fallback。
func (cfg SyncModeConfig) ResolveResourceGroupNameWithFallback(region string) (string, bool) {
	region = strings.TrimSpace(region)
	if region == "" {
		return cfg.ResourceGroupName, true
	}
	for _, rp := range cfg.RegionProjects {
		if strings.EqualFold(strings.TrimSpace(rp.Region), region) {
			if name := strings.TrimSpace(rp.ResourceGroupName); name != "" {
				return name, false
			}
		}
	}
	return cfg.ResourceGroupName, true
}

// ParseSyncModeConfig 从 extra_config 原始 JSON 解析华为云同步配置。
// 解析失败或字段缺失时回落到默认值：Mode=ces、RawRowBudget=20000；resource_group_name 只有在
// 显式写入非占位值时才进入配置，避免把“未指定”或前端占位提示误当成真实值，从而短路后端
// 默认候选回退。未知 Mode 同样回落为 ces，避免误启用 native。
func ParseSyncModeConfig(raw []byte) SyncModeConfig {
	cfg, _ := ParseSyncModeConfigWithDiagnostics(raw)
	return cfg
}

func ParseSyncModeConfigWithDiagnostics(raw []byte) (SyncModeConfig, SyncModeConfigDiagnostics) {
	cfg := SyncModeConfig{
		Mode:                  defaultSyncMode,
		RawFetchedCountBudget: DefaultRawRowBudget,
		MaxResources:          DefaultRawRowBudget,
	}
	diag := SyncModeConfigDiagnostics{}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "{}" {
		return cfg, diag
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		diag.Warnings = append(diag.Warnings, "sync_mode_config: invalid json, using defaults")
		return cfg, diag
	}
	if _, ok := parsed["sync_mode"]; !ok {
		diag.Warnings = append(diag.Warnings, "sync_mode_config: sync_mode missing, using default ces")
	}
	if v, ok := parsed["sync_mode"].(string); ok {
		diag.RawMode = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(v)) {
		case SyncModeHybrid:
			cfg.Mode = SyncModeHybrid
		case SyncModeNative:
			cfg.Mode = SyncModeNative
		case "", SyncModeCES:
			cfg.Mode = SyncModeCES
		default:
			cfg.Mode = SyncModeCES
			diag.FallbackToCES = true
			diag.InvalidSyncMode = diag.RawMode
			diag.Warnings = append(diag.Warnings, "sync_mode_config: invalid sync_mode, using ces fallback")
		}
		diag.EffectiveMode = cfg.Mode
	}
	if v, ok := parsed["resource_group_name"].(string); ok {
		trimmedName := strings.TrimSpace(v)
		switch trimmedName {
		case "":
			diag.ResourceGroupNameSource = "unspecified"
		case "全部资源", "All resources", "All Resources":
			diag.ResourceGroupNameSource = "placeholder"
		default:
			cfg.ResourceGroupName = trimmedName
			diag.ResourceGroupNameSource = "explicit"
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
			cfg.RawFetchedCountBudget = n
		}
	}
	cfg.MaxResources = cfg.RawFetchedCountBudget
	if arr, ok := parsed["region_projects"].([]any); ok {
		seen := map[string]struct{}{}
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			region, _ := m["region"].(string)
			pid, _ := m["project_id"].(string)
			region = strings.TrimSpace(region)
			pid = strings.TrimSpace(pid)
			if region == "" || pid == "" {
				continue
			}
			key := strings.ToLower(region)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			rgID, _ := m["resource_group_id"].(string)
			rgName, _ := m["resource_group_name"].(string)
			cfg.RegionProjects = append(cfg.RegionProjects, RegionProject{
				Region:            region,
				ProjectID:         pid,
				ResourceGroupID:   strings.TrimSpace(rgID),
				ResourceGroupName: strings.TrimSpace(rgName),
			})
		}
	}
	return cfg, diag
}

func ValidateSyncModeConfig(raw []byte) []string {
	_, diag := ParseSyncModeConfigWithDiagnostics(raw)
	return append([]string(nil), diag.Warnings...)
}

// ResolveSyncScopes 根据账号快照解析计划同步的 scope 列表，包含每个 region 实际使用的
// project_id、sync_mode、resource_group_id/name。不触发云 API，供 sync_started 审计和
// 失败 scope 补齐使用。见 ops/huawei-ces-sync-contract.md §13。
func ResolveSyncScopes(account domain.AccountSnapshot, regions []string) ([]CloudSyncSummary, []string) {
	cfg, diag := ParseSyncModeConfigWithDiagnostics(account.ExtraConfig)
	fallbackProjectID := strings.TrimSpace(account.ProjectID)
	scopes := make([]CloudSyncSummary, 0, len(regions))
	fallbackRegions := make([]string, 0)
	seen := map[string]struct{}{}
	for _, r := range regions {
		region := strings.TrimSpace(r)
		if region == "" {
			continue
		}
		key := strings.ToLower(region)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		projectID, projectFallback := cfg.ResolveProjectIDWithFallback(region, fallbackProjectID)
		resourceGroupID, rgFallback := cfg.ResolveResourceGroupIDWithFallback(region)
		resourceGroupName, rgnFallback := cfg.ResolveResourceGroupNameWithFallback(region)
		if projectFallback || rgFallback || rgnFallback {
			fallbackRegions = append(fallbackRegions, region)
		}
		scopes = append(scopes, CloudSyncSummary{
			Region:            region,
			ProjectID:         projectID,
			SyncMode:          cfg.Mode,
			ResourceGroupID:   resourceGroupID,
			ResourceGroupName: resourceGroupName,
			ResourceGroupSelection: func() string {
				if projectFallback || rgFallback || rgnFallback {
					return "fallback"
				}
				return "explicit"
			}(),
			ConfigModeFallback: diag.FallbackToCES,
		})
	}
	if len(scopes) == 0 {
		return nil, nil
	}
	return scopes, fallbackRegions
}
