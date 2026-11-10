package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed system.json
var systemJSON []byte

// SystemRule is a cleanup of files that belong to root. Most rules name a
// fixed folder, file name patterns and a minimum age: the cleanup removes
// only regular files that match all three, so it is one fixed find
// command. A rule with a Command runs a tool of macOS instead, from the
// fixed list allowedCommands, and Measure names the folders it empties.
type SystemRule struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Root       string   `json:"root"`
	Names      []string `json:"names,omitempty"`
	MinAgeDays int      `json:"minAgeDays,omitempty"`
	Summary    string   `json:"summary"`
	Recovery   string   `json:"recovery"`
	// Tier is "safe" (the default) or "review". Smart Clean takes only safe
	// rules; a review rule needs its own confirmation.
	Tier    string   `json:"tier,omitempty"`
	Command []string `json:"command,omitempty"`
	Measure []string `json:"measure,omitempty"`
}

// Review reports whether the rule needs its own confirmation.
func (r SystemRule) Review() bool { return r.Tier == "review" }

// allowedCommands are the only tools a system rule can run.
var allowedCommands = [][]string{
	{"/usr/bin/log", "erase", "--all"},
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
		case r.Summary == "" || r.Recovery == "":
			return System{}, fmt.Errorf("system rule %s: summary and recovery are required", r.ID)
		case r.Tier != "" && r.Tier != "safe" && r.Tier != "review":
			return System{}, fmt.Errorf("system rule %s: unknown tier %q", r.ID, r.Tier)
		}
		if len(r.Command) > 0 {
			if err := checkCommandRule(r); err != nil {
				return System{}, err
			}
			ids[r.ID] = true
			continue
		}
		if len(r.Names) == 0 || r.MinAgeDays < 1 || len(r.Measure) > 0 {
			return System{}, fmt.Errorf("system rule %s: names and a minimum age are required", r.ID)
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

// checkCommandRule keeps a command rule to a tool of the fixed list, a
// review tier and measure folders below /private/var/db.
func checkCommandRule(r SystemRule) error {
	allowed := false
	for _, c := range allowedCommands {
		if slices.Equal(c, r.Command) {
			allowed = true
		}
	}
	switch {
	case !allowed:
		return fmt.Errorf("system rule %s: the command is not in the allowed list", r.ID)
	case r.Tier != "review":
		return fmt.Errorf("system rule %s: a command rule must have the review tier", r.ID)
	case len(r.Names) > 0 || r.MinAgeDays != 0 || len(r.Measure) == 0:
		return fmt.Errorf("system rule %s: a command rule has measure folders and no names", r.ID)
	}
	for _, m := range r.Measure {
		if filepath.Clean(m) != m || !strings.HasPrefix(m, "/private/var/db/") {
			return fmt.Errorf("system rule %s: the measure folder %q is not allowed", r.ID, m)
		}
	}
	return nil
}
