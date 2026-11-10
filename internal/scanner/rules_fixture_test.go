package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wanggang316/handbyte/catalog"
)

// generic rules have no single owner; the CLI only shows their items.
var genericRules = map[string]bool{"library-caches": true, "library-logs": true, "xdg-cache": true, "trash": true, "downloads": true, "containers": true}

// Each preset finds its own location: a positive fixture for every rule.
func TestEveryPresetFindsItsLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := map[string]string{}
	for _, p := range Presets {
		rel := strings.ReplaceAll(strings.TrimPrefix(p.Path, "~/"), "*", "Default")
		dir := filepath.Join(home, rel)
		if p.ExpandChildren {
			dir = filepath.Join(dir, "child-of-"+p.Source)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		want[dir] = p.Source
	}
	items, err := Scan(context.Background(), Config{Workers: 4, SkipProjectScan: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	physicalHome, _ := Physical(home)
	got := map[string]string{}
	for _, it := range items {
		got[strings.Replace(it.Path, physicalHome, home, 1)] = it.Source
	}
	for dir, source := range want {
		if got[dir] != source {
			t.Errorf("%s: want %s, got %q", strings.TrimPrefix(dir, home), source, got[dir])
		}
	}
}

// A safe rule of one owner names the owner for both products: processes
// for the CLI, bundle IDs for the app.
func TestSafeRulesNameTheirOwner(t *testing.T) {
	for _, r := range catalog.Load().Rules {
		if r.Kind != catalog.KindPreset || r.Safety != catalog.SafetySafe || genericRules[r.ID] {
			continue
		}
		if len(r.Processes) == 0 {
			t.Errorf("%s: a safe rule needs owner processes", r.ID)
		}
	}
}
