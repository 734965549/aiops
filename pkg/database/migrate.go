package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/734965549/aiops/pkg/logger"
	"go.uber.org/zap"
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

// 平台采用「自实现的最小迁移 runner」，为唯一允许的迁移执行方式（见 ops/migration-contract.md）：
//
//   - 禁止与 golang-migrate 等外部工具混用同一数据库实例；
//   - 通过 schema_migrations 表跟踪已应用版本，保证幂等与可重启；
//   - 仅处理 *.up.sql；回滚由人工执行对应 *.down.sql；
//   - 文件名要求 <version>_<name>.up.sql，<version> 4 位数字递增。
//
// 显式执行入口：make migrate / go run ./cmd/migrate。

const schemaMigrationsTable = "schema_migrations"

// 文件名规则：0001_init_identity.up.sql -> version=0001, name=init_identity。
var migrationFilePattern = regexp.MustCompile(`^(\d{4,})_([a-zA-Z0-9_\-]+)\.up\.sql$`)

// MigrationFile 描述一个迁移条目。
type MigrationFile struct {
	Version string // 4 位以上的版本号字符串，保持原始零填充以便字典序排序
	Name    string // 业务可读的名称
	Path    string // 绝对或相对的 SQL 文件路径
}

// MigrateOptions 控制 RunMigrations 的行为。
type MigrateOptions struct {
	// Dir 是 *.up.sql 所在目录。空则默认 ./migrations。
	Dir string
}

// MigrationStatus 描述迁移就绪情况，供 readiness 检查复用。
type MigrationStatus struct {
	Dir            string
	LatestVersion  string
	AppliedVersion string
	PendingCount   int
	UpToDate       bool
}

// RunMigrations 顺序执行 Dir 下尚未应用的 *.up.sql。
//
// 流程：
//  1. 确保 schema_migrations 表存在；
//  2. 扫描 Dir 下符合命名规则的 *.up.sql；
//  3. 与 schema_migrations 中已应用版本求差集；
//  4. 按版本号顺序、每个迁移单独事务地执行；
//  5. 每个迁移成功后写入 schema_migrations。
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
		logger.From(ctx).Info("no migration files found", zap.String("dir", dir))
		return nil
	}

	applied, err := loadAppliedVersions(ctx, db)
	if err != nil {
		return fmt.Errorf("load applied versions: %w", err)
	}

	pending := make([]MigrationFile, 0)
	for _, f := range files {
		if !applied[f.Version] {
			pending = append(pending, f)
		}
	}
	if len(pending) == 0 {
		logger.From(ctx).Info("database migrations up to date",
			zap.Int("applied", len(applied)),
			zap.Int("files", len(files)),
		)
		return nil
	}

	logger.From(ctx).Info("applying database migrations",
		zap.Int("pending", len(pending)),
		zap.String("dir", dir),
	)
	for _, m := range pending {
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("apply %s: %w", m.Version, err)
		}
		logger.From(ctx).Info("migration applied",
			zap.String("version", m.Version),
			zap.String("name", m.Name),
		)
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, db *gorm.DB) error {
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
        version    VARCHAR(64) PRIMARY KEY,
        name       VARCHAR(255) NOT NULL,
        applied_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
    )`, schemaMigrationsTable)
	return db.WithContext(ctx).Exec(stmt).Error
}

func loadAppliedVersions(ctx context.Context, db *gorm.DB) (map[string]bool, error) {
	type row struct{ Version string }
	var rows []row
	if err := db.WithContext(ctx).
		Raw(fmt.Sprintf("SELECT version FROM %s", schemaMigrationsTable)).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	applied := make(map[string]bool, len(rows))
	for _, r := range rows {
		applied[r.Version] = true
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
		files = append(files, MigrationFile{
			Version: matches[1],
			Name:    matches[2],
			Path:    filepath.Join(dir, name),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}

func applyMigration(ctx context.Context, db *gorm.DB, m MigrationFile) error {
	content, err := os.ReadFile(m.Path)
	if err != nil {
		return fmt.Errorf("read sql: %w", err)
	}
	sqlText := strings.TrimSpace(string(content))
	if sqlText == "" {
		return fmt.Errorf("empty migration file: %s", m.Path)
	}

	stmts := splitSQLStatements(sqlText)
	if len(stmts) == 0 {
		return fmt.Errorf("no executable statements in migration file: %s", m.Path)
	}

	// 单个迁移文件作为一个事务执行；PostgreSQL 支持事务内的 DDL，便于失败回滚。
	// 按语句逐条执行：*.up.sql 常含多条 DDL/DML，RDS/PgBouncer 不接受单条 prepared
	// statement 内嵌多命令（SQLSTATE 42601）。
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec := tx.Session(&gorm.Session{PrepareStmt: false})
		for _, stmt := range stmts {
			if err := exec.Exec(stmt).Error; err != nil {
				return err
			}
		}
		return tx.Exec(
			fmt.Sprintf("INSERT INTO %s (version, name, applied_at) VALUES (?, ?, ?)", schemaMigrationsTable),
			m.Version, m.Name, time.Now(),
		).Error
	})
}

// splitSQLStatements 将迁移 SQL 按分号拆成可独立执行的语句（忽略整行 -- 注释）。
//
// 约定与限制（与仓库内 *.up.sql 风格一致）：
//   - 仅剥离「整行」-- 注释；行内 -- 注释不在此处理；
//   - 分号仅在单引号字符串外作为语句分隔符；支持 PostgreSQL ” 转义；
//   - 不支持美元引用（$tag$...$tag$）、函数/触发器体内的多语句块——若未来需要，
//     应改用专用 splitter 或将该逻辑拆成独立迁移文件。
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

// ReadMigrationStatus 读取迁移就绪状态。
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
		if applied[f.Version] {
			status.AppliedVersion = f.Version
			continue
		}
		status.PendingCount++
	}
	status.UpToDate = status.PendingCount == 0
	return status, nil
}
