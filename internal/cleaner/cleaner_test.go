package cleaner

import (
	"context"
	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutePreviewsUniqueReviewedTargets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".npm/_cacache")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "data"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	idle := func(context.Context, scanner.Item) (scanner.Activity, error) { return scanner.Idle, nil }
	items, err := scanner.Scan(context.Background(), scanner.Config{Workers: 1, SkipProjectScan: true, ActivityProbe: idle}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Selectable() {
		t.Fatalf("bad fixture: %+v", items)
	}
	results := Execute(context.Background(), append(items, items...), Options{Mode: ModeDryRun, ActivityProbe: idle}, nil)
	if len(results) != 1 || results[0].Status != "previewed" || results[0].Freed != 0 || results[0].Previewed != items[0].Size {
		t.Fatalf("incorrect preview: %+v", results)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("preview changed target", err)
	}
}

func TestExecuteRefusesProtectedAndChangedTargets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(os.Getenv("HOME"), ".npm/_cacache")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	idle := func(context.Context, scanner.Item) (scanner.Activity, error) { return scanner.Idle, nil }
	items, err := scanner.Scan(context.Background(), scanner.Config{Workers: 1, SkipProjectScan: true, ActivityProbe: idle}, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("scan: %v %v", items, err)
	}
	run := func(item scanner.Item, opts Options) {
		t.Helper()
		r := Execute(context.Background(), []scanner.Item{item}, opts, nil)
		if len(r) != 1 || r[0].Status != "skipped" {
			t.Fatalf("unsafe target accepted: %+v", r)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("protected data changed", err)
		}
	}
	protected := items[0]
	protected.Eligibility = scanner.Protected
	run(protected, Options{Mode: ModeRemove, ActivityProbe: idle})
	run(items[0], Options{Mode: ModeRemove, ActivityProbe: idle, Exclusions: []string{path}})
	run(items[0], Options{Mode: ModeRemove, ActivityProbe: func(context.Context, scanner.Item) (scanner.Activity, error) { return scanner.Active, nil }})
	changed := items[0]
	changed.Identity.Inode++
	run(changed, Options{Mode: ModeRemove, ActivityProbe: idle})
	cfg, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Exclusions = []string{path}
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	run(items[0], Options{Mode: ModeRemove, ActivityProbe: idle})
}

func TestExecuteCancellationKeepsPendingTargets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := Execute(ctx, []scanner.Item{{Path: "/fixture/a"}, {Path: "/fixture/b"}}, Options{Mode: ModeRemove}, nil)
	if len(results) != 2 {
		t.Fatal(results)
	}
	for _, r := range results {
		if r.Status != "cancelled" {
			t.Fatalf("not cancelled: %+v", r)
		}
	}
}

func TestOverlappingSelectionSkipsParent(t *testing.T) {
	results := Execute(context.Background(), []scanner.Item{{Path: "/fixture"}, {Path: "/fixture/cache"}}, Options{Mode: ModeDryRun}, nil)
	if len(results) != 2 || !strings.Contains(results[0].Message, "overlaps") {
		t.Fatalf("parent overlap not blocked: %+v", results)
	}
}
