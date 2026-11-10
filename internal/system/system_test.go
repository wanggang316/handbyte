package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wanggang316/handbyte/catalog"
)

func TestMeasureCountsOnlyOldMatchingRegularFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour)
	write := func(rel string, age time.Time) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, 5000), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, age, age); err != nil {
			t.Fatal(err)
		}
	}
	write("App_1.ips", old)
	write("Retired/App_2.diag", old)
	write("App_3.ips", now)
	write("notes.txt", old)
	if err := os.Symlink(filepath.Join(root, "App_1.ips"), filepath.Join(root, "link.ips")); err != nil {
		t.Fatal(err)
	}
	rule := catalog.LoadSystem().Rules[0]
	m := MeasureRule(rule, root, now)
	if m.Files != 2 || m.Bytes < 10000 || m.Partial {
		t.Fatalf("measure: %+v", m)
	}
}

func TestCommandIsFixedAndRunsAsPlannedOnAFolder(t *testing.T) {
	rule := catalog.SystemRule{ID: "x", Root: "/Library/Logs/DiagnosticReports", Names: []string{"*.ips", "*.diag"}, MinAgeDays: 30}
	got := strings.Join(Command(rule), " ")
	want := "/usr/bin/find /Library/Logs/DiagnosticReports -xdev -type f ( -name *.ips -o -name *.diag ) -mtime +30 -print -delete"
	if got != want {
		t.Fatalf("command:\n%s\n%s", got, want)
	}
	if s := ShellCommand(rule); !strings.HasPrefix(s, "'/usr/bin/find' '/Library/Logs/DiagnosticReports' '-xdev'") || !strings.Contains(s, "'*.ips'") {
		t.Fatalf("shell command: %s", s)
	}
	// The same command on a temporary folder removes only what Measure counts.
	root := t.TempDir()
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, name := range []string{"a.ips", "b.diag", "keep.txt"} {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "new.ips"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rule.Root = root
	if m := MeasureRule(rule, "", time.Now()); m.Files != 2 {
		t.Fatalf("measure: %+v", m)
	}
	args := Command(rule)
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "\n") != 2 {
		t.Fatalf("removed: %q", out)
	}
	left, _ := os.ReadDir(root)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "keep.txt,new.ips" {
		t.Fatalf("left: %v", names)
	}
}
