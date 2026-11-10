// Package apps lists the installed apps, finds the files each app keeps in
// the home folder, and finds the data of apps that are gone. It reads only;
// the cleaner package moves files.
package apps

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wanggang316/handbyte/catalog"
	"github.com/wanggang316/handbyte/internal/usage"
)

// Source says how an app was installed.
const (
	SourceAppStore = "App Store"
	SourceHomebrew = "Homebrew"
	SourceSetapp   = "Setapp"
	SourceDirect   = "Direct"
)

// App is one installed application bundle.
type App struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	BundleID   string    `json:"bundle_id"`
	Version    string    `json:"version,omitempty"`
	Executable string    `json:"-"`
	Size       int64     `json:"size_bytes"`
	LastUsed   time.Time `json:"last_used,omitempty"`
	// LastUsedKnown is false when Spotlight has no date; that is not the
	// same as "never opened".
	LastUsedKnown bool     `json:"last_used_known"`
	Source        string   `json:"source"`
	CaskToken     string   `json:"cask,omitempty"`
	Running       bool     `json:"running"`
	System        bool     `json:"system"`
	Archs         []string `json:"architectures,omitempty"`
	// Team is the signing team, empty for an app without one.
	Team string `json:"team,omitempty"`
	// SystemExtension: the app installs a system extension. Moving it to
	// the Trash leaves the extension active, so an uninstall is refused.
	SystemExtension bool `json:"system_extension,omitempty"`
	// Zap are the paths the Homebrew cask names for a full removal.
	Zap []string `json:"-"`
}

// Options controls Inventory. Tests set the roots and the probes.
type Options struct {
	Home  string
	Roots []string
	// Caskrooms are the Homebrew cask folders.
	Caskrooms []string
	// Processes lists the executables that run, as `ps -axo comm=` prints them.
	Processes func() []string
	// LastUsed returns the last-used dates of app paths; false: unknown.
	LastUsed func(paths []string) []time.Time
	// SkipSizes leaves the sizes at zero, for a fast list.
	SkipSizes bool
	// Team reads the signing team of a bundle; nil leaves it empty.
	Team func(bundle string) string
}

// DefaultOptions are the real locations and probes.
func DefaultOptions(home string) Options {
	a := catalog.LoadApps()
	var roots []string
	for _, r := range a.ApplicationRoots {
		roots = append(roots, expand(home, r))
	}
	return Options{
		Home:      home,
		Roots:     roots,
		Caskrooms: []string{"/opt/homebrew/Caskroom", "/usr/local/Caskroom"},
		Processes: runningExecutables,
		LastUsed:  spotlightLastUsed,
		Team:      signingTeam,
	}
}

func expand(home, p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

var reverseDNS = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*(\.[A-Za-z0-9][A-Za-z0-9_-]*)+$`)

// validBundleID reports whether id is a reverse DNS name that file names
// can be matched against.
func validBundleID(id string) bool { return len(id) >= 5 && reverseDNS.MatchString(id) }

// isProtected reports whether a bundle ID belongs to the system.
func isProtected(id string) bool {
	for _, p := range catalog.LoadApps().ProtectedPrefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// Inventory lists the apps directly in the roots, sorted by name. Nested
// apps and symbolic links are left out.
func Inventory(ctx context.Context, opts Options) ([]App, error) {
	casks := caskApps(opts.Caskrooms)
	var apps []App
	for _, root := range opts.Roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			// A name that starts with a dot is a hidden helper, not an app the user installed.
			if !strings.HasSuffix(e.Name(), ".app") || strings.HasPrefix(e.Name(), ".") || e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
				continue
			}
			path := filepath.Join(root, e.Name())
			info, err := readInfo(filepath.Join(path, "Contents", "Info.plist"))
			if err != nil {
				continue
			}
			app := App{
				Path:       path,
				Name:       strings.TrimSuffix(e.Name(), ".app"),
				BundleID:   info["CFBundleIdentifier"],
				Version:    info["CFBundleShortVersionString"],
				Executable: info["CFBundleExecutable"],
				Source:     SourceDirect,
			}
			app.System = isProtected(app.BundleID)
			switch {
			case exists(filepath.Join(path, "Contents", "_MASReceipt", "receipt")):
				app.Source = SourceAppStore
			case strings.Contains(path, "/Setapp/"):
				app.Source = SourceSetapp
			}
			if c, ok := casks[e.Name()]; ok {
				app.Source, app.CaskToken, app.Zap = SourceHomebrew, c.token, c.zap
			}
			app.Archs = architectures(filepath.Join(path, "Contents", "MacOS", app.Executable))
			if ext, err := os.ReadDir(filepath.Join(path, "Contents", "Library", "SystemExtensions")); err == nil && len(ext) > 0 {
				app.SystemExtension = true
			}
			apps = append(apps, app)
		}
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name) })

	if opts.Processes != nil {
		running := opts.Processes()
		for i := range apps {
			prefix := apps[i].Path + "/Contents/"
			for _, p := range running {
				if strings.HasPrefix(p, prefix) {
					apps[i].Running = true
					break
				}
			}
		}
	}
	if opts.Team != nil {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		for i := range apps {
			wg.Add(1)
			go func(a *App) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				a.Team = opts.Team(a.Path)
			}(&apps[i])
		}
		wg.Wait()
	}
	if opts.LastUsed != nil && len(apps) > 0 {
		paths := make([]string, len(apps))
		for i, a := range apps {
			paths[i] = a.Path
		}
		dates := opts.LastUsed(paths)
		for i := range apps {
			if i < len(dates) && !dates[i].IsZero() {
				apps[i].LastUsed, apps[i].LastUsedKnown = dates[i], true
			}
		}
	}
	if !opts.SkipSizes {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		for i := range apps {
			wg.Add(1)
			go func(a *App) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if r, err := usage.Scan(ctx, a.Path, usage.Options{Workers: 2, LargestFiles: 1, FilesPerDir: 1}); err == nil {
					a.Size = r.Root.Allocated
				}
			}(&apps[i])
		}
		wg.Wait()
	}
	return apps, ctx.Err()
}

// signingTeam reads the team identifier of the code signature. codesign
// prints it without a check of the signature.
func signingTeam(bundle string) string {
	out, _ := exec.Command("/usr/bin/codesign", "-dv", "--", bundle).CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "TeamIdentifier="); ok && teamID.MatchString(v) {
			return v
		}
	}
	return ""
}

// TargetFor returns the matching facts of an app; all is the inventory.
func TargetFor(a App, all []App) Target {
	t := Target{BundleID: a.BundleID, Name: a.Name, Zap: a.Zap, Bundle: a.Path, Team: a.Team}
	for _, o := range all {
		if o.Path != a.Path && o.Team != "" {
			t.OtherTeams = append(t.OtherTeams, o.Team)
		}
	}
	return t
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// architectures reads the CPU types of a Mach-O executable.
func architectures(path string) []string {
	names := func(cpu macho.Cpu) string {
		switch cpu {
		case macho.CpuArm64:
			return "arm64"
		case macho.CpuAmd64:
			return "x86_64"
		}
		return cpu.String()
	}
	if fat, err := macho.OpenFat(path); err == nil {
		defer fat.Close()
		var out []string
		for _, a := range fat.Arches {
			out = append(out, names(a.Cpu))
		}
		return out
	}
	if f, err := macho.Open(path); err == nil {
		defer f.Close()
		return []string{names(f.Cpu)}
	}
	return nil
}

type cask struct {
	token string
	zap   []string
}

// caskApps reads the install receipts of the Homebrew casks: the app names
// they installed and the paths their zap stanza names.
func caskApps(roots []string) map[string]cask {
	out := map[string]cask{}
	for _, root := range roots {
		tokens, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, t := range tokens {
			data, err := os.ReadFile(filepath.Join(root, t.Name(), ".metadata", "INSTALL_RECEIPT.json"))
			if err != nil {
				continue
			}
			var receipt struct {
				Artifacts []map[string]json.RawMessage `json:"uninstall_artifacts"`
			}
			if json.Unmarshal(data, &receipt) != nil {
				continue
			}
			c := cask{token: t.Name()}
			var appNames []string
			for _, a := range receipt.Artifacts {
				if raw, ok := a["app"]; ok {
					var names []any
					if json.Unmarshal(raw, &names) == nil {
						for _, n := range names {
							if s, ok := n.(string); ok {
								appNames = append(appNames, filepath.Base(s))
							}
						}
					}
				}
				if raw, ok := a["zap"]; ok {
					var stanzas []map[string]any
					if json.Unmarshal(raw, &stanzas) == nil {
						for _, st := range stanzas {
							for _, key := range []string{"trash", "delete"} {
								switch v := st[key].(type) {
								case []any:
									for _, p := range v {
										if s, ok := p.(string); ok {
											c.zap = append(c.zap, s)
										}
									}
								case string:
									c.zap = append(c.zap, v)
								}
							}
						}
					}
				}
			}
			for _, n := range appNames {
				out[n] = c
			}
		}
	}
	return out
}

// runningExecutables lists the executable paths of the running processes.
func runningExecutables() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "comm=").Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// spotlightLastUsed reads kMDItemLastUsedDate of many paths in one call.
// Spotlight sets it only for launches through LaunchServices, so a zero
// date means "unknown", not "never opened".
func spotlightLastUsed(paths []string) []time.Time {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := append([]string{"-raw", "-name", "kMDItemLastUsedDate"}, paths...)
	out, err := exec.CommandContext(ctx, "/usr/bin/mdls", args...).Output()
	if err != nil {
		return nil
	}
	values := bytes.Split(out, []byte{0})
	dates := make([]time.Time, len(paths))
	for i := range paths {
		if i < len(values) {
			if d, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(values[i]))); err == nil {
				dates[i] = d
			}
		}
	}
	return dates
}

// KnownBundleIDs returns the bundle IDs of the apps that Spotlight knows
// anywhere on the Mac, and of the given apps. The leftover search needs all
// of them: an app outside /Applications still owns its data.
func KnownBundleIDs(apps []App) []string {
	ids := map[string]bool{}
	for _, a := range apps {
		if a.BundleID != "" {
			ids[a.BundleID] = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/mdfind", "kMDItemContentType == 'com.apple.application-bundle'").Output()
	if err == nil {
		paths := strings.Split(strings.TrimSpace(string(out)), "\n")
		for start := 0; start < len(paths); start += 200 {
			end := min(len(paths), start+200)
			args := append([]string{"-raw", "-name", "kMDItemCFBundleIdentifier"}, paths[start:end]...)
			values, err := exec.CommandContext(ctx, "/usr/bin/mdls", args...).Output()
			if err != nil {
				continue
			}
			for _, v := range bytes.Split(values, []byte{0}) {
				if id := strings.TrimSpace(string(v)); id != "" && id != "(null)" {
					ids[id] = true
				}
			}
		}
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Strings(list)
	return list
}

// MeasureSizes measures the bundles of apps in place, a few at a time, and
// calls done after each one. It is for a list that shows first and fills
// in the sizes.
func MeasureSizes(ctx context.Context, apps []App, done func(index int, size int64)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := range apps {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			if r, err := usage.Scan(ctx, path, usage.Options{Workers: 2, LargestFiles: 1, FilesPerDir: 1}); err == nil && !r.Cancelled {
				done(i, r.Root.Allocated)
			}
		}(i, apps[i].Path)
	}
	wg.Wait()
}

// MeasureRelated fills in the sizes of related files.
func MeasureRelated(ctx context.Context, files []Related) {
	for i := range files {
		if !files[i].IsDir {
			if info, err := os.Lstat(files[i].Path); err == nil {
				if st, ok := info.Sys().(*syscall.Stat_t); ok {
					files[i].Size = st.Blocks * 512
				} else {
					files[i].Size = info.Size()
				}
			}
			continue
		}
		if r, err := usage.Scan(ctx, files[i].Path, usage.Options{Workers: 2, LargestFiles: 1, FilesPerDir: 1}); err == nil {
			files[i].Size = r.Root.Allocated
		}
	}
}

// Unused reports whether an app was not opened for six months. An app
// without a known date is not called unused.
func (a App) Unused(now time.Time) bool {
	return a.LastUsedKnown && now.Sub(a.LastUsed) > 182*24*time.Hour
}
