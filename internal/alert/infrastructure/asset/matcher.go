// Package asset 将 Alert application.AssetMatcher 适配到 Asset MatcherService（§9.1）。
package asset

import (
	"context"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	assetapp "github.com/734965549/aiops/internal/asset/application"
)

// MatcherAdapter 实现 Alert AssetMatcher port。
type MatcherAdapter struct {
	matcher *assetapp.MatcherService
}

// NewMatcherAdapter 构造适配器。
func NewMatcherAdapter(matcher *assetapp.MatcherService) *MatcherAdapter {
	return &MatcherAdapter{matcher: matcher}
}

func (a *MatcherAdapter) Match(ctx context.Context, in alertapp.AssetMatchInput) (alertapp.AssetMatchResult, error) {
	if a == nil || a.matcher == nil {
		return alertapp.AssetMatchResult{}, nil
	}
	r, err := a.matcher.Match(ctx, assetapp.MatchInput{
		SourceType:      in.SourceType,
		ApplicationName: in.ApplicationName,
		ResourceName:    in.ResourceName,
		ResourceType:    in.ResourceType,
		Environment:     in.Environment,
		Labels:          in.Labels,
	})
	if err != nil {
		return alertapp.AssetMatchResult{}, err
	}
	return alertapp.AssetMatchResult{ApplicationID: r.ApplicationID, ResourceID: r.ResourceID}, nil
}
