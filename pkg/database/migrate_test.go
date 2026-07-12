package database

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSplitSQLStatements_BasicMultiStatement(t *testing.T) {
	sql := `-- header comment
CREATE TABLE foo (id INT);
CREATE INDEX idx_foo ON foo(id);
`
	got := splitSQLStatements(sql)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "CREATE TABLE foo") {
		t.Fatalf("stmt[0] = %q", got[0])
	}
	if !strings.HasPrefix(got[1], "CREATE INDEX idx_foo") {
		t.Fatalf("stmt[1] = %q", got[1])
	}
}

func TestSplitSQLStatements_SemicolonInsideSingleQuotedString(t *testing.T) {
	sql := `INSERT INTO t (c) VALUES ('a;b');
SELECT 1;
`
	got := splitSQLStatements(sql)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %v", len(got), got)
	}
	if !strings.Contains(got[0], `'a;b'`) {
		t.Fatalf("stmt[0] should keep quoted semicolon: %q", got[0])
	}
}

func TestSplitSQLStatements_EscapedSingleQuote(t *testing.T) {
	sql := `INSERT INTO t (c) VALUES ('it''s;ok');
SELECT 1;
`
	got := splitSQLStatements(sql)
	if len(got) != 2 {
		t.Fatalf("got %d statements, want 2: %v", len(got), got)
	}
	if !strings.Contains(got[0], `it''s;ok`) {
		t.Fatalf("stmt[0] = %q", got[0])
	}
}

func TestSplitSQLStatements_EmptyAndCommentOnly(t *testing.T) {
	if got := splitSQLStatements(""); len(got) != 0 {
		t.Fatalf("empty input: got %v", got)
	}
	if got := splitSQLStatements("-- only comment\n-- another\n"); len(got) != 0 {
		t.Fatalf("comment-only: got %v", got)
	}
}

func TestSplitSQLStatements_AllRepoMigrations(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("migrations dir not found: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		stmts := splitSQLStatements(string(content))
		if len(stmts) == 0 {
			t.Fatalf("%s: no executable statements after split", e.Name())
		}
		for i, stmt := range stmts {
			if strings.TrimSpace(stmt) == "" {
				t.Fatalf("%s: empty statement at index %d", e.Name(), i)
			}
		}
	}
}

func TestMigrationSeedPermissionUUIDsAreUnique(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("migrations dir not found: %v", err)
	}
	permissionUUID := regexp.MustCompile(`00000000-0000-0000-000[13]-[0-9]{12}`)
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, id := range permissionUUID.FindAllString(string(content), -1) {
			if prev, ok := seen[id]; ok {
				t.Fatalf("permission seed UUID %s is reused in %s and %s", id, prev, e.Name())
			}
			seen[id] = e.Name()
		}
	}
}

func TestLatestMigrationVersionMatchesFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	files, err := scanMigrationFiles(dir)
	if err != nil {
		t.Fatalf("scan migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}
	if got, want := files[len(files)-1].Version, "0043"; got != want {
		t.Fatalf("latest migration version = %s, want %s", got, want)
	}
}

func TestRunMigrationsSkipsExistingVersionAndDoesNotDuplicateSchemaMigrations(t *testing.T) {
	t.Skip("integration helper is covered by migrate_upgrade tests")
}

func TestCapPendingByTarget(t *testing.T) {
	mf := func(v string) MigrationFile { return MigrationFile{Version: v, Name: "m" + v} }
	pending := []MigrationFile{mf("0030"), mf("0031"), mf("0032"), mf("0033"), mf("0034")}

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{"empty target applies all", "", "0030,0031,0032,0033,0034"},
		{"target equals last", "0034", "0030,0031,0032,0033,0034"},
		{"target in middle", "0031", "0030,0031"},
		{"target before first", "0029", ""},
		{"target after last", "0099", "0030,0031,0032,0033,0034"},
		{"target exactly first", "0030", "0030"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capPendingByTarget(pending, tt.target)
			versions := make([]string, len(got))
			for i, m := range got {
				versions[i] = m.Version
			}
			if join := strings.Join(versions, ","); join != tt.want {
				t.Fatalf("target=%q got %q want %q", tt.target, join, tt.want)
			}
		})
	}
}

// TestDetectChecksumDriftHasNoVersionWhitelist 确保 migrate.go 中不存在任何版本号
// 硬编码白名单（如 f.Version == "0032"），所有版本 checksum 漂移一视同仁报错。
// 扫描源码断言，防止未来重新引入绕过。
func TestDetectChecksumDriftHasNoVersionWhitelist(t *testing.T) {
	src, err := os.ReadFile("migrate.go")
	if err != nil {
		t.Fatalf("read migrate.go: %v", err)
	}
	// 匹配 .Version == "数字 形式的版本号白名单比较
	versionWhitelist := regexp.MustCompile(`\.Version\s*==\s*"\d`)
	if m := versionWhitelist.FindString(string(src)); m != "" {
		t.Fatalf("migrate.go contains version-specific whitelist in checksum drift detection: %q", m)
	}
}

// TestMigrationSQLHash_Stability 验证 checksum 计算的确定性与敏感性：
// 相同内容产出相同 hash；内容变化产出不同 hash；首尾空白被 TrimSpace 忽略。
func TestMigrationSQLHash_Stability(t *testing.T) {
	a := []byte("CREATE TABLE foo (id INT);")
	b := []byte("CREATE TABLE foo (id INT);")
	if migrationSQLHash(a) != migrationSQLHash(b) {
		t.Fatal("identical content should produce identical hash")
	}
	c := []byte("CREATE TABLE bar (id INT);")
	if migrationSQLHash(a) == migrationSQLHash(c) {
		t.Fatal("different content should produce different hash")
	}
	padded := []byte("\n  " + string(a) + "  \n")
	if migrationSQLHash(a) != migrationSQLHash(padded) {
		t.Fatal("leading/trailing whitespace should not affect hash (TrimSpace)")
	}
}

// TestDetectChecksumDrift_EmptyAppliedReturnsNil 验证无已应用版本时不执行漂移检测。
func TestDetectChecksumDrift_EmptyAppliedReturnsNil(t *testing.T) {
	files := []MigrationFile{{Version: "0032", Name: "test", Path: "nonexistent"}}
	applied := map[string]string{}
	if err := detectMigrationChecksumDrift(context.Background(), nil, files, applied); err != nil {
		t.Fatalf("empty applied should return nil, got: %v", err)
	}
}

// TestDetectChecksumDrift_MismatchReturnsError 验证已应用版本的 checksum 与当前文件 hash
// 不一致时，detectMigrationChecksumDrift 返回漂移错误。这是 checksum 漂移检测的核心断言。
func TestDetectChecksumDrift_MismatchReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0099_test.up.sql")
	content := []byte("CREATE TABLE drift_test (id INT);")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0099", Name: "test", Path: path}}
	// 故意使用与文件 hash 不同的 checksum 模拟漂移。
	applied := map[string]string{"0099": " stale-hash-that-does-not-match "}
	err := detectMigrationChecksumDrift(context.Background(), nil, files, applied)
	if err == nil {
		t.Fatal("expected checksum drift error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum drift detected") {
		t.Fatalf("expected 'checksum drift detected' in error, got: %v", err)
	}
}

// TestDetectChecksumDrift_MatchReturnsNil 验证已应用版本的 checksum 与当前文件 hash
// 一致时，detectMigrationChecksumDrift 返回 nil（无漂移）。
func TestDetectChecksumDrift_MatchReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0098_match.up.sql")
	content := []byte("CREATE TABLE match_test (id INT);")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0098", Name: "match", Path: path}}
	// 使用与文件内容一致的 hash，不应报漂移。
	applied := map[string]string{"0098": migrationSQLHash(content)}
	if err := detectMigrationChecksumDrift(context.Background(), nil, files, applied); err != nil {
		t.Fatalf("matching checksum should return nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// computeChecksumDrifts 单元测试（不需要 PG）
// ---------------------------------------------------------------------------

// TestComputeChecksumDrifts_OldVersionApplied 验证"旧 0032 已应用"场景：
// 旧版本迁移已入库（checksum 不同），当前文件是新版内容 -> 应报告漂移。
func TestComputeChecksumDrifts_OldVersionApplied(t *testing.T) {
	dir := t.TempDir()
	content := []byte("CREATE TABLE new_schema (id INT);")
	path := filepath.Join(dir, "0032_rename.up.sql")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0032", Name: "rename", Path: path}}
	applied := map[string]string{"0032": "old-checksum-from-previous-release"}
	drifts := computeChecksumDrifts(files, applied)
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].Version != "0032" {
		t.Errorf("expected version 0032, got %s", drifts[0].Version)
	}
	if drifts[0].Stored != "old-checksum-from-previous-release" {
		t.Errorf("expected stored 'old-checksum-from-previous-release', got %s", drifts[0].Stored)
	}
	if drifts[0].Current != migrationSQLHash(content) {
		t.Errorf("expected current to match file hash, got %s", drifts[0].Current)
	}
}

// TestComputeChecksumDrifts_ChecksumMismatch 验证"旧 checksum"场景：
// 已存储的 checksum 与当前文件 hash 不一致 -> 应报告漂移。
func TestComputeChecksumDrifts_ChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	content := []byte("CREATE TABLE mismatch_test (id INT);")
	path := filepath.Join(dir, "0050_mismatch.up.sql")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0050", Name: "mismatch", Path: path}}
	applied := map[string]string{"0050": "definitely-not-the-right-hash"}
	drifts := computeChecksumDrifts(files, applied)
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].Version != "0050" {
		t.Errorf("expected version 0050, got %s", drifts[0].Version)
	}
	if drifts[0].Stored != "definitely-not-the-right-hash" {
		t.Errorf("expected stored 'definitely-not-the-right-hash', got %s", drifts[0].Stored)
	}
}

// TestComputeChecksumDrifts_EmptyChecksum 验证"空 checksum"场景：
// 已应用版本但 checksum 为空（旧版迁移 runner 未记录）-> 应报告漂移。
func TestComputeChecksumDrifts_EmptyChecksum(t *testing.T) {
	dir := t.TempDir()
	content := []byte("CREATE TABLE empty_checksum_test (id INT);")
	path := filepath.Join(dir, "0060_empty.up.sql")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0060", Name: "empty", Path: path}}
	applied := map[string]string{"0060": ""}
	drifts := computeChecksumDrifts(files, applied)
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift for empty checksum, got %d", len(drifts))
	}
	if drifts[0].Stored != "" {
		t.Errorf("expected empty stored, got %s", drifts[0].Stored)
	}
	if drifts[0].Current != migrationSQLHash(content) {
		t.Errorf("expected current to match file hash, got %s", drifts[0].Current)
	}
}

// TestComputeChecksumDrifts_FileDrift 验证"文件漂移"场景：
// 迁移已应用（checksum 正确），之后文件内容被修改 -> 应报告漂移。
func TestComputeChecksumDrifts_FileDrift(t *testing.T) {
	dir := t.TempDir()
	originalContent := []byte("CREATE TABLE drift_test (id INT);")
	path := filepath.Join(dir, "0070_drift.up.sql")
	if err := os.WriteFile(path, originalContent, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	originalHash := migrationSQLHash(originalContent)
	// 模拟文件漂移：覆盖为不同内容。
	driftedContent := []byte("CREATE TABLE drift_test (id INT, name TEXT);")
	if err := os.WriteFile(path, driftedContent, 0644); err != nil {
		t.Fatalf("overwrite temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0070", Name: "drift", Path: path}}
	applied := map[string]string{"0070": originalHash}
	drifts := computeChecksumDrifts(files, applied)
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift after file content change, got %d", len(drifts))
	}
	if drifts[0].Stored != originalHash {
		t.Errorf("expected stored to be original hash, got %s", drifts[0].Stored)
	}
	if drifts[0].Current != migrationSQLHash(driftedContent) {
		t.Errorf("expected current to match drifted file hash, got %s", drifts[0].Current)
	}
}

// TestComputeChecksumDrifts_NoDrift 验证 checksum 一致时不报告漂移（对照测试）。
func TestComputeChecksumDrifts_NoDrift(t *testing.T) {
	dir := t.TempDir()
	content := []byte("CREATE TABLE no_drift_test (id INT);")
	path := filepath.Join(dir, "0080_no_drift.up.sql")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp migration: %v", err)
	}
	files := []MigrationFile{{Version: "0080", Name: "no_drift", Path: path}}
	applied := map[string]string{"0080": migrationSQLHash(content)}
	drifts := computeChecksumDrifts(files, applied)
	if len(drifts) != 0 {
		t.Fatalf("expected 0 drifts for matching checksum, got %d: %+v", len(drifts), drifts)
	}
}
