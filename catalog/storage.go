package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed storage.json
var storageJSON []byte

// OtherCategory is the category of what no path rule covers.
const OtherCategory = "other"

// Storage is the content of storage.json: the categories of the storage
// views. A category owns the trees below its paths; the longest path wins.
type Storage struct {
	Version    int               `json:"version"`
	Categories []StorageCategory `json:"categories"`
}

type StorageCategory struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Paths []string `json:"paths"`
	// ProjectRoots adds the configured project roots to the paths.
	ProjectRoots bool `json:"projectRoots,omitempty"`
}

// CategoryPath is one path rule with the home directory expanded.
type CategoryPath struct {
	Path     string
	Category string
}

// LoadStorage returns the embedded storage categories. It panics on a
// defective file, because that is a build defect.
func LoadStorage() Storage {
	s, err := ParseStorage(storageJSON)
	if err != nil {
		panic("catalog: " + err.Error())
	}
	return s
}

// ParseStorage decodes and validates storage categories.
func ParseStorage(data []byte) (Storage, error) {
	var s Storage
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Storage{}, err
	}
	if s.Version != 1 {
		return Storage{}, fmt.Errorf("unsupported version %d", s.Version)
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	for _, c := range s.Categories {
		if c.ID == "" || c.Title == "" || ids[c.ID] {
			return Storage{}, fmt.Errorf("category %q is empty or repeated", c.ID)
		}
		ids[c.ID] = true
		for _, p := range c.Paths {
			if !(strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "/")) || strings.Contains(p, "..") || (len(p) > 1 && strings.HasSuffix(p, "/")) {
				return Storage{}, fmt.Errorf("category %s: path %q must be absolute or start with ~/, with no .. or trailing slash", c.ID, p)
			}
			if paths[p] {
				return Storage{}, fmt.Errorf("path %q is in two categories", p)
			}
			paths[p] = true
		}
	}
	if !ids[OtherCategory] {
		return Storage{}, fmt.Errorf("the category %q is required", OtherCategory)
	}
	return s, nil
}

// Paths expands the path rules for a home directory and the project roots.
func (s Storage) Paths(home string, projectRoots []string) []CategoryPath {
	expand := func(p string) string {
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, p[2:])
		}
		return filepath.Clean(p)
	}
	var out []CategoryPath
	for _, c := range s.Categories {
		for _, p := range c.Paths {
			out = append(out, CategoryPath{expand(p), c.ID})
		}
		if c.ProjectRoots {
			for _, p := range projectRoots {
				out = append(out, CategoryPath{expand(p), c.ID})
			}
		}
	}
	return out
}

// Title returns the display title of a category ID.
func (s Storage) Title(id string) string {
	for _, c := range s.Categories {
		if c.ID == id {
			return c.Title
		}
	}
	return ""
}
