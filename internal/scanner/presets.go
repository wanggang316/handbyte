package scanner

import "github.com/wanggang316/handbyte/catalog"

// Preset describes a known cleanable location on macOS.
//
// Path is the directory to inspect. If ExpandChildren is true, the scanner
// lists each direct child as a separate Item so the user can pick per app;
// otherwise the directory is reported as one Item.
type Preset struct {
	Path           string
	ExpandChildren bool
	Category       Category
	Safety         Safety
	Description    string
	Source         string
	Area           string
	// Rebuildable data is made again by its owner; see catalog.Rule.
	Rebuildable bool
	// Processes are executable names of the owner.
	Processes []string
}

// ProjectArtifact is a directory name commonly produced by build systems.
// The scanner walks user project roots and reports each match as a candidate
// cleanup target.
type ProjectArtifact struct {
	Name        string
	Category    Category
	Safety      Safety
	Description string
	Area        string
	Rebuildable bool
}

// The rule set comes from the shared catalog (catalog/cleanup.json), in
// catalog order: specific rules first, generic fallbacks last. Paths use ~
// for the home directory; the scanner expands them. Anything that does not
// exist on the current machine is silently skipped.
var (
	// Presets is the curated list of known cleanable macOS paths.
	Presets []Preset
	// ProjectArtifactPatterns are matched below the project roots.
	ProjectArtifactPatterns []ProjectArtifact
	// DefaultProjectRoots are scanned for ProjectArtifactPatterns by
	// default. Override via --roots flag.
	DefaultProjectRoots []string
)

func init() {
	c := catalog.Load()
	safety := map[string]Safety{
		catalog.SafetySafe:    SafetyAuto,
		catalog.SafetyReview:  SafetyReview,
		catalog.SafetyCaution: SafetyCaution,
	}
	for _, r := range c.Rules {
		category := Category(c.CategoryTitle(r.Category))
		description := r.Summary + " " + r.Recovery
		switch r.Kind {
		case catalog.KindPreset:
			Presets = append(Presets, Preset{
				Path:           r.Path,
				ExpandChildren: r.ExpandChildren,
				Category:       category,
				Safety:         safety[r.Safety],
				Description:    description,
				Source:         r.ID,
				Area:           r.Area,
				Rebuildable:    r.Rebuildable,
				Processes:      r.Processes,
			})
		case catalog.KindArtifact:
			ProjectArtifactPatterns = append(ProjectArtifactPatterns, ProjectArtifact{
				Name:        r.Name,
				Category:    category,
				Safety:      safety[r.Safety],
				Description: description,
				Area:        r.Area,
				Rebuildable: r.Rebuildable,
			})
		}
	}
	DefaultProjectRoots = c.ProjectRoots
}
