package application

import "context"

// AssetMatchInput 是 Alert 接入时传给 Asset 模块的标签快照（ops/alert-contract.md §9.1）。
type AssetMatchInput struct {
	SourceType      string
	ApplicationName string
	ResourceName    string
	ResourceType    string
	Environment     string
	Labels          map[string]string
}

// AssetMatchResult 匹配结果；未命中时 ID 为空。
type AssetMatchResult struct {
	ApplicationID string
	ResourceID    string
}

// AssetMatcher 由 Asset 模块实现，Alert 接入时自动写入 application_id / resource_id。
type AssetMatcher interface {
	Match(ctx context.Context, in AssetMatchInput) (AssetMatchResult, error)
}

// NoopAssetMatcher 未配置 Asset 时不做匹配。
type NoopAssetMatcher struct{}

func (NoopAssetMatcher) Match(context.Context, AssetMatchInput) (AssetMatchResult, error) {
	return AssetMatchResult{}, nil
}
