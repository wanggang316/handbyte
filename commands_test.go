package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

	if r := run("history", "--json"); strings.TrimSpace(r.stdout) != "[]" {
		t.Fatalf("history --json: %+v", r)
	}
	if r := run("history"); r.code != 0 || !strings.Contains(r.stdout, "No actions recorded") {
		t.Fatalf("history: %+v", r)
	}
	if r := run("restore", "missing-id"); r.code != 1 || r.stderr == "" {
		t.Fatalf("restore of an unknown ID must fail: %+v", r)
	}

	r = run("analyze", root, "--json")
	if err := json.Unmarshal([]byte(r.stdout), &scan); err != nil || r.code != 0 || len(scan.Items) != 1 {
		t.Fatalf("analyze --json: %v %+v", err, r)
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
