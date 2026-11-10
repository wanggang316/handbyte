package scanner

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

// The golden file fixes the fields that decide what a scan can report, so a
// change of the rule source cannot change the cleanup scope by accident.
func TestRuleSetMatchesGolden(t *testing.T) {
	var b strings.Builder
	for _, p := range Presets {
		fmt.Fprintf(&b, "preset\t%s\t%s\t%t\t%s\t%s\t%s\trebuildable=%t\t%s\n", p.Source, p.Path, p.ExpandChildren, p.Category, p.Safety, p.Area, p.Rebuildable, strings.Join(p.Processes, ","))
	}
	for _, p := range ProjectArtifactPatterns {
		fmt.Fprintf(&b, "artifact\t%s\t%s\t%s\t%s\trebuildable=%t\n", p.Name, p.Category, p.Safety, p.Area, p.Rebuildable)
	}
	for _, r := range DefaultProjectRoots {
		fmt.Fprintf(&b, "root\t%s\n", r)
	}
	const path = "testdata/rules.golden"
	if *updateGolden {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(want) {
		t.Fatalf("rule set differs from %s; run with -update only for an intended change\n%s", path, b.String())
	}
}
