package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed apps.json
var appsJSON []byte

// Apps is the content of apps.json: where an app keeps its files, and the
// rules that keep the matching exact.
type Apps struct {
	Version          int           `json:"version"`
	ApplicationRoots []string      `json:"applicationRoots"`
	Locations        []AppLocation `json:"locations"`
	// LeftoverRoots are searched for data of apps that are gone.
	LeftoverRoots []string `json:"leftoverRoots"`
	// GenericNames never match an app by name.
	GenericNames []string `json:"genericNames"`
	// ProtectedPrefixes are bundle IDs that are never offered.
	ProtectedPrefixes []string `json:"protectedPrefixes"`
	// LeftoverIgnorePrefixes are names of libraries and updaters that many
	// apps share; their folders are not leftovers of one app.
	LeftoverIgnorePrefixes []string `json:"leftoverIgnorePrefixes"`
}

// AppLocation is a place where an app keeps files. The path holds one
// placeholder: {bundleID}, {name} (the app name) or {lowername}. A segment
// may end in ".*" (a dot, then anything) or start with "*." (anything, then
// a dot): the dot keeps the match on a name boundary.
type AppLocation struct {
	Path  string `json:"path"`
	Class string `json:"class"`
}

// Classes of app files. Caches, logs and state are made again by the app;
// data and settings are the user's.
const (
	ClassCache    = "cache"
	ClassLogs     = "logs"
	ClassState    = "state"
	ClassSettings = "settings"
	ClassData     = "data"
)

// LoadApps returns the embedded app catalog. It panics on a defective file,
// because that is a build defect.
func LoadApps() Apps {
	a, err := ParseApps(appsJSON)
	if err != nil {
		panic("catalog: " + err.Error())
	}
	return a
}

// ParseApps decodes and validates an app catalog.
func ParseApps(data []byte) (Apps, error) {
	var a Apps
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return Apps{}, err
	}
	if a.Version != 1 {
		return Apps{}, fmt.Errorf("unsupported version %d", a.Version)
	}
	classes := map[string]bool{ClassCache: true, ClassLogs: true, ClassState: true, ClassSettings: true, ClassData: true}
	for _, l := range a.Locations {
		if !strings.HasPrefix(l.Path, "~/") || strings.Contains(l.Path, "..") {
			return Apps{}, fmt.Errorf("location %q must start with ~/", l.Path)
		}
		n := strings.Count(l.Path, "{")
		if n != 1 || !(strings.Contains(l.Path, "{bundleID}") || strings.Contains(l.Path, "{name}") || strings.Contains(l.Path, "{lowername}")) {
			return Apps{}, fmt.Errorf("location %q needs exactly one placeholder", l.Path)
		}
		if !classes[l.Class] {
			return Apps{}, fmt.Errorf("location %q: unknown class %q", l.Path, l.Class)
		}
	}
	return a, nil
}
