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
	// SystemLocations are places outside the home folder. Their files
	// belong to root: they are listed, never selected and never removed.
	SystemLocations []AppLocation `json:"systemLocations"`
	// LaunchdFolders hold launchd property lists. A list that names a path
	// inside the app bundle belongs to the app.
	LaunchdFolders []string `json:"launchdFolders"`
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
// placeholder: {bundleID}, {name} (the app name), {lowername} or {team}
// (the signing team of the app). A segment
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
	check := func(l AppLocation, prefix string) error {
		if !strings.HasPrefix(l.Path, prefix) || strings.Contains(l.Path, "..") {
			return fmt.Errorf("location %q must start with %s", l.Path, prefix)
		}
		n := strings.Count(l.Path, "{")
		if n != 1 || !(strings.Contains(l.Path, "{bundleID}") || strings.Contains(l.Path, "{name}") || strings.Contains(l.Path, "{lowername}") || strings.Contains(l.Path, "{team}")) {
			return fmt.Errorf("location %q needs exactly one placeholder", l.Path)
		}
		if !classes[l.Class] {
			return fmt.Errorf("location %q: unknown class %q", l.Path, l.Class)
		}
		return nil
	}
	for _, l := range a.Locations {
		if err := check(l, "~/"); err != nil {
			return Apps{}, err
		}
	}
	for _, l := range a.SystemLocations {
		if err := check(l, "/Library/"); err != nil {
			return Apps{}, err
		}
	}
	for _, f := range a.LaunchdFolders {
		if !strings.HasPrefix(f, "~/Library/") && !strings.HasPrefix(f, "/Library/") {
			return Apps{}, fmt.Errorf("launchd folder %q must be in a Library folder", f)
		}
	}
	return a, nil
}
