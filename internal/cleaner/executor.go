package cleaner

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
)

type Options struct {
	Mode          Mode
	Exclusions    []string
	ActivityProbe scanner.ActivityProbe
}

// Execute applies only revalidated, non-overlapping candidates. Cancellation
// stops the next action; an in-flight filesystem operation must report its outcome.
func Execute(ctx context.Context, items []scanner.Item, opts Options, progress chan<- Progress) []Result {
	if progress != nil {
		defer close(progress)
	}
	items = uniqueItems(items)
	results := make([]Result, 0, len(items))
	journalFailure := ""
	for _, it := range items {
		r := Result{ID: rand.Text(), Path: it.Path, StartedAt: time.Now()}
		emit := func(done bool) {
			if progress == nil {
				return
			}
			select {
			case progress <- Progress{Current: it.Path, Completed: len(results), Total: len(items), LastResult: &r, Done: done}:
			default:
			}
		}
		switch {
		case ctx.Err() != nil:
			r.Status = "cancelled"
			r.Err = ctx.Err()
		case journalFailure != "":
			r.Status = "skipped"
			r.Err = fmt.Errorf("journal unavailable: %s", journalFailure)
		case overlapsChild(it, items):
			r.Status = "skipped"
			r.Err = fmt.Errorf("selection overlaps a child target")
		default:
			config, err := settings.Load()
			if err == nil {
				err = scanner.Validate(ctx, it, append(append([]string{}, opts.Exclusions...), config.Exclusions...), opts.ActivityProbe)
			}
			if err != nil {
				r.Status = "skipped"
				r.Err = err
				break
			}
			if opts.Mode == ModeDryRun {
				r.Status = "previewed"
				r.Previewed = it.Size
				break
			}
			if opts.Mode != ModeTrash && opts.Mode != ModeRemove {
				r.Status = "failed"
				r.Err = fmt.Errorf("invalid cleanup mode")
				break
			}
			r.Status = "started"
			if err := appendJournal(r); err != nil {
				r.Status = "failed"
				r.Err = err
				journalFailure = err.Error()
				break
			}
			// Check again after writing intent, immediately before the destructive call.
			if err := scanner.Validate(ctx, it, append(append([]string{}, opts.Exclusions...), config.Exclusions...), opts.ActivityProbe); err != nil {
				r.Status = "skipped"
				r.Err = err
			} else {
				before, beforeErr := freeBytes(it.Path)
				if opts.Mode == ModeTrash {
					r.Destination, r.Err = moveToTrash(it.Path)
					if r.Err == nil {
						r.Status = "trashed"
						r.Moved = it.Size
						r.TrashIdentity, r.Err = scanner.CaptureIdentity(r.Destination)
						if r.Err != nil {
							r.Message = "Moved to Trash; destination identity unavailable. Restore in Finder."
						}
					}
				} else {
					r.Err = os.RemoveAll(it.Path)
					if r.Err == nil {
						r.Status = "removed"
						r.Freed = it.Size
					}
				}
				if r.Err != nil && r.Status == "started" {
					r.Status = "failed"
				}
				if after, err := freeBytes(filepath.Dir(it.Path)); err == nil && beforeErr == nil {
					delta := after - before
					r.FreeDelta = &delta
				}
			}
			r.EndedAt = time.Now()
			if r.Err != nil && r.Message == "" {
				r.Message = r.Err.Error()
			}
			if err := appendJournal(r); err != nil {
				r.JournalError = err.Error()
				journalFailure = err.Error()
			}
		}
		if r.Err != nil && r.Message == "" {
			r.Message = r.Err.Error()
		}
		r.EndedAt = time.Now()
		results = append(results, r)
		emit(len(results) == len(items))
	}
	return results
}

func uniqueItems(items []scanner.Item) []scanner.Item {
	seen := map[string]bool{}
	out := make([]scanner.Item, 0, len(items))
	for _, it := range items {
		if !seen[it.Path] {
			seen[it.Path] = true
			out = append(out, it)
		}
	}
	return out
}

func overlapsChild(it scanner.Item, items []scanner.Item) bool {
	for _, other := range items {
		if other.Path != it.Path && scanner.Contains(it.Path, other.Path) {
			return true
		}
	}
	return false
}

func freeBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
