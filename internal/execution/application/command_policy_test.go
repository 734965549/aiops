package application

import "testing"

func TestBuildCommandArgv(t *testing.T) {
	argv, err := BuildCommandArgv("df -h {mount_point}", map[string]any{"mount_point": "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 3 || argv[0] != "df" || argv[2] != "/" {
		t.Fatalf("unexpected argv: %#v", argv)
	}
}

func TestBuildCommandArgvRejectsShellMeta(t *testing.T) {
	_, err := BuildCommandArgv("df -h {mount_point}", map[string]any{"mount_point": "/; rm -rf /"})
	if err == nil {
		t.Fatal("expected shell metachar rejection")
	}
}

func TestRedactOutput(t *testing.T) {
	out, changed := RedactOutput("password=secret token=abc", map[string]any{
		"patterns": []any{"(?i)password=.*", "(?i)token=.*"},
	})
	if !changed {
		t.Fatal("expected redaction")
	}
	if out == "password=secret token=abc" {
		t.Fatalf("content not redacted: %s", out)
	}
}
