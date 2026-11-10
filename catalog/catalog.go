// Package catalog holds the cleanup rules that the CLI and the macOS app
// share. The JSON files in this directory are the source of truth; the app
// repository keeps a byte-identical copy.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed cleanup.json
var cleanupJSON []byte

const (
	KindPreset   = "preset"
	KindArtifact = "artifact"

	SafetySafe    = "safe"
	SafetyReview  = "review"
	SafetyCaution = "caution"
)

// Cleanup is the content of cleanup.json.
type Cleanup struct {
	Version      int        `json:"version"`
	Categories   []Category `json:"categories"`
	ProjectRoots []string   `json:"projectRoots"`
	Rules        []Rule     `json:"rules"`
}

type Category struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Rule is one cleanup rule. A preset names a location below the home
// directory. An artifact names a directory that build tools make below the
// project roots. Several presets can share one ID when they are the same
// rule at different locations.
type Rule struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Path           string   `json:"path,omitempty"`
	Name           string   `json:"name,omitempty"`
	ExpandChildren bool     `json:"expandChildren,omitempty"`
	Category       string   `json:"category"`
	Safety         string   `json:"safety"`
	Owners         []string `json:"owners,omitempty"`
	Summary        string   `json:"summary"`
	Recovery       string   `json:"recovery"`
}

// Load returns the embedded cleanup catalog. It panics on a defective
// catalog, because that is a build defect and not a runtime condition.
func Load() Cleanup {
	c, err := Parse(cleanupJSON)
	if err != nil {
		panic("catalog: " + err.Error())
	}
	return c
}

// Parse decodes and validates a cleanup catalog.
func Parse(data []byte) (Cleanup, error) {
	var c Cleanup
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Cleanup{}, err
	}
	if c.Version != 1 {
		return Cleanup{}, fmt.Errorf("unsupported version %d", c.Version)
	}
	categories := map[string]bool{}
	for _, cat := range c.Categories {
		if cat.ID == "" || cat.Title == "" || categories[cat.ID] {
			return Cleanup{}, fmt.Errorf("category %q is empty or repeated", cat.ID)
		}
		categories[cat.ID] = true
	}
	for _, root := range c.ProjectRoots {
		if !strings.HasPrefix(root, "~/") {
			return Cleanup{}, fmt.Errorf("project root %q must start with ~/", root)
		}
	}
	locations := map[string]bool{}
	for i, r := range c.Rules {
		fail := func(msg string) (Cleanup, error) {
			return Cleanup{}, fmt.Errorf("rule %d (%s): %s", i, r.ID, msg)
		}
		if !categories[r.Category] {
			return fail("unknown category " + r.Category)
		}
		if r.Safety != SafetySafe && r.Safety != SafetyReview && r.Safety != SafetyCaution {
			return fail("unknown safety " + r.Safety)
		}
		if r.Summary == "" || r.Recovery == "" {
			return fail("summary and recovery are required")
		}
		var location string
		switch r.Kind {
		case KindPreset:
			if r.ID == "" || strings.HasPrefix(r.ID, "artifact:") || r.Name != "" {
				return fail("a preset needs an ID without the artifact prefix and no name")
			}
			if !strings.HasPrefix(r.Path, "~/") || strings.Contains(r.Path, "..") || strings.HasSuffix(r.Path, "/") {
				return fail("path must start with ~/ and contain no .. or trailing slash")
			}
			location = r.Path
		case KindArtifact:
			if r.Name == "" || strings.ContainsAny(r.Name, "/~") || r.ID != "artifact:"+r.Name || r.Path != "" || r.ExpandChildren {
				return fail("an artifact needs a plain name and the ID artifact:<name>")
			}
			location = r.ID
		default:
			return fail("unknown kind " + r.Kind)
		}
		if locations[location] {
			return fail("location is repeated")
		}
		locations[location] = true
	}
	return c, nil
}

// CategoryTitle returns the display title of a category ID.
func (c Cleanup) CategoryTitle(id string) string {
	for _, cat := range c.Categories {
		if cat.ID == id {
			return cat.Title
		}
	}
	return ""
}
