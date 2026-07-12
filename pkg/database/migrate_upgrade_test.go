package database

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/734965549/aiops/pkg/config"
	"gorm.io/gorm"
)

// TestMigrateUpgrade_From0031DeletesLegacyCloudAppIDs 验证从 0031 数据快照升级到 latest 时，
// 0032（已发布 DELETE 版本）会删除旧格式 cloud-<account_id> 应用及其关联的 asset_resource /
// asset_match_rule，但不处理 alert_alert 和 inspection_policy 中的旧格式引用。
// 0037 因 has_old=false（旧应用已被 0032 删除）而跳过这两个表。
// 0039 补全：将 alert_alert.application_id 和 inspection_policy.scope.application_ids 中
// 残留的旧格式孤儿引用改写为新格式 cloud-<prefix>-<hash>。
// 0042 补建：为仍被引用但不存在的新格式 cloud application ID 补建 asset_application 记录，
// 硬验收守卫确保 v_asset_app_ref_integrity 返回 0 行。
// 覆盖 account_id 不含 '-'（纯数字，常见）与含 '-'（如 acc-xxx）两类账号。
//
// 该测试在独立临时数据库中执行，避免影响共享的 aiops 测试库；PG 不可用即 Skip。
func TestMigrateUpgrade_From0031DeletesLegacyCloudAppIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	// docker-compose 中 aiops 账号为 POSTGRES_USER（superuser），具备 createdb 权限。
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 只应用到 0031（含），构建 0031 数据快照基线。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0031"}); err != nil {
		t.Fatalf("run migrations up to 0031: %v", err)
	}

	// 2) 灌入旧格式数据：两个账号（无连字符 / 含连字符）+ 旧 cloud-<account_id> 应用及其资源、匹配规则、告警，
	//    以及一个非 cloud- 控制应用，用于确认 0032 不误伤无关应用。
	const (
		accountNoDash = "1234567890123456789" // 19 位纯数字
		accountDash   = "acc-xyz-001"         // 含 '-'
		controlAppID  = "manual-control-app-1"
	)
	oldNoDash := "cloud-" + accountNoDash
	oldDash := "cloud-" + accountDash
	seedLegacyCloudData(t, edb, accountNoDash, accountDash, oldNoDash, oldDash, controlAppID)

	// 3) 应用 0032..latest：0032 DELETE 删除旧应用及其资源/规则，0039 清理 alert_alert /
	//    inspection_policy 中的孤儿引用。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations 0032..latest: %v", err)
	}

	// 4) 断言：0032 删除旧应用/资源/规则；0039 把 alert_alert 和
	//    inspection_policy 中残留的旧格式引用改写为新格式；0042 补建新格式应用。
	wantNoDash := cloudAppIDForTest(accountNoDash)
	wantDash := cloudAppIDForTest(accountDash)

	for _, tc := range []struct {
		name      string
		oldID     string
		newID     string
		accountID string
	}{
		{name: "no-dash", oldID: oldNoDash, newID: wantNoDash, accountID: accountNoDash},
		{name: "dash", oldID: oldDash, newID: wantDash, accountID: accountDash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := countWhere(t, edb, "asset_application", "application_id = ?", tc.oldID); got != 0 {
				t.Errorf("asset_application 旧 application_id=%s 仍存在 %d 行，期望 0（0032 已删除）", tc.oldID, got)
			}
			if got := countWhere(t, edb, "asset_application", "application_id = ?", tc.newID); got != 1 {
				t.Errorf("asset_application 新 application_id=%s 期望 1 行（0042 补建），得到 %d", tc.newID, got)
			}
			if got := countWhere(t, edb, "asset_resource", "application_id = ?", tc.oldID); got != 0 {
				t.Errorf("asset_resource 仍指向旧 application_id=%s %d 行，期望 0（0032 已删除）", tc.oldID, got)
			}
			if got := countWhere(t, edb, "asset_resource", "application_id = ?", tc.newID); got != 0 {
				t.Errorf("asset_resource 指向新 application_id=%s 期望 0 行（0032 是 DELETE 不迁移资源），得到 %d", tc.newID, got)
			}
			if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", tc.oldID); got != 0 {
				t.Errorf("asset_match_rule 仍指向旧 application_id=%s %d 行，期望 0（0032 已删除）", tc.oldID, got)
			}
			if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", tc.newID); got != 0 {
				t.Errorf("asset_match_rule 指向新 application_id=%s 期望 0 行（0032 是 DELETE 不迁移规则），得到 %d", tc.newID, got)
			}
			if got := countWhere(t, edb, "alert_alert", "application_id = ?", tc.oldID); got != 0 {
				t.Errorf("alert_alert 仍指向旧 application_id=%s %d 行，期望 0（0039 应已改写）", tc.oldID, got)
			}
			if got := countWhere(t, edb, "alert_alert", "application_id = ?", tc.newID); got != 1 {
				t.Errorf("alert_alert 指向新 application_id=%s 期望 1 行（0039 改写），得到 %d", tc.newID, got)
			}
			if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-"+tc.accountID, `["`+tc.oldID+`"]`); got != 0 {
				t.Errorf("inspection_policy %s scope 仍包含旧 application_id=%s，期望 0（0039 应已改写）", "policy-"+tc.accountID, tc.oldID)
			}
			if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-"+tc.accountID, `["`+tc.newID+`"]`); got != 1 {
				t.Errorf("inspection_policy %s scope 应包含新 application_id=%s，得到 %d 行，期望 1（0039 改写）", "policy-"+tc.accountID, tc.newID, got)
			}
			if got := countWhere(t, edb, "integration_account", "account_id = ?", tc.accountID); got != 1 {
				t.Errorf("integration_account 账号 %s 应保留 1 行，得到 %d", tc.accountID, got)
			}
		})
	}

	// 5) 控制应用（非 cloud- 旧格式）不受影响。
	if got := countWhere(t, edb, "asset_application", "application_id = ?", controlAppID); got != 1 {
		t.Errorf("控制应用 %s 应保持不变（1 行），得到 %d", controlAppID, got)
	}

	// 6) 0042 补建的新格式应用字段与 ensureCloudApplication 一致。
	for _, tc := range []struct {
		accountID string
		newID     string
	}{
		{accountID: accountNoDash, newID: wantNoDash},
		{accountID: accountDash, newID: wantDash},
	} {
		var name, environment, description string
		if err := edb.Raw("SELECT name, environment, description FROM asset_application WHERE application_id = ?", tc.newID).Row().Scan(&name, &environment, &description); err != nil {
			t.Fatalf("query backfilled app %s: %v", tc.newID, err)
		}
		wantName := "huawei_cloud-cloud-" + tc.accountID
		if name != wantName {
			t.Errorf("补建应用 %s name = %q, want %q", tc.newID, name, wantName)
		}
		if environment != "cloud" {
			t.Errorf("补建应用 %s environment = %q, want %q", tc.newID, environment, "cloud")
		}
		wantDesc := "Auto-created cloud sync application for account " + tc.accountID
		if description != wantDesc {
			t.Errorf("补建应用 %s description = %q, want %q", tc.newID, description, wantDesc)
		}
	}

	// 7) 0042 补建新格式应用后，引用完整性视图应返回 0 行（无孤儿引用）。
	//    0042 硬验收守卫已在此断言前通过（否则迁移失败）。
	if got := countWhere(t, edb, "v_asset_app_ref_integrity", ""); got != 0 {
		t.Errorf("v_asset_app_ref_integrity 期望 0 行（0042 已补建新格式应用），得到 %d", got)
	}
}

// TestMigrateUpgrade_From0031OldAndNewCoexist 验证从 0031 数据快照升级到 latest 时，
// 当同一账号的旧格式 cloud-<account_id> 和新格式 cloud-<prefix>-<hash> 应用并存：
// 0032 DELETE 删除旧应用及其资源/规则（新应用及其数据不受影响）；
// 0037 因 has_old=false 而跳过；
// 0039 把 alert_alert 和 inspection_policy 中旧格式孤儿引用改写为新格式。
//
// 该测试在独立临时数据库中执行，避免影响共享的 aiops 测试库；PG 不可用即 Skip。
func TestMigrateUpgrade_From0031OldAndNewCoexist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 只应用到 0031（含），构建 0031 数据快照基线。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0031"}); err != nil {
		t.Fatalf("run migrations up to 0031: %v", err)
	}

	// 2) 灌入新旧并存数据：同一账号的旧格式和新格式应用各一条，各带资源/规则/告警/策略。
	const accountID = "coexist-acct-001"
	oldAppID := "cloud-" + accountID
	newAppID := cloudAppIDForTest(accountID)
	if oldAppID == newAppID {
		t.Fatalf("测试前提不成立：旧格式与新格式 application_id 应不同，账号 %q", accountID)
	}
	seedLegacyAndNewCloudData(t, edb, accountID, oldAppID, newAppID)

	// 3) 应用 0032..latest：0032 DELETE 删除旧应用/资源/规则，0039 清理孤儿引用。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations 0032..latest: %v", err)
	}

	// 4) 断言：旧应用/资源/规则被 0032 删除；新应用/资源/规则不受影响。
	if got := countWhere(t, edb, "asset_application", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_application 旧 application_id=%s 仍存在 %d 行，期望 0（0032 已删除）", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_application", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_application 新 application_id=%s 期望 1 行（不受影响），得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "asset_resource", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_resource 旧 application_id=%s 仍存在 %d 行，期望 0（0032 已删除）", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_resource", "application_id = ?", newAppID); got != 2 {
		t.Errorf("asset_resource 新 application_id=%s 期望 2 行（不受影响），得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_match_rule 旧 application_id=%s 仍存在 %d 行，期望 0（0032 已删除）", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_match_rule 新 application_id=%s 期望 1 行（不受影响），得到 %d", newAppID, got)
	}

	// 5) alert_alert：旧应用告警被 0039 改写为新 application_id，新应用告警不受影响。
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("alert_alert 旧 application_id=%s 仍存在 %d 行，期望 0（0039 应已改写）", oldAppID, got)
	}
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", newAppID); got != 2 {
		t.Errorf("alert_alert 新 application_id=%s 期望 2 行（1 原始 + 1 由 0039 改写），得到 %d", newAppID, got)
	}

	// 6) inspection_policy：旧 ID 被替换为新 ID，新 ID 策略不受影响。
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-coexist-old", `["`+oldAppID+`"]`); got != 0 {
		t.Errorf("inspection_policy policy-coexist-old scope 仍包含旧 application_id=%s，期望 0（0039 应已改写）", oldAppID)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-coexist-old", `["`+newAppID+`"]`); got != 1 {
		t.Errorf("inspection_policy policy-coexist-old scope 应包含新 application_id=%s，得到 %d 行，期望 1（0039 改写）", newAppID, got)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-coexist-new", `["`+newAppID+`"]`); got != 1 {
		t.Errorf("inspection_policy policy-coexist-new scope 应包含新 application_id=%s，得到 %d 行，期望 1（不受影响）", newAppID, got)
	}

	// 7) integration_account 保留不变。
	if got := countWhere(t, edb, "integration_account", "account_id = ?", accountID); got != 1 {
		t.Errorf("integration_account 账号 %s 应保留 1 行，得到 %d", accountID, got)
	}

	// 5) 0040 引用完整性视图：新格式应用存在且所有引用指向它 -> 视图应返回 0 行。
	if got := countWhere(t, edb, "v_asset_app_ref_integrity", ""); got != 0 {
		t.Errorf("v_asset_app_ref_integrity 期望 0 行（无孤儿引用），得到 %d", got)
	}
}

// TestMigrateUpgrade_From0031OnlyNewAppUnaffected 验证从 0031 数据快照升级到 latest 时，
// 当账号仅存在新格式 cloud-<prefix>-<hash> 应用（无旧格式应用）：
// 0032 精确匹配 'cloud-' || account_id 不命中新格式应用，不删除任何数据；
// 0037 has_old=false/has_new=true，跳过；0039 无孤儿引用，跳过。
// 覆盖第五种场景：仅新格式应用。
//
// 该测试在独立临时数据库中执行，避免影响共享的 aiops 测试库；PG 不可用即 Skip。
func TestMigrateUpgrade_From0031OnlyNewAppUnaffected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 只应用到 0031（含）。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0031"}); err != nil {
		t.Fatalf("run migrations up to 0031: %v", err)
	}

	// 2) 灌入仅新格式数据：一个含连字符账号 + 新格式 cloud-<prefix>-<hash> 应用及其资源、规则、告警、策略。
	const accountID = "acc-new-only-001"
	newAppID := cloudAppIDForTest(accountID)
	seedOnlyNewCloudData(t, edb, accountID, newAppID)

	// 3) 应用 0032..latest。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations 0032..latest: %v", err)
	}

	// 4) 断言：新格式应用及其全部关联数据不受 0032/0037/0039 影响。
	if got := countWhere(t, edb, "asset_application", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_application 新 application_id=%s 期望 1 行（不受影响），得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "asset_resource", "application_id = ?", newAppID); got != 2 {
		t.Errorf("asset_resource 新 application_id=%s 期望 2 行（不受影响），得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_match_rule 新 application_id=%s 期望 1 行（不受影响），得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", newAppID); got != 1 {
		t.Errorf("alert_alert 新 application_id=%s 期望 1 行（不受影响），得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-new-only", `["`+newAppID+`"]`); got != 1 {
		t.Errorf("inspection_policy policy-new-only scope 应包含新 application_id=%s，得到 %d 行，期望 1", newAppID, got)
	}
	if got := countWhere(t, edb, "integration_account", "account_id = ?", accountID); got != 1 {
		t.Errorf("integration_account 账号 %s 应保留 1 行，得到 %d", accountID, got)
	}

	// 5) 0040 引用完整性视图：新格式应用存在且所有引用指向它 -> 视图应返回 0 行。
	if got := countWhere(t, edb, "v_asset_app_ref_integrity", ""); got != 0 {
		t.Errorf("v_asset_app_ref_integrity 期望 0 行（无孤儿引用），得到 %d", got)
	}
}

func seedOnlyNewCloudData(t *testing.T, db *gorm.DB, accountID, newAppID string) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed exec failed: %s: %v", sql, err)
		}
	}

	exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		accountID, "huawei-new-only", "huawei_cloud", "ak_sk", now, now)

	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		newAppID, "cloud-new-only", "cloud", now, now)

	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-new-only-a", newAppID, now, now)
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-new-only-b", newAppID, now, now)

	exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-new-only", "rule-new", "alert", "cloud_account", accountID, newAppID, now, now)

	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-new-only", "huawei_ces", "dk-new-only", "alert-new", "p2", "new", newAppID, now, now, now, now)

	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-new-only", "policy-new", `{"application_ids":["`+newAppID+`"]}`, now, now)
}

func seedLegacyAndNewCloudData(t *testing.T, db *gorm.DB, accountID, oldAppID, newAppID string) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed exec failed: %s: %v", sql, err)
		}
	}

	// 接入账号。
	exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		accountID, "huawei-coexist", "huawei_cloud", "ak_sk", now, now)

	// 旧格式应用 + 新格式应用。
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		oldAppID, "cloud-coexist-old", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		newAppID, "cloud-coexist-new", "cloud", now, now)

	// 资源：旧应用 2 条 + 新应用 2 条。
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-coexist-old-a", oldAppID, now, now)
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-coexist-old-b", oldAppID, now, now)
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-coexist-new-a", newAppID, now, now)
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-coexist-new-b", newAppID, now, now)

	// 匹配规则：旧应用 1 条 + 新应用 1 条。
	exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-coexist-old", "rule-old", "alert", "cloud_account", accountID, oldAppID, now, now)
	exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-coexist-new", "rule-new", "alert", "cloud_account", accountID, newAppID, now, now)

	// 告警：旧应用 1 条 + 新应用 1 条（dedup_key 互异）。
	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-coexist-old", "huawei_ces", "dk-coexist-old", "alert-old", "p2", "new", oldAppID, now, now, now, now)
	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-coexist-new", "huawei_ces", "dk-coexist-new", "alert-new", "p2", "new", newAppID, now, now, now, now)

	// 巡检策略：旧 ID 策略 + 新 ID 策略。
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-coexist-old", "policy-old", `{"application_ids":["`+oldAppID+`"]}`, now, now)
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-coexist-new", "policy-new", `{"application_ids":["`+newAppID+`"]}`, now, now)
}

// cloudAppIDForTest 镜像 internal/asset/application/sync_service.go:cloudApplicationID 的新格式算法，
// 用于在测试中说明期望的新 application_id。按 rune 截取前 17 位，与 PostgreSQL left(...,17) 一致。
func cloudAppIDForTest(accountID string) string {
	id := strings.TrimSpace(accountID)
	h := sha1.Sum([]byte(id))
	suffix := hex.EncodeToString(h[:])[:12]
	if r := []rune(id); len(r) > 17 {
		id = string(r[:17])
	}
	return "cloud-" + id + "-" + suffix
}

// cloudAppIDForTestLegacyByte 复现 cloudApplicationID 修复前的字节截断算法（id[:17]），
// 仅用于在升级测试中 seed 旧字节版 application_id，验证 0035 把它改写为 rune 版。
// 注意：当 17 字节边界落在多字节字符中间时，本函数返回值含非法 UTF-8，无法写入 UTF8 库；
// 测试仅对边界落在字符上的可存储场景使用。
func cloudAppIDForTestLegacyByte(accountID string) string {
	id := strings.TrimSpace(accountID)
	h := sha1.Sum([]byte(id))
	suffix := hex.EncodeToString(h[:])[:12]
	if len(id) > 17 {
		id = id[:17]
	}
	return "cloud-" + id + "-" + suffix
}

// TestMigrateUpgrade_RewritesByteTruncatedCloudAppIDs 验证 0035 把旧字节版 cloud application_id
// 无损改写为 rune 版（与修复后的 cloudApplicationID / PostgreSQL left(...,17) 一致），而非删除旧应用数据。
// 覆盖可存储分歧场景：中文账号 17 字节边界恰好落在字符边界，字节版前缀(5中文+2ASCII)与 rune 版
// 前缀(7中文+10ASCII)不同，但二者共享同一 sha1 后缀，0035 据此关联账号精确改写。
//
// 该测试在独立临时数据库中执行，避免影响共享的 aiops 测试库；PG 不可用即 Skip。
func TestMigrateUpgrade_RewritesByteTruncatedCloudAppIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 只应用到 0034（含），构建 0034 数据快照基线（0035 尚未运行）。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0034"}); err != nil {
		t.Fatalf("run migrations up to 0034: %v", err)
	}

	// 2) 灌入旧字节版数据：一个中文账号 + 旧字节版 cloud 应用及其资源、匹配规则、告警，
	//    以及一个非 cloud 控制应用，用于确认 0035 不误伤无关应用。
	const (
		// 5 中文(15字节)+15 ASCII=20 rune/30 字节：旧字节版 id[:17]=5中文+2ASCII(可存储,7 rune)，
		// rune 版=前 17 rune(5中文+12ASCII)，二者不同——可存储分歧场景。
		accountID    = "中文字符测ab0123456789xyz"
		controlAppID = "manual-control-byte-1"
	)
	oldAppID := cloudAppIDForTestLegacyByte(accountID)
	newAppID := cloudAppIDForTest(accountID)
	if oldAppID == newAppID {
		t.Fatalf("测试前提不成立：字节版与 rune 版应不同，账号 %q", accountID)
	}
	seedByteTruncatedCloudData(t, edb, accountID, oldAppID, controlAppID)

	// 3) 应用 0035..latest，触发字节版 -> rune 版改写。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations 0035..latest: %v", err)
	}

	// 4) 断言：旧字节版应用被改写而非删除，引用表同步指向新 application_id。
	if got := countWhere(t, edb, "asset_application", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_application 旧字节版 application_id=%s 仍存在 %d 行，期望 0（应已改写）", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_application", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_application rune 版 application_id=%s 期望 1 行，得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "asset_resource", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_resource 仍指向旧字节版 application_id=%s %d 行，期望 0", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_resource", "application_id = ?", newAppID); got != 2 {
		t.Errorf("asset_resource 指向 rune 版 application_id=%s 期望 2 行，得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_match_rule 仍指向旧字节版 application_id=%s %d 行，期望 0", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_match_rule", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_match_rule 指向 rune 版 application_id=%s 期望 1 行，得到 %d", newAppID, got)
	}
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("alert_alert 仍指向旧字节版 application_id=%s %d 行，期望 0", oldAppID, got)
	}
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", newAppID); got != 1 {
		t.Errorf("alert_alert 指向 rune 版 application_id=%s 期望 1 行，得到 %d", newAppID, got)
	}

	// 5) integration_account 保留不变。
	if got := countWhere(t, edb, "integration_account", "account_id = ?", accountID); got != 1 {
		t.Errorf("integration_account 账号 %s 应保留 1 行，得到 %d", accountID, got)
	}

	// 6) 控制应用（非 cloud-）不受影响。
	if got := countWhere(t, edb, "asset_application", "application_id = ?", controlAppID); got != 1 {
		t.Errorf("控制应用 %s 应保持不变（1 行），得到 %d", controlAppID, got)
	}
}

func seedByteTruncatedCloudData(t *testing.T, db *gorm.DB, accountID, oldAppID, controlAppID string) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed exec failed: %s: %v", sql, err)
		}
	}

	// 接入账号（0035 按 application_id 后缀关联 account_id 的 sha1，账号必须存在）。
	exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		accountID, "huawei-cn", "huawei_cloud", "ak_sk", now, now)

	// 旧字节版应用 + 控制应用。
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		oldAppID, "cloud-cn-byte", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		controlAppID, "manual-control", "default", now, now)

	// 资源：旧应用 2 条（source 默认 manual，避开 0034 的 cloud_sync 分支与部分唯一索引）。
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-cn-byte-a", oldAppID, now, now)
	exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
		"res-cn-byte-b", oldAppID, now, now)

	// 匹配规则：旧应用 1 条。
	exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-cn-byte", "rule-cn", "alert", "cloud_account", accountID, oldAppID, now, now)

	// 告警：旧应用 1 条。
	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-cn-byte", "huawei_ces", "dk-cn-byte", "alert-cn", "p2", "new", oldAppID, now, now, now, now)
}

// TestMigrateUpgrade_HuaweiCesApplicationIdMerge covers the safety contract requested by P0:
// 1) only legacy application exists,
// 2) only new application exists,
// 3) legacy and new applications both exist,
// 4) duplicate references are deduplicated before old application deletion.
func TestMigrateUpgrade_HuaweiCesApplicationIdMerge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")
	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)
	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})
	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0036"}); err != nil {
		t.Fatalf("run migrations up to 0036: %v", err)
	}

	legacyAccount := "legacy-merge-001"
	newOnlyAccount := "new-only-002"
	bothAccount := "both-merge-003"
	seedHuaweiCesMergeData(t, edb, legacyAccount, newOnlyAccount, bothAccount)

	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations 0037..latest: %v", err)
	}

	assertHuaweiCesMerged(t, edb, legacyAccount, false, true, 2)
	assertHuaweiCesMerged(t, edb, newOnlyAccount, false, true, 2)
	assertHuaweiCesMerged(t, edb, bothAccount, false, true, 4)

	// 4) inspection_policy.scope.application_ids 旧 ID 必须被替换为新 ID，而非丢失（0037 回归保护）。
	//    policy-both-legacy-only 只含 both-exist 账号的旧 ID -> 触发 section 1；
	//    policy-legacy-only 只含 only-legacy 账号的旧 ID -> 触发 section 2。
	//    修复前内部 WHERE 会把旧 ID 过滤掉导致 scope 变空，此处断言新 ID 存在即可抓住该回归。
	bothLegacyID := "cloud-" + bothAccount
	bothNewID := cloudAppIDForTest(bothAccount)
	legacyLegacyID := "cloud-" + legacyAccount
	legacyNewID := cloudAppIDForTest(legacyAccount)
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-both-legacy-only", `["`+bothNewID+`"]`); got != 1 {
		t.Errorf("policy-both-legacy-only scope 应包含新 application_id=%s，得到 %d 行，期望 1（0037 section 1 不得丢弃旧 ID）", bothNewID, got)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-both-legacy-only", `["`+bothLegacyID+`"]`); got != 0 {
		t.Errorf("policy-both-legacy-only scope 不应再包含旧 application_id=%s，得到 %d 行", bothLegacyID, got)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-legacy-only", `["`+legacyNewID+`"]`); got != 1 {
		t.Errorf("policy-legacy-only scope 应包含新 application_id=%s，得到 %d 行，期望 1（0037 section 2 不得丢弃旧 ID）", legacyNewID, got)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-legacy-only", `["`+legacyLegacyID+`"]`); got != 0 {
		t.Errorf("policy-legacy-only scope 不应再包含旧 application_id=%s，得到 %d 行", legacyLegacyID, got)
	}
}

// TestMigrateUpgrade_ReadMigrationStatusReportsChecksumDrift 验证 ReadMigrationStatus 的端到端 drift 检测：
//  1. 干净迁移后无 drift；
//  2. 手动篡改某版本 checksum（旧 checksum 场景）-> ReadMigrationStatus 报告 drift，且 UpToDate 仍为 true；
//  3. 手动清空某版本 checksum（空 checksum 场景）-> 同样报告 drift；
//  4. ReadMigrationStatus 是只读操作，不做 backfill，再次调用仍报告 drift。
//
// PG 不可用时 Skip（CI 设 AIOPS_TEST_REQUIRE_PG=1 则 Fatal）。
func TestMigrateUpgrade_ReadMigrationStatusReportsChecksumDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 应用全部迁移，干净状态下不应有 drift。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	status, err := ReadMigrationStatus(ctx, edb, MigrateOptions{Dir: migrationsDir})
	if err != nil {
		t.Fatalf("read migration status (clean): %v", err)
	}
	if len(status.ChecksumDrifts) != 0 {
		t.Fatalf("expected 0 drifts after clean migration, got %d: %+v", len(status.ChecksumDrifts), status.ChecksumDrifts)
	}
	if !status.UpToDate {
		t.Errorf("expected UpToDate=true after clean migration, got false")
	}

	// 2) 篡改 0001 的 checksum（旧 checksum 场景）。
	if err := edb.Exec(`UPDATE schema_migrations SET checksum = 'stale-old-checksum' WHERE version = '0001'`).Error; err != nil {
		t.Fatalf("corrupt checksum for 0001: %v", err)
	}
	status, err = ReadMigrationStatus(ctx, edb, MigrateOptions{Dir: migrationsDir})
	if err != nil {
		t.Fatalf("read migration status (stale 0001): %v", err)
	}
	foundStale := false
	for _, d := range status.ChecksumDrifts {
		if d.Version == "0001" {
			foundStale = true
			if d.Stored != "stale-old-checksum" {
				t.Errorf("0001 drift: expected stored 'stale-old-checksum', got %s", d.Stored)
			}
		}
	}
	if !foundStale {
		t.Errorf("expected drift for 0001 (stale checksum), drifts: %+v", status.ChecksumDrifts)
	}
	// UpToDate 仍为 true：所有文件已应用，drift 是独立的健康维度。
	if !status.UpToDate {
		t.Errorf("expected UpToDate=true (all applied, drift is separate), got false")
	}

	// 3) 清空 0002 的 checksum（空 checksum 场景）。
	if err := edb.Exec(`UPDATE schema_migrations SET checksum = '' WHERE version = '0002'`).Error; err != nil {
		t.Fatalf("empty checksum for 0002: %v", err)
	}
	status, err = ReadMigrationStatus(ctx, edb, MigrateOptions{Dir: migrationsDir})
	if err != nil {
		t.Fatalf("read migration status (empty 0002): %v", err)
	}
	foundEmpty := false
	for _, d := range status.ChecksumDrifts {
		if d.Version == "0002" {
			foundEmpty = true
			if d.Stored != "" {
				t.Errorf("0002 drift: expected empty stored, got %s", d.Stored)
			}
		}
	}
	if !foundEmpty {
		t.Errorf("expected drift for 0002 (empty checksum), drifts: %+v", status.ChecksumDrifts)
	}
	// 至少 2 个 drift：0001 stale + 0002 empty。
	if len(status.ChecksumDrifts) < 2 {
		t.Errorf("expected at least 2 drifts, got %d: %+v", len(status.ChecksumDrifts), status.ChecksumDrifts)
	}

	// 4) ReadMigrationStatus 是只读的：再次调用仍报告相同 drift（不做 backfill）。
	status2, err := ReadMigrationStatus(ctx, edb, MigrateOptions{Dir: migrationsDir})
	if err != nil {
		t.Fatalf("read migration status (re-read): %v", err)
	}
	if len(status2.ChecksumDrifts) != len(status.ChecksumDrifts) {
		t.Errorf("re-read should report same drift count: first=%d second=%d", len(status.ChecksumDrifts), len(status2.ChecksumDrifts))
	}
	// 确认 0002 的 checksum 仍为空（未被 backfill）。
	var stillEmpty string
	if err := edb.Raw(`SELECT COALESCE(checksum, '') FROM schema_migrations WHERE version = '0002'`).Scan(&stillEmpty).Error; err != nil {
		t.Fatalf("verify 0002 checksum still empty: %v", err)
	}
	if stillEmpty != "" {
		t.Errorf("ReadMigrationStatus should not backfill, but 0002 checksum is now %s", stillEmpty)
	}
}

// TestMigrateUpgrade_0042BackfillsMultiByteAccount 验证 0042 对多字节（中文）账号
// 的补建行为：0032 删除旧格式应用，0039 把 alert/inspection 引用改写为新格式，
// 0042 按新格式 application_id 补建 asset_application，硬验收通过。
//
// 该测试在独立临时数据库中执行，避免影响共享的 aiops 测试库；PG 不可用即 Skip。
func TestMigrateUpgrade_0042BackfillsMultiByteAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 只应用到 0031（含）。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0031"}); err != nil {
		t.Fatalf("run migrations up to 0031: %v", err)
	}

	// 2) 灌入多字节账号数据：中文账号 + 旧格式应用 + 告警 + 巡检策略。
	const accountID = "中文测试账号001"
	oldAppID := "cloud-" + accountID
	newAppID := cloudAppIDForTest(accountID)
	if oldAppID == newAppID {
		t.Fatalf("测试前提不成立：旧格式与新格式 application_id 应不同，账号 %q", accountID)
	}
	seedMultiByteCloudData(t, edb, accountID, oldAppID)

	// 3) 应用 0032..latest：0032 DELETE 删除旧应用，0039 改写引用，0042 补建新格式应用。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations 0032..latest: %v", err)
	}

	// 4) 断言：旧应用删除，新格式应用由 0042 补建。
	if got := countWhere(t, edb, "asset_application", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("asset_application 旧 application_id=%s 仍存在 %d 行，期望 0（0032 已删除）", oldAppID, got)
	}
	if got := countWhere(t, edb, "asset_application", "application_id = ?", newAppID); got != 1 {
		t.Errorf("asset_application 新 application_id=%s 期望 1 行（0042 补建），得到 %d", newAppID, got)
	}

	// 5) 补建应用字段与 ensureCloudApplication 一致。
	var name, environment, description string
	if err := edb.Raw("SELECT name, environment, description FROM asset_application WHERE application_id = ?", newAppID).Row().Scan(&name, &environment, &description); err != nil {
		t.Fatalf("query backfilled app %s: %v", newAppID, err)
	}
	wantName := "huawei_cloud-cloud-" + accountID
	if name != wantName {
		t.Errorf("补建应用 name = %q, want %q", name, wantName)
	}
	if environment != "cloud" {
		t.Errorf("补建应用 environment = %q, want %q", environment, "cloud")
	}
	wantDesc := "Auto-created cloud sync application for account " + accountID
	if description != wantDesc {
		t.Errorf("补建应用 description = %q, want %q", description, wantDesc)
	}

	// 6) alert_alert 引用已改写为新格式。
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", oldAppID); got != 0 {
		t.Errorf("alert_alert 旧 application_id=%s 仍存在 %d 行，期望 0（0039 应已改写）", oldAppID, got)
	}
	if got := countWhere(t, edb, "alert_alert", "application_id = ?", newAppID); got != 1 {
		t.Errorf("alert_alert 新 application_id=%s 期望 1 行（0039 改写），得到 %d", newAppID, got)
	}

	// 7) inspection_policy 引用已改写为新格式。
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-cn-test", `["`+oldAppID+`"]`); got != 0 {
		t.Errorf("inspection_policy scope 仍包含旧 application_id=%s，期望 0（0039 应已改写）", oldAppID)
	}
	if got := countWhere(t, edb, "inspection_policy", "policy_id = ? AND scope->'application_ids' @> ?::jsonb", "policy-cn-test", `["`+newAppID+`"]`); got != 1 {
		t.Errorf("inspection_policy scope 应包含新 application_id=%s，得到 %d 行，期望 1", newAppID, got)
	}

	// 8) 引用完整性视图返回 0 行（0042 补建后无孤儿）。
	if got := countWhere(t, edb, "v_asset_app_ref_integrity", ""); got != 0 {
		t.Errorf("v_asset_app_ref_integrity 期望 0 行（0042 已补建），得到 %d", got)
	}
}

func seedMultiByteCloudData(t *testing.T, db *gorm.DB, accountID, oldAppID string) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed exec failed: %s: %v", sql, err)
		}
	}

	exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		accountID, "huawei-cn-test", "huawei_cloud", "ak_sk", now, now)

	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		oldAppID, "cloud-cn-old", "cloud", now, now)

	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-cn-test", "huawei_ces", "dk-cn-test", "alert-cn", "p2", "new", oldAppID, now, now, now, now)

	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-cn-test", "policy-cn", `{"application_ids":["`+oldAppID+`"]}`, now, now)
}

// TestMigrateUpgrade_0042GuardFailsOnUnbackfillableOrphan 验证 0042 硬验收守卫：
// 当存在无法通过 integration_account 补建的非 cloud 格式孤儿引用时，
// CHECK(n=0) 约束失败导致迁移终止，runner 不记录 0042 版本号。
//
// 该测试在独立临时数据库中执行，避免影响共享的 aiops 测试库；PG 不可用即 Skip。
func TestMigrateUpgrade_0042GuardFailsOnUnbackfillableOrphan(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PG upgrade integration test in short mode")
	}
	migrationsDir := filepath.Join(findRepoRoot(t), "migrations")

	cfg := testDatabaseConfig()
	mdb := requirePostgres(t, cfg)

	dbName := fmt.Sprintf("aiops_migtest_%d", time.Now().UnixNano())
	_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
	if err := mdb.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Error; err != nil {
		t.Fatalf("create ephemeral db %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_ = mdb.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, dbName)).Error
		_ = ClosePostgres(mdb)
	})

	ephCfg := cfg
	ephCfg.Name = dbName
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	edb, err := NewPostgres(ctx, ephCfg, "Asia/Shanghai")
	if err != nil {
		t.Fatalf("open ephemeral db: %v", err)
	}
	defer func() { _ = ClosePostgres(edb) }()

	// 1) 应用到 0041（含），构建 0042 前基线。
	if err := RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir, TargetVersion: "0041"}); err != nil {
		t.Fatalf("run migrations up to 0041: %v", err)
	}

	// 2) 灌入非 cloud 格式孤儿引用：alert 引用不存在的 application_id，
	//    0042 无法通过 integration_account 补建，硬验收守卫应阻断迁移。
	const orphanAppID = "orphan-non-cloud-app-001"
	now := time.Now().UTC()
	if err := edb.Exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-orphan", "huawei_ces", "dk-orphan", "alert-orphan", "p2", "new", orphanAppID, now, now, now, now).Error; err != nil {
		t.Fatalf("seed orphan alert: %v", err)
	}

	// 3) 应用 0042..latest：期望迁移失败（CHECK 约束违反）。
	err = RunMigrations(ctx, edb, MigrateOptions{Dir: migrationsDir})
	if err == nil {
		t.Fatal("期望 0042 迁移因孤儿引用失败，但 RunMigrations 返回 nil")
	}
	if !strings.Contains(err.Error(), "0042") {
		t.Errorf("错误信息应包含 0042 版本号，得到: %v", err)
	}

	// 4) 确认 0042 未被记录（runner 失败不写 schema_migrations）。
	var count int64
	if err := edb.Raw("SELECT count(*) FROM schema_migrations WHERE version = '0042'").Scan(&count).Error; err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 0 {
		t.Errorf("schema_migrations 中不应有 0042 记录（迁移失败），得到 %d", count)
	}
}

func seedLegacyCloudData(t *testing.T, db *gorm.DB, accountNoDash, accountDash, oldNoDash, oldDash, controlAppID string) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed exec failed: %s: %v", sql, err)
		}
	}

	// 接入账号（0032 按 application_id = 'cloud-' || account_id 关联，账号必须存在）。
	exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		accountNoDash, "huawei-nodash", "huawei_cloud", "ak_sk", now, now)
	exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		accountDash, "huawei-dash", "huawei_cloud", "ak_sk", now, now)

	// 旧格式应用 + 控制应用。
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		oldNoDash, "cloud-nodash", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		oldDash, "cloud-dash", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		controlAppID, "manual-control", "default", now, now)

	// 资源：每个旧应用 2 条（source 默认 manual，避开 0034 的 cloud_sync 分支与部分唯一索引）。
	for _, app := range []string{oldNoDash, oldDash} {
		exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
			"res-"+app+"-a", app, now, now)
		exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)",
			"res-"+app+"-b", app, now, now)
	}

	// 匹配规则：每个旧应用 1 条。
	exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-"+accountNoDash, "rule-nodash", "alert", "cloud_account", accountNoDash, oldNoDash, now, now)
	exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-"+accountDash, "rule-dash", "alert", "cloud_account", accountDash, oldDash, now, now)

	// 告警：每个旧应用 1 条，dedup_key 互异，status=new（避开 closed 之外的 active 去重冲突）。
	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-"+accountNoDash, "huawei_ces", "dk-"+accountNoDash, "alert-nodash", "p2", "new", oldNoDash, now, now, now, now)
	exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"alert-"+accountDash, "huawei_ces", "dk-"+accountDash, "alert-dash", "p2", "new", oldDash, now, now, now, now)

	// 巡检策略：scope.application_ids 引用旧格式应用，验证升级后引用被改写为新 ID 而非丢失。
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-"+accountNoDash, "policy-nodash", `{"application_ids":["`+oldNoDash+`"]}`, now, now)
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-"+accountDash, "policy-dash", `{"application_ids":["`+oldDash+`"]}`, now, now)
}

func countWhere(t *testing.T, db *gorm.DB, table, where string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	q := db.Table(table)
	if where != "" {
		q = q.Where(where, args...)
	}
	if err := q.Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func seedHuaweiCesMergeData(t *testing.T, db *gorm.DB, legacyAccount, newOnlyAccount, bothAccount string) {
	t.Helper()
	now := time.Now().UTC()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("seed exec failed: %s: %v", sql, err)
		}
	}
	for _, account := range []string{legacyAccount, newOnlyAccount, bothAccount} {
		exec("INSERT INTO integration_account (account_id, name, provider, auth_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			account, "huawei-"+account, "huawei_cloud", "ak_sk", now, now)
	}
	legacyID := "cloud-" + legacyAccount
	bothLegacyID := "cloud-" + bothAccount
	bothNewID := cloudAppIDForTest(bothAccount)
	newOnlyID := cloudAppIDForTest(newOnlyAccount)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", legacyID, "legacy-only", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", newOnlyID, "new-only", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", bothLegacyID, "both-legacy", "cloud", now, now)
	exec("INSERT INTO asset_application (application_id, name, environment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", bothNewID, "both-new", "cloud", now, now)
	for _, app := range []string{legacyID, newOnlyID, bothLegacyID, bothNewID} {
		exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)", "r1-"+app, app, now, now)
		exec("INSERT INTO asset_resource (resource_id, application_id, created_at, updated_at) VALUES (?, ?, ?, ?)", "r2-"+app, app, now, now)
		exec("INSERT INTO asset_match_rule (rule_id, name, target_type, label_key, label_value_pattern, application_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "m-"+app, "rule-"+app, "alert", "cloud_account", app, app, now, now)
		exec("INSERT INTO alert_alert (alert_id, source, dedup_key, name, severity, status, application_id, first_seen_at, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "a-"+app, "huawei_ces", "dk-"+app, "alert-"+app, "p2", "new", app, now, now, now, now)
	}
	// duplicate application_ids for bothAccount to verify dedupe in JSON array rewrite.
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-merge", "policy-merge", `{"application_ids":["`+legacyID+`","`+bothLegacyID+`","`+bothNewID+`","`+bothLegacyID+`"]}`, now, now)
	if err := db.Exec(`UPDATE inspection_policy SET scope = jsonb_set(scope, '{application_ids}', (
		SELECT jsonb_agg(value)
		FROM (
			SELECT DISTINCT value
			FROM jsonb_array_elements_text(scope->'application_ids')
		) t
	)) WHERE policy_id = ?`, "policy-merge").Error; err != nil {
		t.Fatalf("normalize inspection policy scope: %v", err)
	}

	// 策略仅引用旧 ID（both-exist 账号），验证 0037 section 1 把旧 ID 替换为新 ID 而非丢弃。
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-both-legacy-only", "policy-both-legacy-only", `{"application_ids":["`+bothLegacyID+`"]}`, now, now)
	// 策略仅引用旧 ID（only-legacy 账号），验证 0037 section 2 同样替换而非丢弃。
	exec("INSERT INTO inspection_policy (policy_id, name, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"policy-legacy-only", "policy-legacy-only", `{"application_ids":["`+legacyID+`"]}`, now, now)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func testDatabaseConfig() config.DatabaseConfig {
	return config.DatabaseConfig{
		Host:     testEnvOrDefault("AIOPS_TEST_DATABASE_HOST", "127.0.0.1"),
		Port:     testEnvIntOrDefault("AIOPS_TEST_DATABASE_PORT", 5432),
		User:     testEnvOrDefault("AIOPS_TEST_DATABASE_USER", "aiops"),
		Password: testEnvOrDefault("AIOPS_TEST_DATABASE_PASSWORD", "aiops"),
		Name:     testEnvOrDefault("AIOPS_TEST_DATABASE_NAME", "aiops"),
		SSLMode:  "disable",
	}
}

func openMaintenancePostgres(cfg config.DatabaseConfig) (*gorm.DB, error) {
	mCfg := cfg
	mCfg.Name = "postgres"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return NewPostgres(ctx, mCfg, "Asia/Shanghai")
}

// requirePostgres 打开维护用 PG 连接。当 AIOPS_TEST_REQUIRE_PG=1（CI 模式）时，
// PG 不可用直接 t.Fatal 而非 t.Skip，避免所有集成测试静默跳过后仍显示 PASS 的假象。
func requirePostgres(t *testing.T, cfg config.DatabaseConfig) *gorm.DB {
	t.Helper()
	mdb, err := openMaintenancePostgres(cfg)
	if err != nil {
		if os.Getenv("AIOPS_TEST_REQUIRE_PG") == "1" {
			t.Fatalf("AIOPS_TEST_REQUIRE_PG=1 but postgres unavailable: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	return mdb
}

func testEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func assertHuaweiCesMerged(t *testing.T, db *gorm.DB, accountID string, wantLegacy, wantNew bool, wantResources int64) {
	t.Helper()
	legacyID := "cloud-" + accountID
	newID := cloudAppIDForTest(accountID)
	if got := countWhere(t, db, "asset_application", "application_id = ?", legacyID); (got > 0) != wantLegacy {
		t.Fatalf("legacy application existence for %s = %v, want %v", accountID, got > 0, wantLegacy)
	}
	if got := countWhere(t, db, "asset_application", "application_id = ?", newID); (got > 0) != wantNew {
		t.Fatalf("new application existence for %s = %v, want %v", accountID, got > 0, wantNew)
	}
	if wantNew {
		if got := countWhere(t, db, "asset_resource", "application_id = ?", newID); got != wantResources {
			t.Errorf("resources for %s = %d, want %d", accountID, got, wantResources)
		}
	}
	if got := countWhere(t, db, "asset_resource", "application_id = ?", legacyID); got != 0 {
		t.Fatalf("legacy resources for %s remain: %d", accountID, got)
	}
	if got := countWhere(t, db, "asset_match_rule", "application_id = ?", legacyID); got != 0 {
		t.Fatalf("legacy rules for %s remain: %d", accountID, got)
	}
	if got := countWhere(t, db, "alert_alert", "application_id = ?", legacyID); got != 0 {
		t.Fatalf("legacy alerts for %s remain: %d", accountID, got)
	}
}

func testEnvIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
