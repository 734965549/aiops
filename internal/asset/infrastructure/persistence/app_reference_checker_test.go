package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// createTestAlert 在 alert_alert 表中插入一条测试告警，返回 alert_id 用于清理。
func createTestAlert(t *testing.T, db *gorm.DB, alertID, applicationID, status string) {
	t.Helper()
	if status == "" {
		status = "open"
	}
	now := time.Now().UTC()
	err := db.Exec(`
		INSERT INTO alert_alert
			(alert_id, source, dedup_key, name, severity, status,
			 application_id, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES (?, 'test-source', ?, 'test-alert', 'warning', ?,
		        ?, ?, ?, ?, ?)
	`, alertID, "dedup-"+alertID, status, applicationID, now, now, now, now).Error
	if err != nil {
		t.Fatalf("create test alert: %v", err)
	}
}

// deleteTestAlerts 清理测试告警。
func deleteTestAlerts(t *testing.T, db *gorm.DB, alertIDs ...string) {
	t.Helper()
	if len(alertIDs) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM alert_alert WHERE alert_id IN ?", alertIDs).Error; err != nil {
		t.Fatalf("cleanup alerts: %v", err)
	}
}

// createTestInspectionPolicy 在 inspection_policy 表中插入一条测试策略，返回 policy_id 用于清理。
// applicationIDs 序列化为 scope->'application_ids' JSONB 数组。
func createTestInspectionPolicy(t *testing.T, db *gorm.DB, policyID string, applicationIDs []string, deleted bool) {
	t.Helper()
	now := time.Now().UTC()
	// 构建 scope JSON: {"application_ids": ["id1", "id2"]}
	scope := `{"application_ids": [`
	for i, id := range applicationIDs {
		if i > 0 {
			scope += ", "
		}
		scope += fmt.Sprintf(`"%s"`, id)
	}
	scope += `]}`
	err := db.Exec(`
		INSERT INTO inspection_policy
			(policy_id, name, scope, deleted, created_at, updated_at)
		VALUES (?, ?, ?::jsonb, ?, ?, ?)
	`, policyID, "test-policy-"+policyID, scope, deleted, now, now).Error
	if err != nil {
		t.Fatalf("create test inspection policy: %v", err)
	}
}

// deleteTestInspectionPolicies 清理测试策略。
func deleteTestInspectionPolicies(t *testing.T, db *gorm.DB, policyIDs ...string) {
	t.Helper()
	if len(policyIDs) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM inspection_policy WHERE policy_id IN ?", policyIDs).Error; err != nil {
		t.Fatalf("cleanup inspection policies: %v", err)
	}
}

// ——— CountAlertsByApplicationID ———

func TestApplicationReferenceChecker_CountAlertsByApplicationID_Matches(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	alertID := "alert-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestAlerts(t, db, alertID) })

	createTestAlert(t, db, alertID, appID, "open")

	n, err := checker.CountAlertsByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountAlertsByApplicationID: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected count >= 1 for matching application_id, got %d", n)
	}
}

func TestApplicationReferenceChecker_CountAlertsByApplicationID_NoMatch(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	alertID := "alert-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestAlerts(t, db, alertID) })

	// 创建一条指向其他 application_id 的告警
	createTestAlert(t, db, alertID, "other-app-id", "open")

	n, err := checker.CountAlertsByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountAlertsByApplicationID: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected count 0 for non-matching application_id, got %d", n)
	}
}

func TestApplicationReferenceChecker_CountAlertsByApplicationID_ClosedExcluded(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	alertID := "alert-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestAlerts(t, db, alertID) })

	createTestAlert(t, db, alertID, appID, "closed")

	n, err := checker.CountAlertsByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountAlertsByApplicationID: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected count 0 for closed alert, got %d", n)
	}
}

func TestApplicationReferenceChecker_DetachClosedAlertReferences(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	alertID := "alert-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestAlerts(t, db, alertID) })

	createTestAlert(t, db, alertID, appID, "closed")

	if err := checker.DetachClosedAlertReferences(ctx, appID); err != nil {
		t.Fatalf("DetachClosedAlertReferences: %v", err)
	}

	var appRef string
	if err := db.Raw("SELECT application_id FROM alert_alert WHERE alert_id = ?", alertID).Scan(&appRef).Error; err != nil {
		t.Fatalf("load alert: %v", err)
	}
	if appRef != "" {
		t.Fatalf("expected empty application_id after detach, got %q", appRef)
	}
}

// ——— CountInspectionPoliciesByApplicationID ———

func TestApplicationReferenceChecker_CountInspectionPoliciesByApplicationID_Matches(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	policyID := "pol-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestInspectionPolicies(t, db, policyID) })

	createTestInspectionPolicy(t, db, policyID, []string{appID}, false)

	n, err := checker.CountInspectionPoliciesByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountInspectionPoliciesByApplicationID: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected count >= 1 for matching application_ids, got %d", n)
	}
}

func TestApplicationReferenceChecker_CountInspectionPoliciesByApplicationID_NoMatch(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	policyID := "pol-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestInspectionPolicies(t, db, policyID) })

	// 创建一条 scope 中包含其他 application_id 的策略
	createTestInspectionPolicy(t, db, policyID, []string{"other-app-id"}, false)

	n, err := checker.CountInspectionPoliciesByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountInspectionPoliciesByApplicationID: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected count 0 for non-matching application_ids, got %d", n)
	}
}

func TestApplicationReferenceChecker_CountInspectionPoliciesByApplicationID_DeletedExcluded(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	policyID := "pol-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestInspectionPolicies(t, db, policyID) })

	// 创建一条已删除的策略，其 scope 中仍包含 application_id
	createTestInspectionPolicy(t, db, policyID, []string{appID}, true)

	n, err := checker.CountInspectionPoliciesByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountInspectionPoliciesByApplicationID: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected count 0 when policy is deleted, got %d", n)
	}
}

func TestApplicationReferenceChecker_CountInspectionPoliciesByApplicationID_Multiple(t *testing.T) {
	db := openAssetTestPostgres(t)
	checker := NewApplicationReferenceChecker(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	policyID1 := "pol-" + uuid.NewString()[:8]
	policyID2 := "pol-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestInspectionPolicies(t, db, policyID1, policyID2) })

	// 创建两条策略，都引用同一个 application_id
	createTestInspectionPolicy(t, db, policyID1, []string{appID}, false)
	createTestInspectionPolicy(t, db, policyID2, []string{appID, "other-app-id"}, false)

	n, err := checker.CountInspectionPoliciesByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("CountInspectionPoliciesByApplicationID: %v", err)
	}
	if n < 2 {
		t.Fatalf("expected count >= 2 when multiple policies match, got %d", n)
	}
}

// ——— Nil / unconfigured checker ———

func TestApplicationReferenceChecker_NilDB(t *testing.T) {
	checker := &ApplicationReferenceChecker{db: nil}
	ctx := context.Background()

	_, err := checker.CountAlertsByApplicationID(ctx, "app-1")
	if err == nil {
		t.Fatal("expected error when db is nil")
	}

	_, err = checker.CountInspectionPoliciesByApplicationID(ctx, "app-1")
	if err == nil {
		t.Fatal("expected error when db is nil")
	}
}
