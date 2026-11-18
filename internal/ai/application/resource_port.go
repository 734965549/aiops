package application

import "context"

// AssetContext 是 AI 分析所需的资产/云资源脱敏快照。
// Labels 只用于服务端推导 CES namespace 与主维度，不整体进入模型输入。
type AssetContext struct {
	ResourceID        string            `json:"resource_id"`
	Name              string            `json:"name,omitempty"`
	ApplicationID     string            `json:"application_id,omitempty"`
	ResourceType      string            `json:"resource_type,omitempty"`
	CloudResourceType string            `json:"cloud_resource_type,omitempty"`
	CloudResourceID   string            `json:"cloud_resource_id,omitempty"`
	Region            string            `json:"region,omitempty"`
	AccountID         string            `json:"account_id,omitempty"`
	Labels            map[string]string `json:"-"`
}

// CloudSelector 返回资源对应的 CES namespace 与主维度，用于指标查询与规则查询推导。
func (c *AssetContext) CloudSelector() (namespace, dimensionName, dimensionValue string) {
	selector := deriveResourceCloudSelector(c)
	return selector.Namespace, selector.DimensionName, selector.DimensionValue
}

// AssetReader 按资源标识读取分析所需的资源上下文。
// 实现方负责资源对象级数据范围校验，拒绝时返回 PERMISSION_DENIED。
type AssetReader interface {
	GetForAnalysis(ctx context.Context, userID, resourceID string) (*AssetContext, error)
}

// ResourceQuery 是一次云同步资源发现请求（provider 无关）。
type ResourceQuery struct {
	AccountID    string
	ResourceType string
	Region       string
	Keyword      string
	Limit        int
}

// ResourceSummary 是注入对话观测的云资源摘要。
// Labels 仅用于服务端推导 namespace/主维度，json:"-" 保证不进入模型输入。
type ResourceSummary struct {
	ResourceID        string            `json:"resource_id"`
	Name              string            `json:"name,omitempty"`
	CloudResourceType string            `json:"cloud_resource_type,omitempty"`
	CloudResourceID   string            `json:"cloud_resource_id,omitempty"`
	Region            string            `json:"region,omitempty"`
	AccountID         string            `json:"account_id,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	DimensionName     string            `json:"dimension_name,omitempty"`
	DimensionValue    string            `json:"dimension_value,omitempty"`
	Labels            map[string]string `json:"-"`
}

// ResourceListResult 是资源发现结果；status 非 ok 时 Resources 为空且 Reason 说明原因。
type ResourceListResult struct {
	Status    string            `json:"status"`
	Reason    string            `json:"reason,omitempty"`
	Resources []ResourceSummary `json:"resources,omitempty"`
}

// ResourceDiscoveryPort 按账号发现云同步资源，由 infrastructure/asset 适配。
// 实现方负责账号解析与数据范围校验：业务失败以 Result.Status 表达。
type ResourceDiscoveryPort interface {
	List(ctx context.Context, userID string, q ResourceQuery) (*ResourceListResult, error)
}

// SetAssets 注入资源上下文读取能力；nil 表示关闭 resource.analyze 工具。
func (s *ChatService) SetAssets(assets AssetReader) {
	if s == nil {
		return
	}
	s.assets = assets
}

// SetResources 注入资源发现能力；nil 表示关闭 resources.list 工具。
func (s *ChatService) SetResources(resources ResourceDiscoveryPort) {
	if s == nil {
		return
	}
	s.resources = resources
}
