package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wanggang316/handbyte/internal/scanner"
)

func appFixture(t *testing.T) (home, apps string, plan AppPlan, opts AppOptions) {
	t.Helper()
	home, opts0 := collectHome(t)
	apps = filepath.Join(home, "Applications")
	bundle := filepath.Join(apps, "Notes.app")
	makeFile(t, filepath.Join(bundle, "Contents/Info.plist"), 10)
	cache := filepath.Join(home, "Library/Caches/com.example.Notes")
	prefs := filepath.Join(home, "Library/Preferences/com.example.Notes.plist")
	makeFile(t, filepath.Join(cache, "c"), 100)
	makeFile(t, prefs, 10)
	id, err := scanner.EntryIdentity(bundle)
	if err != nil {
		t.Fatal(err)
	}
	plan = AppPlan{Name: "Notes", BundleID: "com.example.Notes", Bundle: bundle, BundleIdentity: id}
	for _, p := range []string{cache, prefs} {
		f, err := NewAppFile(p, 5)
		if err != nil {
			t.Fatal(err)
		}
		plan.Files = append(plan.Files, f)
	}
	opts = AppOptions{Trash: opts0.Trash, Roots: []string{apps}, Running: func(string) bool { return false }}
	return home, apps, plan, opts
}

func TestUninstallMovesTheAppAndItsFilesAsOneGroup(t *testing.T) {
	home, _, plan, opts := appFixture(t)
	group, results, err := Uninstall(context.Background(), plan, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || !strings.HasPrefix(group, "app-") {
		t.Fatalf("results: %+v", results)
	}
	for _, r := range results {
		if r.Status != "trashed" || r.Group != group {
			t.Fatalf("result: %+v", r)
		}
		if _, err := os.Lstat(r.Path); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", r.Path)
		}
	}
	history, err := History()
	if err != nil || len(history) != 3 || history[0].Group != group {
		t.Fatalf("journal: %+v %v", history, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".Trash", "Notes.app")); err != nil {
		t.Fatal("the app must be in the Trash")
	}
}

func TestUninstallRefusals(t *testing.T) {
	_, _, plan, opts := appFixture(t)
	cases := map[string]func(p *AppPlan, o *AppOptions){
		"system":   func(p *AppPlan, o *AppOptions) { p.System = true },
		"homebrew": func(p *AppPlan, o *AppOptions) { p.Cask = "notes" },
		"running":  func(p *AppPlan, o *AppOptions) { o.Running = func(string) bool { return true } },
		"root":     func(p *AppPlan, o *AppOptions) { o.Roots = []string{"/Applications"} },
		"changed":  func(p *AppPlan, o *AppOptions) { p.BundleIdentity.Inode++ },
	}
	for name, change := range cases {
		p, o := plan, opts
		change(&p, &o)
		if _, results, err := Uninstall(context.Background(), p, o); err == nil || len(results) != 0 {
			t.Errorf("%s: accepted (%v, %d results)", name, err, len(results))
		}
	}
	if _, err := os.Stat(plan.Bundle); err != nil {
		t.Fatal("a refused uninstall must not move the app")
	}
}

func TestResetKeepsTheAppAndDryRunChangesNothing(t *testing.T) {
	_, _, plan, opts := appFixture(t)
	dry := opts
	dry.DryRun = true
	if _, results, err := Uninstall(context.Background(), plan, dry); err != nil || results[0].Status != "previewed" {
		t.Fatalf("dry run: %+v %v", results, err)
	}
	if _, err := os.Stat(plan.Bundle); err != nil {
		t.Fatal("a dry run must not move the app")
	}
	reset := plan
	reset.Bundle = ""
	_, results, err := Uninstall(context.Background(), reset, opts)
	if err != nil || len(results) != 2 {
		t.Fatalf("reset: %+v %v", results, err)
	}
	if _, err := os.Stat(plan.Bundle); err != nil {
		t.Fatal("a reset must keep the app")
	}
}

func TestAppFilesMustBeInTheHomeFolder(t *testing.T) {
	home, _, _, _ := appFixture(t)
	outside, _ := scanner.Physical(t.TempDir())
	makeFile(t, filepath.Join(outside, "x"), 1)
	for _, p := range []string{home, filepath.Join(home, "Library"), filepath.Join(outside, "x")} {
		if _, err := NewAppFile(p, 1); err == nil {
			t.Errorf("%s: accepted", p)
		}
	}
}

func TestCheckAppRefusesAnAppWithASystemExtension(t *testing.T) {
	err := CheckApp(AppPlan{Name: "Guard", Bundle: "/Applications/Guard.app", SystemExtension: true}, AppOptions{})
	if err == nil || !strings.Contains(err.Error(), "system extension") {
		t.Fatalf("err = %v", err)
	}
	// A reset keeps the app and its extension.
	if err := CheckApp(AppPlan{Name: "Guard", SystemExtension: true}, AppOptions{}); err != nil {
		t.Fatalf("reset: %v", err)
	}
}
