package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Progress is emitted live during scanning.
type Progress struct {
	CurrentPath string
	BytesSeen   int64
	FilesSeen   int64
	ItemsFound  int
	Done        bool
	Items       []Item
}

// Config controls a scan.
type Config struct {
	Exclusions    []string
	ActivityProbe ActivityProbe
	// ProjectRoots overrides DefaultProjectRoots if non-empty.
	ProjectRoots []string
	// MinSize: items smaller than this are dropped from the result.
	MinSize int64
	// Workers caps the number of concurrent du workers (>=1).
	Workers int
	// SkipProjectScan turns off project-root artifact discovery.
	SkipProjectScan bool
}

// Default returns a sensible default Config.
func Default() Config {
	return Config{
		MinSize: 50 * 1024 * 1024, // 50 MiB floor
		Workers: 6,
	}
}

// Scan inspects the filesystem and emits Progress updates on progress until
// done. The returned slice is sorted by size descending.
//
// progress may be nil. It is closed when the scan completes.
func Scan(ctx context.Context, cfg Config, progress chan<- Progress) ([]Item, error) {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if progress != nil {
		defer close(progress)
	}

	// Build the task list: preset paths (possibly expanded), plus project
	// artifact scans rooted at user dev directories.
	type task struct {
		path                  string
		category              Category
		safety                Safety
		description           string
		source                string
		project, scope, owner string
		name                  string
	}
	var tasks []task
	var unavailable []Item
	report := func(path string, category Category, err error) {
		if os.IsNotExist(err) {
			return
		}
		unavailable = append(unavailable, Item{Path: path, Category: category, Safety: SafetyCaution, ScanStatus: "unavailable", ScanError: err.Error()})
	}

	for _, p := range Presets {
		expanded, ok := expandHome(p.Path)
		if !ok {
			continue
		}
		info, err := os.Lstat(expanded)
		if err != nil {
			report(expanded, p.Category, err)
			continue
		}
		if !info.IsDir() {
			continue
		}
		if p.ExpandChildren {
			entries, err := os.ReadDir(expanded)
			if err != nil {
				report(expanded, p.Category, err)
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				tasks = append(tasks, task{
					path:        filepath.Join(expanded, e.Name()),
					category:    p.Category,
					safety:      p.Safety,
					description: p.Description,
					source:      p.Source,
				})
			}
		} else {
			tasks = append(tasks, task{
				path:        expanded,
				category:    p.Category,
				safety:      p.Safety,
				description: p.Description,
				source:      p.Source,
			})
		}
	}

	simulatorItems, simulatorErr := discoverSimulators(ctx)
	if simulatorErr != nil {
		for _, it := range simulatorItems {
			report(it.Path, CatXcode, simulatorErr)
		}
	} else {
		for _, it := range simulatorItems {
			tasks = append(tasks, task{path: it.Path, category: it.Category, safety: it.Safety, description: it.Description, source: it.Source, owner: it.Owner, name: it.Name})
		}
	}

	// Project artifact roots: collected up-front so we can du them like presets.
	if !cfg.SkipProjectScan {
		roots := cfg.ProjectRoots
		if len(roots) == 0 {
			roots = DefaultProjectRoots
		}
		patternByName := make(map[string]ProjectArtifact, len(ProjectArtifactPatterns))
		for _, p := range ProjectArtifactPatterns {
			patternByName[p.Name] = p
		}
		matches := findArtifacts(ctx, roots, patternByName, progress, func(path string, err error) { report(path, CatProjectBuild, err) })
		for _, m := range matches {
			pat := patternByName[filepath.Base(m.path)]
			tasks = append(tasks, task{
				path: m.path, project: m.project, scope: m.scope, owner: m.owner,
				category:    pat.Category,
				safety:      pat.Safety,
				description: pat.Description,
				source:      "artifact:" + pat.Name,
			})
		}
	}

	// Presets are ordered from specific rules to generic fallbacks.
	seen := make(map[string]bool)
	unique := tasks[:0]
	for _, t := range tasks {
		t.path = filepath.Clean(t.path)
		if canonical, err := Physical(t.path); err == nil {
			t.path = canonical
		}
		if seen[t.path] {
			continue
		}
		seen[t.path] = true
		unique = append(unique, t)
	}
	tasks = unique

	if cfg.ActivityProbe == nil {
		cfg.ActivityProbe = processProbe(ctx)
	}

	// Now du each task concurrently.
	var (
		mu       sync.Mutex
		items    = unavailable
		bytes    atomic.Int64
		files    atomic.Int64
		jobs     = make(chan task)
		wg       sync.WaitGroup
		emitOnce sync.Mutex
	)

	emit := func(current string) {
		if progress == nil {
			return
		}
		emitOnce.Lock()
		mu.Lock()
		snapshot := append([]Item(nil), items...)
		mu.Unlock()
		select {
		case progress <- Progress{
			CurrentPath: current,
			BytesSeen:   bytes.Load(),
			FilesSeen:   files.Load(),
			ItemsFound:  len(snapshot),
			Items:       snapshot,
		}:
		default:
		}
		emitOnce.Unlock()
	}

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				if ctx.Err() != nil {
					return
				}
				emit(t.path)
				id, idErr := CaptureIdentity(t.path)
				size, count, mt, scanErr := du(ctx, t.path)
				if idErr != nil {
					scanErr = idErr
				}
				bytes.Add(size)
				files.Add(count)
				if size < cfg.MinSize && scanErr == nil {
					continue
				}
				status := "complete"
				reason := ""
				if scanErr != nil {
					status = "partial"
					reason = scanErr.Error()
				}
				if ctx.Err() != nil {
					status = "cancelled"
				}
				it := Item{
					Identity: id, Project: t.project, Scope: t.scope, Owner: t.owner, Name: t.name,
					ScanStatus: status, ScanError: reason,
					Path:        t.path,
					Size:        size,
					FileCount:   count,
					ModTime:     mt,
					Category:    t.category,
					Safety:      t.safety,
					Description: t.description,
					Source:      t.source,
				}
				assess(ctx, &it, cfg.Exclusions, cfg.ActivityProbe)
				mu.Lock()
				items = append(items, it)
				mu.Unlock()
				emit(t.path)
			}
		}()
	}
dispatch:
	for _, t := range tasks {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- t:
		}
	}
	close(jobs)
	wg.Wait()

	sort.Slice(items, func(i, j int) bool { return items[i].Size > items[j].Size })

	if progress != nil {
		emitOnce.Lock()
		select {
		case progress <- Progress{
			BytesSeen:  bytes.Load(),
			FilesSeen:  files.Load(),
			ItemsFound: len(items),
			Done:       true,
		}:
		default:
		}
		emitOnce.Unlock()
	}

	return items, ctx.Err()
}

// du recursively sums the size of path. Returns (bytes, files, mostRecentMtime).
//
// Incomplete measurements retain their error instead of implying an empty subtree.
func du(ctx context.Context, path string) (int64, int64, time.Time, error) {
	var scanErr error
	var (
		total int64
		count int64
		mod   time.Time
	)
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			scanErr = ctx.Err()
			return filepath.SkipAll
		}
		if err != nil {
			if scanErr == nil {
				scanErr = fmt.Errorf("%s: %w", p, err)
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if scanErr == nil {
				scanErr = err
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		total += diskBytes(info)
		count++
		if mt := info.ModTime(); mt.After(mod) {
			mod = mt
		}
		return nil
	})
	return total, count, mod, scanErr
}

// findArtifacts walks roots looking for directories whose basename matches
// one of the configured project artifact patterns. When a match is found,
// the walker does not descend into it (so we count node_modules without
// walking into nested node_modules).
func findArtifacts(ctx context.Context, roots []string, patterns map[string]ProjectArtifact, progress chan<- Progress, report func(string, error)) []projectTarget {
	var (
		found []projectTarget
		mu    sync.Mutex
	)
	for _, root := range roots {
		expanded, ok := expandHome(root)
		if !ok {
			continue
		}
		canonical, err := Physical(expanded)
		if err != nil {
			report(expanded, err)
			continue
		}
		expanded = canonical
		if progress != nil {
			select {
			case progress <- Progress{CurrentPath: "scanning " + expanded}:
			default:
			}
		}
		_ = filepath.WalkDir(expanded, func(p string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return filepath.SkipAll
			}
			if err != nil {
				report(p, err)
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			// Skip dot-dirs at root level except those we explicitly want.
			base := d.Name()
			if _, hit := patterns[base]; hit {
				mu.Lock()
				if candidate, ok := projectEvidence(p, expanded); ok && p != expanded {
					found = append(found, candidate)
				}
				mu.Unlock()
				return filepath.SkipDir
			}
			// Don't descend into VCS metadata.
			if base == ".git" || base == ".svn" || base == ".hg" {
				return filepath.SkipDir
			}
			return nil
		})
	}
	return found
}

// diskBytes returns the actual on-disk size in bytes for the given file.
// On macOS/Linux this is `st_blocks * 512`, which correctly accounts for
// sparse files (the OrbStack/Apple-Container disk image trick). Falls back
// to logical Size() on non-unix platforms.
func diskBytes(info fs.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		// st.Blocks is number of 512-byte blocks actually allocated.
		return int64(st.Blocks) * 512
	}
	return info.Size()
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path string) (string, bool) {
	if !strings.HasPrefix(path, "~") {
		return path, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	if path == "~" {
		return home, true
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), true
	}
	return path, true
}
