package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/734965549/aiops/pkg/logger"
	"gorm.io/gorm"
)

// ResolveMigrationDir 选择第一个存在的 migrations 目录：
//   - 二进制同级目录的 ./migrations（容器内典型布局）；
//   - 工作目录的 ./migrations（go run 的本地开发场景）。
//
// 找不到时返回默认 ./migrations，让 RunMigrations / ReadMigrationStatus 暴露明确的报错。
func ResolveMigrationDir() string {
	candidates := []string{"./migrations"}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{
			fmt.Sprintf("%s/migrations", trimDirSuffix(exe)),
		}, candidates...)
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, p := range candidates {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return "./migrations"
}

func trimDirSuffix(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

const schemaMigrationsTable = "schema_migrations"

var migrationFilePattern = regexp.MustCompile(`^(\d{4,})_([a-zA-Z0-9_\-]+)\.up\.sql$`)

type MigrationFile struct {
	Version string
	Name    string
	Path    string
}

type MigrateOptions struct {
	Dir           string
	TargetVersion string
}

type MigrationStatus struct {
	Dir            string
	LatestVersion  string
	AppliedVersion string
	PendingCount   int
	UpToDate       bool
	PendingHash    string
	ChecksumDrifts []ChecksumDrift
}

// ChecksumDrift 描述一个已应用迁移文件的 checksum 与当前文件内容不一致（或为空）。
// 用于 /readyz 只读报告漂移，不做 backfill，不返回 error。
type ChecksumDrift struct {
	Version string
	Name    string
	Stored  string
	Current string
}

func RunMigrations(ctx context.Context, db *gorm.DB, opt MigrateOptions) error {
	if db == nil {
		return fmt.Errorf("nil *gorm.DB")
	}
	dir := opt.Dir
	if dir == "" {
		dir = ResolveMigrationDir()
	}
	if err := ensureMigrationTable(ctx, db); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	files, err := scanMigrationFiles(dir)
	if err != nil {
		return fmt.Errorf("scan migration dir %q: %w", dir, err)
	}
	if len(files) == 0 {
		logger.From(ctx).Info("no migration files found", logger.String("dir", dir))
		return nil
	}
	applied, err := loadAppliedVersions(ctx, db)
	if err != nil {
		return fmt.Errorf("load applied versions: %w", err)
	}
	if err := detectMigrationChecksumDrift(ctx, db, files, applied); err != nil {
		return err
	}
	pending := make([]MigrationFile, 0)
	for _, f := range files {
		if _, ok := applied[f.Version]; !ok {
			pending = append(pending, f)
		}
	}
	pending = capPendingByTarget(pending, opt.TargetVersion)
	if len(pending) == 0 {
		logger.From(ctx).Info("database migrations up to date", logger.Int("applied", len(applied)), logger.Int("files", len(files)))
		return nil
	}
	logger.From(ctx).Info("applying database migrations", logger.Int("pending", len(pending)), logger.String("dir", dir))
	for _, m := range pending {
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("apply %s: %w", m.Version, err)
		}
		logger.From(ctx).Info("migration applied", logger.String("version", m.Version), logger.String("name", m.Name))
	}
	return nil
}

func capPendingByTarget(pending []MigrationFile, target string) []MigrationFile {
	if target == "" {
		return pending
	}
	// 版本号正则允许 4+ 位数字，字符串比较在 10000 vs 9999 时字典序错误。
	// 优先按整数比较；target 非数字时回退字符串比较。
	targetInt, targetIsInt := strconv.Atoi(target)
	cut := len(pending)
	for i, m := range pending {
		var exceeded bool
		if targetIsInt == nil {
			if v, err := strconv.Atoi(m.Version); err == nil {
				exceeded = v > targetInt
			} else {
				exceeded = m.Version > target
			}
		} else {
			exceeded = m.Version > target
		}
		if exceeded {
			cut = i
			break
		}
	}
	return pending[:cut]
}

func detectMigrationChecksumDrift(ctx context.Context, db *gorm.DB, files []MigrationFile, applied map[string]string) error {
	if len(files) == 0 || len(applied) == 0 {
		return nil
	}
	for _, f := range files {
		stored, ok := applied[f.Version]
		if !ok {
			continue
		}
		content, err := os.ReadFile(f.Path)
		if err != nil {
			return fmt.Errorf("read migration %s for checksum: %w", f.Version, err)
		}
		current := migrationSQLHash(content)
		if stored == "" {
			if err := backfillMigrationChecksum(ctx, db, f.Version, current); err != nil {
				return err
			}
			continue
		}
		if stored != current {
			return fmt.Errorf("migration %s checksum drift detected: database=%s file=%s", f.Version, stored, current)
		}
	}
	return nil
}

// computeChecksumDrifts 对已应用迁移做只读 checksum 比对。
// 与 detectMigrationChecksumDrift（RunMigrations 使用）不同，它不做 backfill、不返回 error，
// 仅报告漂移列表，供 /readyz 展示。空 checksum 也算漂移，因为意味着该版本缺少可校验的基线。
func computeChecksumDrifts(files []MigrationFile, applied map[string]string) []ChecksumDrift {
	if len(files) == 0 || len(applied) == 0 {
		return nil
	}
	var drifts []ChecksumDrift
	for _, f := range files {
		stored, ok := applied[f.Version]
		if !ok {
			continue
		}
		content, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		current := migrationSQLHash(content)
		if stored == "" || stored != current {
			drifts = append(drifts, ChecksumDrift{
				Version: f.Version,
				Name:    f.Name,
				Stored:  stored,
				Current: current,
			})
		}
	}
	return drifts
}

func backfillMigrationChecksum(ctx context.Context, db *gorm.DB, version, checksum string) error {
	return db.WithContext(ctx).Exec(fmt.Sprintf("UPDATE %s SET checksum = ? WHERE version = ? AND (checksum IS NULL OR checksum = '')", schemaMigrationsTable), checksum, version).Error
}

func ensureMigrationTable(ctx context.Context, db *gorm.DB) error {
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
        version    VARCHAR(64) PRIMARY KEY,
        name       VARCHAR(255) NOT NULL,
        checksum   VARCHAR(64),
        applied_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
    )`, schemaMigrationsTable)
	if err := db.WithContext(ctx).Exec(stmt).Error; err != nil {
		return err
	}
	return db.WithContext(ctx).Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS checksum VARCHAR(64)`, schemaMigrationsTable)).Error
}

func loadAppliedVersions(ctx context.Context, db *gorm.DB) (map[string]string, error) {
	type row struct{ Version, Checksum string }
	var rows []row
	if err := db.WithContext(ctx).Raw(fmt.Sprintf("SELECT version, COALESCE(checksum, '') AS checksum FROM %s", schemaMigrationsTable)).Scan(&rows).Error; err != nil {
		return nil, err
	}
	applied := make(map[string]string, len(rows))
	for _, r := range rows {
		applied[r.Version] = r.Checksum
	}
	return applied, nil
}

func scanMigrationFiles(dir string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]MigrationFile, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		matches := migrationFilePattern.FindStringSubmatch(name)
		if len(matches) != 3 {
			continue
		}
		files = append(files, MigrationFile{Version: matches[1], Name: matches[2], Path: filepath.Join(dir, name)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}

func applyMigration(ctx context.Context, db *gorm.DB, m MigrationFile) error {
	content, err := os.ReadFile(m.Path)
	if err != nil {
		return fmt.Errorf("read sql: %w", err)
	}
	checksum := migrationSQLHash(content)
	sqlText := strings.TrimSpace(string(content))
	if sqlText == "" {
		return fmt.Errorf("empty migration file: %s", m.Path)
	}
	stmts := splitSQLStatements(sqlText)
	if len(stmts) == 0 {
		return fmt.Errorf("no executable statements in migration file: %s", m.Path)
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec := tx.Session(&gorm.Session{PrepareStmt: false})
		for _, stmt := range stmts {
			if err := exec.Exec(stmt).Error; err != nil {
				return err
			}
		}
		return tx.Exec(fmt.Sprintf("INSERT INTO %s (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)", schemaMigrationsTable), m.Version, m.Name, checksum, time.Now()).Error
	})
}

func migrationSQLHash(content []byte) string {
	sum := sha256.Sum256(bytes.TrimSpace(content))
	return hex.EncodeToString(sum[:])
}

func splitSQLStatements(sqlText string) []string {
	var cleaned strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteByte('\n')
	}
	return splitOnSemicolonOutsideSingleQuotes(cleaned.String())
}

func splitOnSemicolonOutsideSingleQuotes(s string) []string {
	out := make([]string, 0, 8)
	var buf strings.Builder
	inSingle := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inSingle:
			inSingle = true
			buf.WriteByte(c)
		case c == '\'' && inSingle:
			if i+1 < len(s) && s[i+1] == '\'' {
				buf.WriteByte(c)
				buf.WriteByte(c)
				i++
				continue
			}
			inSingle = false
			buf.WriteByte(c)
		case c == ';' && !inSingle:
			if stmt := strings.TrimSpace(buf.String()); stmt != "" {
				out = append(out, stmt)
			}
			buf.Reset()
		default:
			buf.WriteByte(c)
		}
	}
	if stmt := strings.TrimSpace(buf.String()); stmt != "" {
		out = append(out, stmt)
	}
	return out
}

func ReadMigrationStatus(ctx context.Context, db *gorm.DB, opt MigrateOptions) (MigrationStatus, error) {
	status := MigrationStatus{Dir: opt.Dir}
	if status.Dir == "" {
		status.Dir = ResolveMigrationDir()
	}
	files, err := scanMigrationFiles(status.Dir)
	if err != nil {
		return status, err
	}
	if len(files) == 0 {
		status.UpToDate = true
		return status, nil
	}
	status.LatestVersion = files[len(files)-1].Version
	applied, err := loadAppliedVersions(ctx, db)
	if err != nil {
		return status, err
	}
	for _, f := range files {
		if _, ok := applied[f.Version]; ok {
			status.AppliedVersion = f.Version
			continue
		}
		status.PendingCount++
	}
	status.UpToDate = status.PendingCount == 0
	status.ChecksumDrifts = computeChecksumDrifts(files, applied)
	return status, nil
}
