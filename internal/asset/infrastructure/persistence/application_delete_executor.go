package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ApplicationDeleteExecutor 在单事务内完成应用删除的前置校验与写入。
type ApplicationDeleteExecutor struct {
	db *gorm.DB
}

// NewApplicationDeleteExecutor 创建应用删除执行器。
func NewApplicationDeleteExecutor(db *gorm.DB) *ApplicationDeleteExecutor {
	return &ApplicationDeleteExecutor{db: db}
}

// DeleteApplicationAtomic 锁定应用行后重新计数引用，解除 closed 告警关联并删除应用。
func (e *ApplicationDeleteExecutor) DeleteApplicationAtomic(ctx context.Context, applicationID string) error {
	if e == nil || e.db == nil {
		return errors.New("application delete executor is not configured")
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return domain.ErrNotFound
	}
	return e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockApplicationForDelete(tx, applicationID); err != nil {
			return err
		}
		resourceCount, err := countResourcesByApplicationID(tx, applicationID)
		if err != nil {
			return err
		}
		if resourceCount > 0 {
			return domain.ErrHasResources
		}
		ruleCount, err := countMatchRulesByApplicationID(tx, applicationID)
		if err != nil {
			return err
		}
		if ruleCount > 0 {
			return domain.ErrHasMatchRules
		}
		alertCount, err := countOpenAlertsByApplicationID(tx, applicationID)
		if err != nil {
			return err
		}
		if alertCount > 0 {
			return domain.ErrHasAlertReferences
		}
		policyCount, err := countActivePoliciesByApplicationID(tx, applicationID)
		if err != nil {
			return err
		}
		if policyCount > 0 {
			return domain.ErrHasInspectionPolicyReferences
		}
		if err := detachClosedAlertReferencesTx(tx, applicationID); err != nil {
			return err
		}
		res := tx.Where("application_id = ?", applicationID).Delete(&applicationModel{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

func lockApplicationForDelete(tx *gorm.DB, applicationID string) error {
	var row applicationModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("application_id = ?", applicationID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}

func countResourcesByApplicationID(tx *gorm.DB, applicationID string) (int64, error) {
	var n int64
	err := tx.Model(&resourceModel{}).Where("application_id = ?", applicationID).Count(&n).Error
	return n, err
}

func countMatchRulesByApplicationID(tx *gorm.DB, applicationID string) (int64, error) {
	var n int64
	err := tx.Model(&matchRuleModel{}).Where("application_id = ?", applicationID).Count(&n).Error
	return n, err
}

func countOpenAlertsByApplicationID(tx *gorm.DB, applicationID string) (int64, error) {
	var n int64
	err := tx.Table("alert_alert").
		Where("application_id = ? AND status <> ?", applicationID, "closed").
		Count(&n).Error
	return n, err
}

func countActivePoliciesByApplicationID(tx *gorm.DB, applicationID string) (int64, error) {
	var n int64
	err := tx.Table("inspection_policy").
		Where("deleted = FALSE AND scope->'application_ids' @> jsonb_build_array(?::text)", applicationID).
		Count(&n).Error
	return n, err
}

func detachClosedAlertReferencesTx(tx *gorm.DB, applicationID string) error {
	now := time.Now().UTC()
	return tx.Exec(`
		UPDATE alert_alert
		SET application_id = '', application_name = '', updated_at = ?
		WHERE application_id = ? AND status = ?
	`, now, applicationID, "closed").Error
}
