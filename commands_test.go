package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wanggang316/handbyte/internal/scanner"
)

type cliResult struct {
	stdout, stderr string
	code           int
}

// buildCLI returns a function that runs the built binary with a private home.
func buildCLI(t *testing.T) (run func(args ...string) cliResult, home string) {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "handbyte")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	home = t.TempDir()
	return func(args ...string) cliResult {
		t.Helper()
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, args...)
		cmd.Env = append(os.Environ(), "HOME="+home)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("%v: %v", args, err)
			}
			code = exit.ExitCode()
		}
		return cliResult{stdout.String(), stderr.String(), code}
	}, home
}

func rustProject(t *testing.T, home string) (root, target string) {
	t.Helper()
	root = filepath.Join(home, "work")
	target = filepath.Join(root, "example", "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "example", "Cargo.toml"), []byte("[package]"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, target
}

func TestCommandsCoverTheOldFlags(t *testing.T) {
	run, home := buildCLI(t)
	root, target := rustProject(t, home)

	if r := run("config", "roots", root); r.code != 0 || !strings.Contains(r.stdout, "Project roots saved") {
		t.Fatalf("config roots: %+v", r)
	}
	var scan struct {
		Schema int            `json:"schema_version"`
		Status string         `json:"scan_status"`
		Items  []scanner.Item `json:"items"`
	}
	r := run("clean", "--json", "--scope", "projects", "--min", "0")
	if err := json.Unmarshal([]byte(r.stdout), &scan); err != nil || r.code != 0 {
		t.Fatalf("clean --json: %v %+v", err, r)
	}
	if scan.Schema != 2 || scan.Status != "complete" || len(scan.Items) != 1 || scan.Items[0].Owner != "Rust" {
		t.Fatalf("unexpected scan: %+v", scan)
	}

	if r := run("config", "protect", target); r.code != 0 {
		t.Fatalf("config protect: %+v", r)
	}
	if r := run("config"); !strings.Contains(r.stdout, "example/target") || !strings.Contains(r.stdout, "work") {
		t.Fatalf("config must show the roots and the protected paths: %+v", r)
	}
	r = run("clean", "--json", "--scope", "projects", "--min", "0")
	if err := json.Unmarshal([]byte(r.stdout), &scan); err != nil || scan.Items[0].Eligibility != scanner.Protected {
		t.Fatalf("protection not applied: %v %+v", err, r)
	}
	if r := run("config", "unprotect", target); r.code != 0 {
		t.Fatalf("config unprotect: %+v", r)
	}
	if r := run("config"); strings.Contains(r.stdout, "example/target") {
		t.Fatalf("the protection must be gone: %+v", r)
	}
	if r := run("config", "mouse", "off"); r.code != 0 {
		t.Fatalf("config mouse off: %+v", r)
	}
	if r := run("config"); !strings.Contains(r.stdout, "Mouse support\n  off") {
		t.Fatalf("config must show the mouse setting: %+v", r)
	}
	if r := run("config", "mouse", "maybe"); r.code != 2 {
		t.Fatalf("a bad mouse value: %+v", r)
	}

	if r := run("history", "--json"); strings.TrimSpace(r.stdout) != "[]" {
		t.Fatalf("history --json: %+v", r)
	}
	if r := run("history"); r.code != 0 || !strings.Contains(r.stdout, "No actions recorded") {
		t.Fatalf("history: %+v", r)
	}
	if r := run("restore", "missing-id"); r.code != 1 || r.stderr == "" {
		t.Fatalf("restore of an unknown ID must fail: %+v", r)
	}

}

func TestAnalyzeJSONReportsTheTree(t *testing.T) {
	run, home := buildCLI(t)
	root, target := rustProject(t, home)
	if err := os.WriteFile(filepath.Join(target, "binary"), make([]byte, 20000), 0o600); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Schema    int    `json:"schema_version"`
		Path      string `json:"path"`
		Status    string `json:"status"`
		Allocated int64  `json:"allocated_bytes"`
		Logical   int64  `json:"logical_bytes"`
		Files     int64  `json:"file_count"`
		Children  []struct {
			Name      string `json:"name"`
			Dir       bool   `json:"is_dir"`
			Allocated int64  `json:"allocated_bytes"`
			Files     int64  `json:"file_count"`
		} `json:"children"`
		Largest []struct {
			Path      string `json:"path"`
			Allocated int64  `json:"allocated_bytes"`
		} `json:"largest_files"`
	}
	r := run("analyze", root, "--json")
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil || r.code != 0 {
		t.Fatalf("analyze --json: %v %+v", err, r)
	}
	if report.Schema != 1 || report.Status != "complete" || report.Files != 2 || report.Logical != 20009 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Children) != 1 || report.Children[0].Name != "example" || !report.Children[0].Dir || report.Children[0].Files != 2 {
		t.Fatalf("children: %+v", report.Children)
	}
	if len(report.Largest) != 2 || !strings.HasSuffix(report.Largest[0].Path, "example/target/binary") {
		t.Fatalf("largest files: %+v", report.Largest)
	}
	if r := run("analyze", filepath.Join(home, "missing"), "--json"); r.code != 1 || r.stderr == "" {
		t.Fatalf("a missing path must fail: %+v", r)
	}
}

func TestHelpVersionAndUsageErrors(t *testing.T) {
	run, _ := buildCLI(t)
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		r := run(args...)
		if r.code != 0 || !strings.Contains(r.stdout, "handbyte clean") || !strings.Contains(r.stdout, "handbyte analyze") {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	if r := run("version"); r.code != 0 || strings.TrimSpace(r.stdout) == "" {
		t.Fatalf("version: %+v", r)
	}
	if r := run("frobnicate"); r.code != 2 || !strings.Contains(r.stderr, "unknown command") || !strings.Contains(r.stderr, "handbyte help") {
		t.Fatalf("unknown command: %+v", r)
	}
	if r := run("clean", "--no-such-flag"); r.code != 2 {
		t.Fatalf("unknown flag: %+v", r)
	}
	if r := run("restore"); r.code != 2 {
		t.Fatalf("restore without an ID: %+v", r)
	}
	// Without a terminal there is no home screen; the usage is the answer.
	if r := run(); r.code != 0 || !strings.Contains(r.stdout, "handbyte clean") {
		t.Fatalf("no arguments without a terminal: %+v", r)
	}
}

func TestOldFlagsStillWorkAndNameTheNewCommand(t *testing.T) {
	run, _ := buildCLI(t)
	r := run("--history")
	if r.code != 0 || strings.TrimSpace(r.stdout) != "[]" {
		t.Fatalf("--history: %+v", r)
	}
	if !strings.Contains(r.stderr, "handbyte history --json") {
		t.Fatalf("--history must name the new command on stderr: %+v", r)
	}
	r = run("--scan-only", "--no-projects", "--min", "0")
	if r.code != 0 || !strings.Contains(r.stderr, "handbyte clean --json") || !strings.HasPrefix(strings.TrimSpace(r.stdout), "{") {
		t.Fatalf("--scan-only: %+v", r)
	}
}

func TestCleanWithoutATerminalPrintsTheSummaryAndCleansWithYes(t *testing.T) {
	run, home := buildCLI(t)
	cache := filepath.Join(home, ".terraform.d/plugin-cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "provider"), make([]byte, 3<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	for _, p := range []string{filepath.Join(cache, "provider"), cache} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	r := run("clean")
	if r.code != 0 || !strings.Contains(r.stdout, "Safe to clean: 3.0 MiB in 1 item") || !strings.Contains(r.stdout, "Nothing was changed") {
		t.Fatalf("clean: %+v", r)
	}
	if r := run("clean", "--yes", "--dry-run"); !strings.Contains(r.stdout, "Preview: 3.0 MiB") {
		t.Fatalf("clean --yes --dry-run: %+v", r)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatal("a preview must not change the cache")
	}
	if r := run("clean", "--yes"); !strings.Contains(r.stdout, "Removed 3.0 MiB") {
		t.Fatalf("clean --yes: %+v", r)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatal("the rebuildable cache must be removed directly")
	}
	if r := run("history", "--json"); !strings.Contains(r.stdout, `"Status":"removed"`) {
		t.Fatalf("the removal must be journaled: %+v", r)
	}
	if r := run("clean", "--rm", "--trash"); r.code != 2 {
		t.Fatalf("--rm with --trash: %+v", r)
	}
}

func TestUninstallWithoutATerminalShowsThePlanAndPreviews(t *testing.T) {
	run, home := buildCLI(t)
	bundle := filepath.Join(home, "Applications", "Fake Notes.app", "Contents")
	if err := os.MkdirAll(filepath.Join(bundle, "MacOS"), 0o700); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>net.handbytetest.FakeNotes</string><key>CFBundleExecutable</key><string>run</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(bundle, "Info.plist"), []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"Library/Caches/net.handbytetest.FakeNotes/c", "Library/Application Support/net.handbytetest.FakeNotes/db"} {
		if err := os.MkdirAll(filepath.Join(home, filepath.Dir(p)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, p), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r := run("uninstall", "Fake Notes")
	if r.code != 0 || !strings.Contains(r.stdout, "cache     high     move") || !strings.Contains(r.stdout, "data      high     keep") || !strings.Contains(r.stdout, "Nothing was changed") {
		t.Fatalf("plan: %+v", r)
	}
	r = run("uninstall", "net.handbytetest.FakeNotes", "--with-data", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "data      high     move") || !strings.Contains(r.stdout, "Preview: 3 items would move") {
		t.Fatalf("dry run: %+v", r)
	}
	if _, err := os.Stat(bundle); err != nil {
		t.Fatal("a preview must not move the app")
	}
	if r := run("uninstall", "No Such App"); r.code != 1 || !strings.Contains(r.stderr, "no app named") {
		t.Fatalf("unknown app: %+v", r)
	}
	if r := run("uninstall"); r.code != 2 {
		t.Fatalf("no app: %+v", r)
	}
}
