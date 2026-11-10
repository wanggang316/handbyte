package scanner

import (
	"path/filepath"
	"testing"
)

func TestCoveringRule(t *testing.T) {
	home := "/Users/alice"
	h := NewHints(home, []string{"~/dev"})
	cases := map[string]string{
		"Library/Caches/go-build":                     "go-build",
		".npm/_cacache":                               "npm-cache",
		"Library/Caches/com.example.app":              "library-caches",
		"Library/Developer/Xcode/DerivedData/App-abc": "xcode-derived",
		"dev/web/node_modules":                        "artifact:node_modules",
		"Library/Caches":                              "",
		"Library/Caches/go-build/00":                  "",
		"Music/node_modules":                          "",
		"dev":                                         "",
		"Documents":                                   "",
	}
	for rel, want := range cases {
		got := ""
		if p, ok := h.Rule(filepath.Join(home, rel)); ok {
			got = p.Source
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", rel, got, want)
		}
	}
}
