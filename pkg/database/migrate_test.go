package database

import (
	"os"
	"path/filepath"
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
