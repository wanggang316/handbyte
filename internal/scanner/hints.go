package scanner

import (
	"path/filepath"
	"strings"
)

// Hints finds the cleanup rule that covers a folder of the storage view. A
// hint is not a verdict: a cleanup scan checks the folder again before any
// action.
type Hints struct {
	exact, parents map[string]Preset
	artifacts      map[string]Preset
	roots          []string
}

// NewHints builds the lookup for a home directory and the project roots.
// When two presets reach one path, the one with the deeper template wins.
// A rule of the caution tier gives no hint: it holds user data, and the
// view must not call it cleanable.
func NewHints(home string, roots []string) Hints {
	h := Hints{exact: map[string]Preset{}, parents: map[string]Preset{}, artifacts: map[string]Preset{}}
	expand := func(p string) string {
		if p == "~" {
			return home
		}
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, p[2:])
		}
		return filepath.Clean(p)
	}
	keep := func(table map[string]Preset, path string, p Preset) {
		if other, ok := table[path]; ok && depth(other.Path) >= depth(p.Path) {
			return
		}
		table[path] = p
	}
	for _, p := range Presets {
		if p.Safety == SafetyCaution {
			continue
		}
		if p.ExpandChildren {
			keep(h.parents, expand(p.Path), p)
		} else {
			keep(h.exact, expand(p.Path), p)
		}
	}
	for _, a := range ProjectArtifactPatterns {
		if a.Safety == SafetyCaution {
			continue
		}
		h.artifacts[a.Name] = Preset{Category: a.Category, Safety: a.Safety, Description: a.Description, Source: "artifact:" + a.Name}
	}
	if len(roots) == 0 {
		roots = DefaultProjectRoots
	}
	for _, r := range roots {
		h.roots = append(h.roots, expand(r))
	}
	return h
}

func depth(template string) int { return strings.Count(template, "/") }

// Rule returns the rule that covers path.
func (h Hints) Rule(path string) (Preset, bool) {
	path = filepath.Clean(path)
	exact, isExact := h.exact[path]
	expanded, isChild := h.parents[filepath.Dir(path)]
	switch {
	case isExact && isChild:
		if depth(expanded.Path) > depth(exact.Path) {
			return expanded, true
		}
		return exact, true
	case isExact:
		return exact, true
	case isChild:
		return expanded, true
	}
	if a, ok := h.artifacts[filepath.Base(path)]; ok {
		for _, root := range h.roots {
			if Contains(root, path) && path != root {
				return a, true
			}
		}
	}
	return Preset{}, false
}
