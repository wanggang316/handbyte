package catalog

import (
	"strings"
	"testing"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	c, err := Parse(cleanupJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rules) == 0 || len(c.ProjectRoots) == 0 {
		t.Fatal("catalog is empty")
	}
}

func TestParseRejectsDefectiveRules(t *testing.T) {
	const head = `{"version":2,"areas":[{"id":"x","title":"X"}],"categories":[{"id":"c","title":"C"}],"projectRoots":["~/dev"],"rules":[`
	ok := `{"id":"a","kind":"preset","path":"~/Library/Caches/a","category":"c","safety":"safe","summary":"s","recovery":"r","area":"x","rebuildable":true}`
	if _, err := Parse([]byte(head + ok + "]}")); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}
	cases := map[string]string{
		"path outside home":   strings.Replace(ok, "~/Library/Caches/a", "/Library/Caches/a", 1),
		"path with dot dot":   strings.Replace(ok, "~/Library/Caches/a", "~/Library/../a", 1),
		"unknown category":    strings.Replace(ok, `"category":"c"`, `"category":"x"`, 1),
		"unknown safety":      strings.Replace(ok, `"safety":"safe"`, `"safety":"auto"`, 1),
		"unknown kind":        strings.Replace(ok, `"kind":"preset"`, `"kind":"glob"`, 1),
		"missing recovery":    strings.Replace(ok, `"recovery":"r"`, `"recovery":""`, 1),
		"unknown field":       strings.Replace(ok, `"id":"a"`, `"id":"a","glob":"*"`, 1),
		"repeated location":   ok + "," + ok,
		"unknown area":        strings.Replace(ok, `"area":"x"`, `"area":"y"`, 1),
		"partial wildcard":    strings.Replace(ok, "~/Library/Caches/a", "~/Library/Caches/a*", 1),
		"artifact id differs": `{"id":"artifact:x","kind":"artifact","name":"y","category":"c","safety":"review","summary":"s","recovery":"r","area":"x"}`,
		"artifact with slash": `{"id":"artifact:a/b","kind":"artifact","name":"a/b","category":"c","safety":"review","summary":"s","recovery":"r","area":"x"}`,
	}
	for name, rule := range cases {
		if _, err := Parse([]byte(head + rule + "]}")); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEmbeddedStorageCategoriesAreValid(t *testing.T) {
	s, err := ParseStorage(storageJSON)
	if err != nil {
		t.Fatal(err)
	}
	paths := s.Paths("/Users/alice", []string{"~/dev"})
	found := map[string]string{}
	for _, p := range paths {
		found[p.Path] = p.Category
	}
	if found["/Users/alice/Movies"] != "media" || found["/Users/alice/dev"] != "developer" || found["/Applications"] != "apps" {
		t.Fatalf("unexpected paths: %v", found)
	}
}

func TestParseStorageRejectsDefects(t *testing.T) {
	cases := map[string]string{
		"no other":       `{"version":1,"categories":[{"id":"a","title":"A","paths":[]}]}`,
		"relative path":  `{"version":1,"categories":[{"id":"other","title":"O","paths":["Movies"]}]}`,
		"repeated path":  `{"version":1,"categories":[{"id":"a","title":"A","paths":["~/x"]},{"id":"other","title":"O","paths":["~/x"]}]}`,
		"repeated id":    `{"version":1,"categories":[{"id":"other","title":"A","paths":[]},{"id":"other","title":"O","paths":[]}]}`,
		"unknown field":  `{"version":1,"categories":[{"id":"other","title":"O","paths":[],"glob":"*"}]}`,
		"trailing slash": `{"version":1,"categories":[{"id":"other","title":"O","paths":["~/x/"]}]}`,
	}
	for name, data := range cases {
		if _, err := ParseStorage([]byte(data)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
