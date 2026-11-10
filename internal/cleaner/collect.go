package cleaner

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
)

// Collected is a file or a folder that the user picked in the storage
// view. No cleanup rule covers it, so it only ever goes to the Trash.
type Collected struct {
	Path     string
	Size     int64
	IsDir    bool
	Identity scanner.Identity
}

// CollectOptions controls Collect.
type CollectOptions struct {
	// DryRun reports what would be moved and changes nothing.
	DryRun bool
	// Trash moves a path to the Trash and returns the destination. Tests
	// replace it; the default is the Finder.
	Trash func(path string) (string, error)
}

// Folders of the home directory that are never collected, as a whole or
// below. They hold accounts, keys, mail and files that sync elsewhere.
var privateFolders = []string{
	".Trash",
	"Library/Accounts",
	"Library/CloudStorage",
	"Library/Cookies",
	"Library/Keychains",
	"Library/Mail",
	"Library/Messages",
	"Library/Mobile Documents",
	"Library/Preferences",
	"Library/Safari",
}

// Folders of the home directory that may hold collected items but are not
// collected as a whole.
var structuralFolders = []string{
	"Applications", "Desktop", "Documents", "Downloads", "Library", "Movies", "Music", "Pictures", "Public",
	"Library/Application Support", "Library/Caches", "Library/Containers", "Library/Group Containers", "Library/Logs",
}

// CheckCollectable says why a path cannot be collected, or nil.
func CheckCollectable(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if home, err = scanner.Physical(home); err != nil {
		return err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("the path must be absolute and clean")
	}
	if !scanner.Contains(home, path) || path == home {
		return errors.New("only items inside the home folder can be collected")
	}
	if physical, err := scanner.Physical(path); err != nil || physical != path {
		return errors.New("the path goes through a symbolic link")
	}
	for _, rel := range privateFolders {
		if scanner.Contains(filepath.Join(home, rel), path) {
			return fmt.Errorf("%s holds private data and is never collected", "~/"+rel)
		}
	}
	for _, rel := range structuralFolders {
		if path == filepath.Join(home, rel) {
			return fmt.Errorf("%s is a standard folder; collect the items in it", "~/"+rel)
		}
	}
	config, err := settings.Load()
	if err != nil {
		return err
	}
	if scanner.Excluded(path, config.Exclusions) {
		return errors.New("protected by an exclusion")
	}
	return nil
}

// NewCollected checks a path and binds it to its filesystem object.
func NewCollected(path string, size int64) (Collected, error) {
	if err := CheckCollectable(path); err != nil {
		return Collected{}, err
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

// recheck runs the checks again at the action boundary.
func (c Collected) recheck() error {
	if err := CheckCollectable(c.Path); err != nil {
		return err
	}
	id, err := scanner.EntryIdentity(c.Path)
	if err != nil {
		return err
	}
	if id != c.Identity {
		return errors.New("the item changed since review; scan again")
	}
	return nil
}

// Collect moves the items to the Trash, one at a time, and records each
// action in the journal so that `handbyte restore` can put it back. An item
// inside another selected folder goes with that folder and is skipped.
func Collect(ctx context.Context, items []Collected, opts CollectOptions, progress chan<- Progress) []Result {
	if progress != nil {
		defer close(progress)
	}
	if opts.Trash == nil {
		opts.Trash = moveToTrash
	}
	results := make([]Result, 0, len(items))
	journalFailure := ""
	seen := map[string]bool{}
	for _, c := range items {
		r := Result{ID: rand.Text(), Path: c.Path, StartedAt: time.Now()}
		inside := false
		for _, other := range items {
			if other.Path != c.Path && scanner.Contains(other.Path, c.Path) {
				inside = true
			}
		}
		switch {
		case seen[c.Path]:
			continue
		case ctx.Err() != nil:
			r.Status, r.Err = "cancelled", ctx.Err()
		case journalFailure != "":
			r.Status, r.Err = "skipped", fmt.Errorf("journal unavailable: %s", journalFailure)
		case inside:
			r.Status, r.Err = "skipped", errors.New("goes with the selected folder that holds it")
		default:
			collectOne(c, opts, &r, &journalFailure)
		}
		seen[c.Path] = true
		if r.Err != nil && r.Message == "" {
			r.Message = r.Err.Error()
		}
		r.EndedAt = time.Now()
		results = append(results, r)
		if progress != nil {
			select {
			case progress <- Progress{Current: c.Path, Completed: len(results), Total: len(items), LastResult: &r, Done: len(results) == len(items)}:
			default:
			}
		}
	}
	return results
}

func collectOne(c Collected, opts CollectOptions, r *Result, journalFailure *string) {
	if err := c.recheck(); err != nil {
		r.Status, r.Err = "skipped", err
		return
	}
	if opts.DryRun {
		r.Status, r.Previewed = "previewed", c.Size
		return
	}
	r.Status = "started"
	if err := appendJournal(*r); err != nil {
		r.Status, r.Err = "failed", err
		*journalFailure = err.Error()
		return
	}
	// Check again after writing intent, immediately before the move.
	if err := c.recheck(); err != nil {
		r.Status, r.Err = "skipped", err
	} else {
		before, beforeErr := freeBytes(filepath.Dir(c.Path))
		r.Destination, r.Err = opts.Trash(c.Path)
		if r.Err == nil {
			r.Status, r.Moved = "trashed", c.Size
			if r.TrashIdentity, r.Err = scanner.EntryIdentity(r.Destination); r.Err != nil {
				r.Message = "Moved to Trash; destination identity unavailable. Restore in Finder."
			}
		} else {
			r.Status = "failed"
		}
		if after, err := freeBytes(filepath.Dir(c.Path)); err == nil && beforeErr == nil {
			delta := after - before
			r.FreeDelta = &delta
		}
	}
	r.EndedAt = time.Now()
	if r.Err != nil && r.Message == "" {
		r.Message = r.Err.Error()
	}
	if err := appendJournal(*r); err != nil {
		r.JournalError = err.Error()
		*journalFailure = err.Error()
	}
}
