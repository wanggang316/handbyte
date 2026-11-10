package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed system.json
var systemJSON []byte

// SystemRule is a cleanup of files that belong to root. It names a fixed
// folder, file name patterns and a minimum age: the cleanup removes only
// regular files that match all three, so it is one fixed command.
type SystemRule struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Root       string   `json:"root"`
	Names      []string `json:"names"`
	MinAgeDays int      `json:"minAgeDays"`
	Summary    string   `json:"summary"`
	Recovery   string   `json:"recovery"`
}

// System is the content of system.json.
type System struct {
	Version int          `json:"version"`
	Rules   []SystemRule `json:"rules"`
}

// LoadSystem returns the embedded system rules. It panics on a defective
// file, because that is a build defect.
func LoadSystem() System {
	s, err := ParseSystem(systemJSON)
	if err != nil {
		panic("catalog: " + err.Error())
	}
	return s
}

// ParseSystem decodes and validates system rules. The checks keep each
// rule a narrow, fixed command: an absolute root outside the home folders,
// plain name patterns, and a minimum age of at least one day.
func ParseSystem(data []byte) (System, error) {
	var s System
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return System{}, err
	}
	if s.Version != 1 {
		return System{}, fmt.Errorf("unsupported version %d", s.Version)
	}
	ids := map[string]bool{}
	for _, r := range s.Rules {
		switch {
		case r.ID == "" || ids[r.ID]:
			return System{}, fmt.Errorf("system rule %q is empty or repeated", r.ID)
		case !filepath.IsAbs(r.Root) || filepath.Clean(r.Root) != r.Root || r.Root == "/" || strings.HasPrefix(r.Root, "/Users") || strings.HasPrefix(r.Root, "/System"):
			return System{}, fmt.Errorf("system rule %s: the root %q is not allowed", r.ID, r.Root)
		case len(r.Names) == 0 || r.MinAgeDays < 1 || r.Summary == "" || r.Recovery == "":
			return System{}, fmt.Errorf("system rule %s: names, a minimum age, summary and recovery are required", r.ID)
		}
		for _, n := range r.Names {
			if !strings.HasPrefix(n, "*.") || strings.ContainsAny(n[2:], `*?[]/ '"\`) {
				return System{}, fmt.Errorf("system rule %s: the pattern %q must be *.<extension>", r.ID, n)
			}
		}
		ids[r.ID] = true
	}
	return s, nil
}
