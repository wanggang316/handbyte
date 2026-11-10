package usage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wanggang316/handbyte/catalog"
)

type categoryCase struct {
	Name         string   `json:"name"`
	ProjectRoots []string `json:"projectRoots"`
	Files        []struct {
		Path  string `json:"path"`
		Bytes int    `json:"bytes"`
	} `json:"files"`
	Expect map[string]int64 `json:"expect"`
}

// The cases are shared with the macOS app. See testdata/conformance/README.md.
func TestCategoryConformance(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/conformance/categories/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no conformance cases: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var c categoryCase
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			home := t.TempDir()
			for _, f := range c.Files {
				write(t, home, f.Path, f.Bytes)
			}
			r, err := Scan(context.Background(), home, Options{FilesPerDir: 1})
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]int64{}
			for _, total := range Categorize(r, catalog.LoadStorage().Paths(r.Path, c.ProjectRoots)) {
				got[total.ID] = total.Logical
			}
			if len(got) != len(c.Expect) {
				t.Fatalf("%s: got %v, want %v", c.Name, got, c.Expect)
			}
			for id, want := range c.Expect {
				if got[id] != want {
					t.Fatalf("%s: got %v, want %v", c.Name, got, c.Expect)
				}
			}
		})
	}
}

func TestCategoriesSumToTheRoot(t *testing.T) {
	home := t.TempDir()
	for i, p := range []string{"Movies/a", "Movies/b", "Library/Caches/x/y", "Library/z", "top", "Library/Application Support/q"} {
		write(t, home, p, 1000*(i+1))
	}
	r, err := Scan(context.Background(), home, Options{FilesPerDir: 1})
	if err != nil {
		t.Fatal(err)
	}
	var allocated, logical int64
	for _, total := range Categorize(r, catalog.LoadStorage().Paths(r.Path, nil)) {
		allocated += total.Allocated
		logical += total.Logical
	}
	if allocated != r.Root.Allocated || logical != r.Root.Logical {
		t.Fatalf("categories %d/%d != root %d/%d", allocated, logical, r.Root.Allocated, r.Root.Logical)
	}
}
