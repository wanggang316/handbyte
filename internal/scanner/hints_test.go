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
		"Downloads":                                   "",
		"Library/Developer/Xcode/Archives":            "",
		"Library/Caches/Google":                       "",
		"Library/Caches/Google/Chrome":                "chrome-cache",
		"Library/Application Support/Google/Chrome/Profile 2/Code Cache": "chrome-profile-cache",
		"Library/Application Support/Google/Chrome/Profile 2/Cookies":    "",
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
