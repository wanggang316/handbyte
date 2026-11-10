package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wanggang316/handbyte/catalog"
)

type conformanceCase struct {
	Name         string   `json:"name"`
	ProjectRoots []string `json:"projectRoots"`
	Files        []struct {
		Path  string `json:"path"`
		Bytes int    `json:"bytes"`
	} `json:"files"`
	Expect []struct {
		Path     string `json:"path"`
		Rule     string `json:"rule"`
		Category string `json:"category"`
		Safety   string `json:"safety"`
		Area     string `json:"area"`
	} `json:"expect"`
}

// The cases are shared with the macOS app. See testdata/conformance/README.md.
func TestCleanupConformance(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/conformance/cleanup/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no conformance cases: %v", err)
	}
	titles := catalog.Load()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var c conformanceCase
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, f := range c.Files {
				full := filepath.Join(home, f.Path)
				if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, make([]byte, f.Bytes), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := Config{Workers: 2, ProjectRoots: c.ProjectRoots, SkipProjectScan: len(c.ProjectRoots) == 0}
			items, err := Scan(context.Background(), cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			physicalHome, err := Physical(home)
			if err != nil {
				t.Fatal(err)
			}
			var got, want []string
			for _, it := range items {
				if it.ScanStatus == "unavailable" {
					continue
				}
				rel, err := filepath.Rel(physicalHome, it.Path)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", rel, it.Source, it.Category, it.Safety, it.Area))
			}
			for _, e := range c.Expect {
				want = append(want, fmt.Sprintf("%s\t%s\t%s\t%s\t%s", e.Path, e.Rule, titles.CategoryTitle(e.Category), e.Safety, e.Area))
			}
			sort.Strings(got)
			sort.Strings(want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("%s\ngot:\n%s\nwant:\n%s", c.Name, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}
