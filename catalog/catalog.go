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
	Version int `json:"version"`
	// Areas group the rules on the first screen of a cleanup, in order.
	Areas        []Area     `json:"areas"`
	Categories   []Category `json:"categories"`
	ProjectRoots []string   `json:"projectRoots"`
	Rules        []Rule     `json:"rules"`
}

type Category struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Area is a group of rules that the result of a cleanup shows as one line.
type Area struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Rule is one cleanup rule. A preset names a location below the home
// directory; a path segment "*" stands for each directory at that place,
// for example each browser profile. An artifact names a directory that
// build tools make below the project roots. Several presets can share one
// ID when they are the same rule at different locations.
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
	Area           string   `json:"area"`
	// Rebuildable data is made again by its owner, so it is removed
	// directly. Other data goes to the Trash.
	Rebuildable bool `json:"rebuildable"`
	// Processes are executable names of the owner. While one runs, the
	// CLI does not clean the rule's items. No processes: the CLI cannot
	// check the owner and only shows the items.
	Processes []string `json:"processes,omitempty"`
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
	if c.Version != 2 {
		return Cleanup{}, fmt.Errorf("unsupported version %d", c.Version)
	}
	areas := map[string]bool{}
	for _, a := range c.Areas {
		if a.ID == "" || a.Title == "" || areas[a.ID] {
			return Cleanup{}, fmt.Errorf("area %q is empty or repeated", a.ID)
		}
		areas[a.ID] = true
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
		if !areas[r.Area] {
			return fail("unknown area " + r.Area)
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
			for _, segment := range strings.Split(r.Path, "/") {
				if strings.Contains(segment, "*") && segment != "*" {
					return fail("a wildcard must be a whole path segment")
				}
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

// AreaTitle returns the display title of an area ID.
func (c Cleanup) AreaTitle(id string) string {
	for _, a := range c.Areas {
		if a.ID == id {
			return a.Title
		}
	}
	return ""
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
