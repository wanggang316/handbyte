package cleaner

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
)

// AppPlan is an uninstall that the user reviewed: the bundle and the files
// of the app that stay selected.
type AppPlan struct {
	Name     string
	BundleID string
	// Bundle is the .app folder. Empty for a reset, which keeps the app.
	Bundle         string
	BundleIdentity scanner.Identity
	Files          []Collected
	// System and Cask come from the inventory; both refuse the uninstall.
	System bool
	Cask   string
}

// AppOptions controls Uninstall.
type AppOptions struct {
	DryRun bool
	// Running reports whether a process of the bundle runs now.
	Running func(bundle string) bool
	// Trash moves a path to the Trash; tests replace it.
	Trash func(path string) (string, error)
	// Roots are the application folders; a bundle must be directly in one.
	Roots []string
}

// NewAppFile checks a file of an app and binds it to its file system
// object. It must be in the home folder and not a symbolic link.
func NewAppFile(path string, size int64) (Collected, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Collected{}, err
	}
	if home, err = scanner.Physical(home); err != nil {
		return Collected{}, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !scanner.Contains(home, path) || path == home ||
		path == filepath.Join(home, "Library") {
		return Collected{}, errors.New("an app file must be inside the home folder")
	}
	if physical, err := scanner.Physical(path); err != nil || physical != path {
		return Collected{}, errors.New("the path goes through a symbolic link")
	}
	config, err := settings.Load()
	if err != nil {
		return Collected{}, err
	}
	if scanner.Excluded(path, config.Exclusions) {
		return Collected{}, errors.New("protected by an exclusion")
	}
	id, err := scanner.EntryIdentity(path)
	if err != nil {
		return Collected{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Collected{}, err
	}
	return Collected{Path: path, Size: size, IsDir: info.IsDir(), Identity: id}, nil
}

// CheckApp says why an app cannot be uninstalled now, or nil.
func CheckApp(p AppPlan, opts AppOptions) error {
	switch {
	case p.System:
		return fmt.Errorf("%s is part of macOS and cannot be uninstalled", p.Name)
	case p.Cask != "" && p.Bundle != "":
		return fmt.Errorf("%s was installed with Homebrew; run: brew uninstall --cask %s", p.Name, p.Cask)
	case p.Bundle != "" && opts.Running != nil && opts.Running(p.Bundle):
		return fmt.Errorf("%s is running; quit it first", p.Name)
	}
	if p.Bundle == "" {
		return nil
	}
	inRoot := false
	for _, r := range opts.Roots {
		if filepath.Dir(p.Bundle) == r {
			inRoot = true
		}
	}
	if !inRoot || !strings.HasSuffix(p.Bundle, ".app") {
		return errors.New("the app is not directly in an application folder")
	}
	id, err := scanner.EntryIdentity(p.Bundle)
	if err != nil {
		return err
	}
	if id != p.BundleIdentity {
		return errors.New("the app changed since review; open the list again")
	}
	return nil
}

// Uninstall moves the app and its selected files to the Trash, as one group
// in the journal: `handbyte restore <group>` puts all of them back. The
// bundle goes first; when it cannot move, nothing else moves.
func Uninstall(ctx context.Context, p AppPlan, opts AppOptions) (string, []Result, error) {
	if opts.Trash == nil {
		opts.Trash = moveToTrash
	}
	if err := CheckApp(p, opts); err != nil {
		return "", nil, err
	}
	group := "app-" + strings.ToLower(rand.Text()[:10])
	var results []Result
	record := func(r Result) error {
		r.Group = group
		r.EndedAt = time.Now()
		if r.Err != nil && r.Message == "" {
			r.Message = r.Err.Error()
		}
		results = append(results, r)
		if opts.DryRun {
			return nil
		}
		return appendJournal(r)
	}
	move := func(path string, size int64, recheck func() error) (Result, error) {
		r := Result{ID: rand.Text(), Path: path, StartedAt: time.Now()}
		if err := ctx.Err(); err != nil {
			r.Status, r.Err = "cancelled", err
			return r, err
		}
		if err := recheck(); err != nil {
			r.Status, r.Err = "skipped", err
			return r, nil
		}
		if opts.DryRun {
			r.Status, r.Previewed = "previewed", size
			return r, nil
		}
		r.Status = "started"
		if err := appendJournal(r); err != nil {
			r.Status, r.Err = "failed", err
			return r, err
		}
		r.Destination, r.Err = opts.Trash(path)
		if r.Err != nil {
			r.Status = "failed"
			return r, nil
		}
		r.Status, r.Moved = "trashed", size
		if r.TrashIdentity, r.Err = scanner.EntryIdentity(r.Destination); r.Err != nil {
			r.Message = "Moved to Trash; destination identity unavailable. Restore in Finder."
		}
		return r, nil
	}
	if p.Bundle != "" {
		r, err := move(p.Bundle, 0, func() error { return CheckApp(p, opts) })
		if logErr := record(r); logErr != nil && err == nil {
			err = logErr
		}
		if err != nil || r.Status != "trashed" && r.Status != "previewed" {
			if err == nil {
				err = fmt.Errorf("the app did not move: %s", r.Message)
			}
			return group, results, err
		}
	}
	for _, f := range p.Files {
		f := f
		r, err := move(f.Path, f.Size, func() error {
			id, err := scanner.EntryIdentity(f.Path)
			if err != nil {
				return err
			}
			if id != f.Identity {
				return errors.New("the file changed since review")
			}
			return nil
		})
		if logErr := record(r); logErr != nil && err == nil {
			err = logErr
		}
		if err != nil {
			return group, results, err
		}
	}
	return group, results, nil
}
