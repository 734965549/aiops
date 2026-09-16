// Package persistence 用 GORM 实现 Asset 仓储，映射 asset_* 表。
package persistence

import (
	"context"
	"errors"
	"time"

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

// CountAlertsByApplicationID 统计 alert_alert 中引用了指定 application_id 的未关闭告警数量。
// closed 为最终态，关闭后不再阻塞 DeleteApplication；删除前由 DetachClosedAlertReferences 解除关联。
func (c *ApplicationReferenceChecker) CountAlertsByApplicationID(ctx context.Context, applicationID string) (int64, error) {
	if c == nil || c.db == nil {
		return 0, errors.New("application reference checker is not configured")
	}
	var n int64
	err := c.db.WithContext(ctx).
		Table("alert_alert").
		Where("application_id = ? AND status <> ?", applicationID, "closed").
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CountInspectionPoliciesByApplicationID 统计 inspection_policy 中
// scope->'application_ids' 包含指定 application_id 的未删除策略数量。
// 使用 @> jsonb_build_array 替代 JSONB ? 操作符，避免与 GORM/pgx 参数占位符 ? 冲突。
func (c *ApplicationReferenceChecker) CountInspectionPoliciesByApplicationID(ctx context.Context, applicationID string) (int64, error) {
	if c == nil || c.db == nil {
		return 0, errors.New("application reference checker is not configured")
	}
	var n int64
	err := c.db.WithContext(ctx).
		Table("inspection_policy").
		Where("deleted = FALSE AND scope->'application_ids' @> jsonb_build_array(?::text)", applicationID).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// DetachClosedAlertReferences 删除应用前解除已关闭告警的应用关联，避免 v_asset_app_ref_integrity 出现孤儿行。
func (c *ApplicationReferenceChecker) DetachClosedAlertReferences(ctx context.Context, applicationID string) error {
	if c == nil || c.db == nil {
		return errors.New("application reference checker is not configured")
	}
	now := time.Now().UTC()
	// 原生 Exec：批量解除 closed 告警关联；须手动维护 updated_at（见 database 迁移规则）。
	return c.db.WithContext(ctx).Exec(`
		UPDATE alert_alert
		SET application_id = '', application_name = '', updated_at = ?
		WHERE application_id = ? AND status = ?
	`, now, applicationID, "closed").Error
}
