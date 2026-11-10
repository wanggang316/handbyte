package apps

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/wanggang316/handbyte/catalog"
)

// Confidence says how sure the match of a file to an app is.
const (
	// High: the name is the bundle ID, or starts or ends with it at a dot.
	High = "high"
	// Medium: the name is the app name, or the Homebrew cask names the path.
	Medium = "medium"
	// Low: a dot folder with the app name. Shown, never selected.
	Low = "low"
)

// Related is a file or a folder that an app keeps in the home folder.
type Related struct {
	Path       string `json:"path"`
	Class      string `json:"class"`
	Confidence string `json:"confidence"`
	// Selected is the default choice for an uninstall.
	Selected bool `json:"selected"`
	// System files are outside the home folder and belong to root. They
	// are listed for the user; the tool does not remove them.
	System bool  `json:"system,omitempty"`
	IsDir  bool  `json:"is_dir"`
	Size   int64 `json:"size_bytes"`
}

// Target is what the matching needs to know about an app.
type Target struct {
	BundleID string
	Name     string
	Zap      []string
	// Bundle is the path of the .app folder. A launchd property list that
	// names a path inside it belongs to the app.
	Bundle string
	// Team is the signing team. OtherTeams are the teams of the other
	// installed apps: a group container of a shared team is not offered.
	Team       string
	OtherTeams []string
}

var teamID = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// Selected says whether a file of this class and confidence is chosen for
// an uninstall by default. Caches, logs and state are made again by the
// app; settings go with a sure match; the user's data stays unless asked.
func defaultSelected(class, confidence string, withData bool) bool {
	switch {
	case confidence == Low:
		return false
	case class == catalog.ClassData:
		return withData
	case class == catalog.ClassSettings:
		return confidence == High || withData
	}
	return true
}

// FindRelated returns the files of an app in the home folder, in path
// order. others are the bundle IDs of the other installed apps: a name that
// belongs to one of them more exactly is not taken.
func FindRelated(home string, t Target, others []string, withData bool) []Related {
	if isProtected(t.BundleID) {
		return nil
	}
	a := catalog.LoadApps()
	generic := map[string]bool{}
	for _, g := range a.GenericNames {
		generic[g] = true
	}
	nameOK := len([]rune(t.Name)) >= 3 && !generic[strings.ToLower(t.Name)]
	idOK := validBundleID(t.BundleID)
	found := map[string]Related{}
	add := func(path, class, confidence string) {
		path = filepath.Clean(path)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return
		}
		if belongsToOther(filepath.Base(path), t.BundleID, others) {
			return
		}
		// A higher confidence wins when two locations name one path.
		if old, ok := found[path]; ok && rank(old.Confidence) >= rank(confidence) {
			return
		}
		found[path] = Related{Path: path, Class: class, Confidence: confidence, IsDir: info.IsDir(), Selected: defaultSelected(class, confidence, withData)}
	}
	matchLocations(a, a.Locations, func(p string) string { return expand(home, p) }, t, nameOK, idOK, add)
	for _, folder := range a.LaunchdFolders {
		if strings.HasPrefix(folder, "~/") {
			lists, _ := launchdOwned(expand(home, folder), t, "")
			for _, p := range lists {
				add(p, catalog.ClassState, High)
			}
		}
	}
	for _, z := range t.Zap {
		if !strings.HasPrefix(z, "~/") || strings.ContainsAny(z, "*?[") {
			continue
		}
		path := expand(home, z)
		if !strings.HasPrefix(path, filepath.Join(home, "Library")+"/") {
			continue
		}
		add(path, classOf(path, home), Medium)
	}
	out := make([]Related, 0, len(found))
	for _, r := range found {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// FindSystem returns the files of an app outside the home folder, below
// root ("/" on a Mac, a test folder in tests), in path order. They are
// never selected: they belong to root.
func FindSystem(root string, t Target, others []string) []Related {
	if isProtected(t.BundleID) {
		return nil
	}
	a := catalog.LoadApps()
	generic := map[string]bool{}
	for _, g := range a.GenericNames {
		generic[g] = true
	}
	nameOK := len([]rune(t.Name)) >= 3 && !generic[strings.ToLower(t.Name)]
	found := map[string]Related{}
	add := func(path, class, confidence string) {
		path = filepath.Clean(path)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || belongsToOther(filepath.Base(path), t.BundleID, others) {
			return
		}
		if old, ok := found[path]; ok && rank(old.Confidence) >= rank(confidence) {
			return
		}
		found[path] = Related{Path: path, Class: class, Confidence: confidence, IsDir: info.IsDir(), System: true}
	}
	matchLocations(a, a.SystemLocations, func(p string) string { return filepath.Join(root, p) }, t, nameOK, validBundleID(t.BundleID), add)
	for _, folder := range a.LaunchdFolders {
		if strings.HasPrefix(folder, "/") {
			lists, helpers := launchdOwned(filepath.Join(root, folder), t, root)
			for _, p := range append(lists, helpers...) {
				add(p, catalog.ClassState, High)
			}
		}
	}
	out := make([]Related, 0, len(found))
	for _, r := range found {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// matchLocations expands each location for the target: a bundle ID match
// is high, a name or team match medium, a lower-case name low.
func matchLocations(a catalog.Apps, locations []catalog.AppLocation, expandPath func(string) string, t Target, nameOK, idOK bool, add func(path, class, confidence string)) {
	generic := map[string]bool{}
	for _, g := range a.GenericNames {
		generic[g] = true
	}
	for _, l := range locations {
		template := expandPath(l.Path)
		switch {
		case strings.Contains(template, "{bundleID}"):
			if !idOK {
				continue
			}
			for _, p := range matchSegment(template, "{bundleID}", t.BundleID) {
				add(p, l.Class, High)
			}
		case strings.Contains(template, "{name}"):
			if !nameOK {
				continue
			}
			for _, p := range matchSegment(template, "{name}", t.Name) {
				add(p, l.Class, Medium)
			}
		case strings.Contains(template, "{lowername}"):
			low := strings.ToLower(strings.ReplaceAll(t.Name, " ", ""))
			if !nameOK || len(low) < 3 || generic[low] {
				continue
			}
			for _, p := range matchSegment(template, "{lowername}", low) {
				add(p, l.Class, Low)
			}
		case strings.Contains(template, "{team}"):
			if !teamID.MatchString(t.Team) || slices.Contains(t.OtherTeams, t.Team) {
				continue
			}
			for _, p := range matchSegment(template, "{team}", t.Team) {
				add(p, l.Class, Medium)
			}
		}
	}
}

// launchdOwned returns the property lists in folder that belong to the
// app: they name its bundle ID in AssociatedBundleIdentifiers, or their
// program is inside the bundle. helpers are the programs of those lists in
// /Library/PrivilegedHelperTools, below root.
func launchdOwned(folder string, t Target, root string) (lists, helpers []string) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, nil
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".plist") || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(folder, e.Name())
		job := readLaunchdJob(path)
		if job == nil {
			continue
		}
		program := job.program()
		owned := t.Bundle != "" && strings.HasSuffix(t.Bundle, ".app") && strings.HasPrefix(program, t.Bundle+"/")
		if validBundleID(t.BundleID) && slices.ContainsFunc(job.associated(), func(id string) bool { return strings.EqualFold(id, t.BundleID) }) {
			owned = true
		}
		if !owned {
			continue
		}
		lists = append(lists, path)
		if dir, name := filepath.Split(program); filepath.Clean(dir) == "/Library/PrivilegedHelperTools" && name != "" {
			helpers = append(helpers, filepath.Join(root, program))
		}
	}
	return lists, helpers
}

type launchdJob map[string]any

// readLaunchdJob reads a launchd property list, in XML or binary form,
// through plutil. Nil for a file that is not a property list.
func readLaunchdJob(path string) launchdJob {
	out, err := exec.Command("/usr/bin/plutil", "-convert", "json", "-o", "-", "--", path).Output()
	if err != nil {
		return nil
	}
	var job launchdJob
	if json.Unmarshal(out, &job) != nil {
		return nil
	}
	return job
}

func (j launchdJob) program() string {
	if p, ok := j["Program"].(string); ok {
		return p
	}
	if args, ok := j["ProgramArguments"].([]any); ok && len(args) > 0 {
		p, _ := args[0].(string)
		return p
	}
	return ""
}

func (j launchdJob) associated() []string {
	switch v := j["AssociatedBundleIdentifiers"].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func rank(confidence string) int {
	switch confidence {
	case High:
		return 3
	case Medium:
		return 2
	}
	return 1
}

// matchSegment expands a template whose last segment holds the placeholder.
// "{x}" matches the name exactly, ignoring case; "{x}.*" matches names that
// start with the value and a dot; "*.{x}" matches names that end with a dot
// and the value.
func matchSegment(template, placeholder, value string) []string {
	dir, segment := filepath.Split(template)
	dir = filepath.Clean(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix, suffix, _ := strings.Cut(segment, placeholder)
	var out []string
	for _, e := range entries {
		name := e.Name()
		ok := false
		switch {
		case strings.HasPrefix(prefix, "*.") || prefix == "*.":
			ok = strings.HasSuffix(strings.ToLower(name), strings.ToLower("."+value+suffix)) && len(name) > len(value)+len(suffix)+1
		case strings.HasPrefix(suffix, ".*"):
			rest := strings.TrimPrefix(suffix, ".*")
			lower := strings.ToLower(name)
			ok = strings.HasPrefix(lower, strings.ToLower(prefix+value+".")) && strings.HasSuffix(lower, strings.ToLower(rest))
		default:
			ok = strings.EqualFold(name, prefix+value+suffix)
		}
		if ok {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

// belongsToOther reports whether a name matches another installed app more
// exactly: for app com.foo, the folder com.foo.helper belongs to the
// installed app com.foo.helper.
func belongsToOther(name, own string, others []string) bool {
	lower := strings.ToLower(name)
	for _, o := range others {
		o = strings.ToLower(o)
		if o == strings.ToLower(own) || len(o) <= len(own) {
			continue
		}
		if lower == o || strings.HasPrefix(lower, o+".") || strings.HasSuffix(lower, "."+o) {
			return true
		}
	}
	return false
}

// classOf guesses the class of a path from its place in ~/Library.
func classOf(path, home string) string {
	rel := strings.TrimPrefix(path, filepath.Join(home, "Library")+"/")
	switch {
	case strings.HasPrefix(rel, "Caches/"), strings.HasPrefix(rel, "HTTPStorages/"), strings.HasPrefix(rel, "WebKit/"):
		return catalog.ClassCache
	case strings.HasPrefix(rel, "Logs/"):
		return catalog.ClassLogs
	case strings.HasPrefix(rel, "Preferences/"):
		return catalog.ClassSettings
	case strings.HasPrefix(rel, "Saved Application State/"), strings.HasPrefix(rel, "LaunchAgents/"):
		return catalog.ClassState
	}
	return catalog.ClassData
}

// uuidLike finds a UUID part, as in the settings of a test run.
var uuidLike = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// Leftover is data of an app that is no longer installed.
type Leftover struct {
	Path     string `json:"path"`
	BundleID string `json:"bundle_id"`
	Class    string `json:"class"`
	IsDir    bool   `json:"is_dir"`
	Size     int64  `json:"size_bytes"`
}

// vendor is the first two components of a bundle ID: com.example.
func vendor(id string) string {
	parts := strings.SplitN(strings.ToLower(id), ".", 3)
	if len(parts) < 2 {
		return strings.ToLower(id)
	}
	return parts[0] + "." + parts[1]
}

// FindLeftovers returns entries of the leftover roots whose name is a bundle
// ID that no installed app has. installed must hold every bundle ID that is
// known, also of apps outside the application roots, and names the names of
// the installed apps. A name is not a leftover when it shares the vendor of
// an installed app, or when one of its parts is the vendor name or the app
// name of an installed app: it can be a helper of that app. The list is for
// review: nothing in it is selected.
func FindLeftovers(home string, installed, names []string) []Leftover {
	vendors := map[string]bool{}
	words := map[string]bool{}
	for _, id := range installed {
		v := vendor(id)
		vendors[v] = true
		if _, second, ok := strings.Cut(v, "."); ok && len(second) >= 4 {
			words[second] = true
		}
	}
	for _, n := range names {
		if w := strings.ToLower(strings.ReplaceAll(n, " ", "")); len(w) >= 4 {
			words[w] = true
		}
	}
	ignore := catalog.LoadApps().LeftoverIgnorePrefixes
	covered := func(id string) bool {
		if vendors[vendor(id)] {
			return true
		}
		for _, part := range strings.Split(strings.ToLower(id), ".") {
			if words[part] {
				return true
			}
		}
		lower := strings.ToLower(id)
		for _, p := range ignore {
			if strings.HasPrefix(lower, strings.ToLower(p)) {
				return true
			}
		}
		return false
	}
	var out []Leftover
	for _, root := range catalog.LoadApps().LeftoverRoots {
		dir := expand(home, root)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		preferences := strings.HasSuffix(root, "/Preferences")
		for _, e := range entries {
			name := e.Name()
			// Settings are files; all other app data is in folders.
			if preferences != strings.HasSuffix(name, ".plist") || preferences == e.IsDir() {
				continue
			}
			id := strings.TrimSuffix(strings.TrimSuffix(name, ".plist"), ".savedState")
			if e.Type()&os.ModeSymlink != 0 || strings.Count(id, ".") < 2 || !validBundleID(id) || isProtected(id) || covered(id) || uuidLike.MatchString(id) {
				continue
			}
			path := filepath.Join(dir, name)
			out = append(out, Leftover{Path: path, BundleID: id, Class: classOf(path, home), IsDir: e.IsDir()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// LeftoverGroup is the data of one app that is gone, from all roots.
type LeftoverGroup struct {
	BundleID string     `json:"bundle_id"`
	Items    []Leftover `json:"items"`
	Size     int64      `json:"size_bytes"`
}

// GroupLeftovers puts the leftovers of one app together, largest first.
// The app is the first three parts of the bundle ID, so that the data of
// its helpers and tests is one group.
func GroupLeftovers(items []Leftover) []LeftoverGroup {
	index := map[string]int{}
	var out []LeftoverGroup
	for _, l := range items {
		key := l.BundleID
		if parts := strings.SplitN(key, ".", 4); len(parts) > 3 {
			key = strings.Join(parts[:3], ".")
		}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, LeftoverGroup{BundleID: key})
		}
		out[i].Items = append(out[i].Items, l)
		out[i].Size += l.Size
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}
