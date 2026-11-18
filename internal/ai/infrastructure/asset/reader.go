// Package asset 将 Asset 读取/发现能力适配为 AI 资源上下文端口。
package asset

import (
	"context"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	assetapp "github.com/734965549/aiops/internal/asset/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// ResourceReader 是资源上下文读取依赖的最小能力（由 assetapp.AssetService 实现）。
type ResourceReader interface {
	GetResourceForAnalysis(ctx context.Context, userID, resourceID string) (*assetapp.ResourceAnalysisSnapshot, error)
}

// ReaderAdapter 实现 aiapp.AssetReader。
type ReaderAdapter struct {
	assets ResourceReader
}

func NewReaderAdapter(assets ResourceReader) *ReaderAdapter {
	return &ReaderAdapter{assets: assets}
}

var _ aiapp.AssetReader = (*ReaderAdapter)(nil)

// GetForAnalysis 读取资源快照并映射为 AI 侧脱敏上下文；数据范围由 Asset 应用层收口。
func (a *ReaderAdapter) GetForAnalysis(ctx context.Context, userID, resourceID string) (*aiapp.AssetContext, error) {
	if a == nil || a.assets == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai asset reader is not configured")
	}
	snapshot, err := a.assets.GetResourceForAnalysis(ctx, userID, resourceID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, apperr.New(apperr.CodeNotFound, "resource not found")
	}
	return &aiapp.AssetContext{
		ResourceID:        snapshot.ResourceID,
		Name:              snapshot.Name,
		ApplicationID:     snapshot.ApplicationID,
		ResourceType:      snapshot.ResourceType,
		CloudResourceType: snapshot.CloudResourceType,
		CloudResourceID:   snapshot.CloudResourceID,
		Region:            snapshot.Region,
		AccountID:         snapshot.IntegrationAccountID,
		Labels:            snapshot.Labels,
	}, nil
}
