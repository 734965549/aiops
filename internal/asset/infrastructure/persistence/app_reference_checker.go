// Package persistence 用 GORM 实现 Asset 仓储，映射 asset_* 表。
package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ApplicationReferenceChecker 跨上下文应用引用检查器（GORM 实现）。
// 查询 alert_alert 和 inspection_policy 中是否引用了指定 application_id，
// 防止 DeleteApplication 产生孤儿引用（release-checklist §2.6）。
type ApplicationReferenceChecker struct {
	db *gorm.DB
}

// NewApplicationReferenceChecker 创建跨上下文引用检查器。
func NewApplicationReferenceChecker(db *gorm.DB) *ApplicationReferenceChecker {
	return &ApplicationReferenceChecker{db: db}
}

// CountAlertsByApplicationID 统计 alert_alert 中引用了指定 application_id 的告警数量。
// 包含所有状态的告警（closed 告警中的引用同样是孤儿引用），
// 与 v_asset_app_ref_integrity 视图检查范围一致。
func (c *ApplicationReferenceChecker) CountAlertsByApplicationID(ctx context.Context, applicationID string) (int64, error) {
	if c == nil || c.db == nil {
		return 0, errors.New("application reference checker is not configured")
	}
	var n int64
	err := c.db.WithContext(ctx).
		Table("alert_alert").
		Where("application_id = ?", applicationID).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CountInspectionPoliciesByApplicationID 统计 inspection_policy 中
// scope->'application_ids' 包含指定 application_id 的未删除策略数量。
// 使用 PostgreSQL JSONB ? 操作符检测数组元素是否存在。
func (c *ApplicationReferenceChecker) CountInspectionPoliciesByApplicationID(ctx context.Context, applicationID string) (int64, error) {
	if c == nil || c.db == nil {
		return 0, errors.New("application reference checker is not configured")
	}
	var n int64
	err := c.db.WithContext(ctx).
		Table("inspection_policy").
		Where("deleted = FALSE AND scope->'application_ids' ? ?", applicationID).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}
