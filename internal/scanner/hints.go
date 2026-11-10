package scanner

import (
	"path/filepath"
	"strings"
)

// Hints finds the cleanup rule that covers a folder of the storage view. A
// hint is not a verdict: a cleanup scan checks the folder again before any
// action.
type Hints struct {
	presets   []Preset
	templates []string
	artifacts map[string]Preset
	roots     []string
}

// NewHints builds the lookup for a home directory and the project roots.
// A rule of the caution tier gives no hint: it holds user data, and the
// view must not call it cleanable.
func NewHints(home string, roots []string) Hints {
	h := Hints{artifacts: map[string]Preset{}}
	expand := func(p string) string {
		if p == "~" {
			return home
		}
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, p[2:])
		}
		return filepath.Clean(p)
	}
	for _, p := range Presets {
		if p.Safety == SafetyCaution {
			continue
		}
		h.presets = append(h.presets, p)
		h.templates = append(h.templates, expand(p.Path))
	}
	for _, a := range ProjectArtifactPatterns {
		if a.Safety == SafetyCaution {
			continue
		}
		h.artifacts[a.Name] = Preset{Category: a.Category, Safety: a.Safety, Description: a.Description, Source: "artifact:" + a.Name, Area: a.Area, Rebuildable: a.Rebuildable}
	}
	if len(roots) == 0 {
		roots = DefaultProjectRoots
	}
	for _, r := range roots {
		h.roots = append(h.roots, expand(r))
	}
	return h
}

func depth(template string) int { return templateDepth(template) }

// Rule returns the rule that covers path. The most specific rule wins. A
// folder that holds the location of another rule is not covered, as in a
// cleanup scan.
func (h Hints) Rule(path string) (Preset, bool) {
	path = filepath.Clean(path)
	best, found := Preset{}, false
	bestDepth := -1
	for i, p := range h.presets {
		template := h.templates[i]
		d := depth(p.Path)
		target := path
		if p.ExpandChildren {
			target = filepath.Dir(path)
			d++
		}
		if matchTemplate(template, target) && d > bestDepth {
			best, found, bestDepth = p, true, d
		}
	}
	if found {
		for _, template := range h.templates {
			if templateBelow(template, path) {
				return Preset{}, false
			}
		}
		return best, true
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

// templateBelow reports whether a template names a location strictly
// inside path.
func templateBelow(template, path string) bool {
	t := strings.Split(filepath.Clean(template), string(filepath.Separator))
	p := strings.Split(path, string(filepath.Separator))
	if len(t) <= len(p) {
		return false
	}
	return matchTemplate(strings.Join(t[:len(p)], string(filepath.Separator)), path)
}
