package main

import (
	"encoding/json"
	"github.com/wanggang316/handbyte/internal/scanner"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSizeRejectsNegativeAndOverflow(t *testing.T) {
	for _, value := range []string{"-1", "9223372036854775807G"} {
		if _, err := parseSize(value); err == nil {
			t.Errorf("accepted invalid size %s", value)
		}
	}
	if got, err := parseSize("200M"); err != nil || got != 200*1024*1024 {
		t.Fatalf("valid size: %d %v", got, err)
	}
}

func TestCLISettingsAndScanJSON(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "handbyte")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	home := t.TempDir()
	root := filepath.Join(home, "work")
	project := filepath.Join(root, "example")
	if err := os.MkdirAll(filepath.Join(project, "target"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "Cargo.toml"), []byte("[package]"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %s %v", args, out, err)
		}
		return out
	}
	run("--roots", root, "--save-roots")
	var response struct {
		Schema int            `json:"schema_version"`
		Status string         `json:"scan_status"`
		Items  []scanner.Item `json:"items"`
	}
	if err := json.Unmarshal(run("--scan-only", "--scope", "projects", "--min", "0"), &response); err != nil {
		t.Fatal(err)
	}
	if response.Schema != 2 || response.Status != "complete" || len(response.Items) != 1 || response.Items[0].Owner != "Rust" {
		t.Fatalf("unexpected scan: %+v", response)
	}
	run("--protect", filepath.Join(project, "target"))
	if err := json.Unmarshal(run("--scan-only", "--scope", "projects", "--min", "0"), &response); err != nil {
		t.Fatal(err)
	}
	if response.Items[0].Eligibility != scanner.Protected || !strings.Contains(response.Items[0].Reason, "exclusion") {
		t.Fatal("CLI protection not applied")
	}
	run("--unprotect", filepath.Join(project, "target"))
	if got := strings.TrimSpace(string(run("--history"))); got != "[]" {
		t.Fatalf("unexpected history: %s", got)
	}
}
