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
	const head = `{"version":1,"categories":[{"id":"c","title":"C"}],"projectRoots":["~/dev"],"rules":[`
	ok := `{"id":"a","kind":"preset","path":"~/Library/Caches/a","category":"c","safety":"safe","summary":"s","recovery":"r"}`
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
		"artifact id differs": `{"id":"artifact:x","kind":"artifact","name":"y","category":"c","safety":"review","summary":"s","recovery":"r"}`,
		"artifact with slash": `{"id":"artifact:a/b","kind":"artifact","name":"a/b","category":"c","safety":"review","summary":"s","recovery":"r"}`,
	}
	for name, rule := range cases {
		if _, err := Parse([]byte(head + rule + "]}")); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
