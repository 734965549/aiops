// Package asset 适配 Asset 应用注册表，供 Inspection 校验 scope.application_ids。
package asset

import (
	"context"

	assetdomain "github.com/734965549/aiops/internal/asset/domain"
)

// ApplicationCatalogAdapter 将 Asset ApplicationRepository 适配为 ApplicationCatalogPort。
type ApplicationCatalogAdapter struct {
	apps assetdomain.ApplicationRepository
}

func NewApplicationCatalogAdapter(apps assetdomain.ApplicationRepository) *ApplicationCatalogAdapter {
	return &ApplicationCatalogAdapter{apps: apps}
}

func (a *ApplicationCatalogAdapter) ExistsByID(ctx context.Context, applicationID string) (bool, error) {
	if a == nil || a.apps == nil {
		return false, nil
	}
	return a.apps.ExistsByID(ctx, applicationID)
}
