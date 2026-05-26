package cleaner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Mode selects the deletion strategy.
type Mode int

const (
	// ModeTrash moves the target into ~/.Trash. Default and safest.
	ModeTrash Mode = iota
	// ModeRemove unlinks the target directly. Faster, unrecoverable.
	ModeRemove
	// ModeDryRun reports what would happen but changes nothing.
	ModeDryRun
)

// Result is the outcome of cleaning one path.
type Result struct {
	Path      string
	Freed     int64
	Err       error
	StartedAt time.Time
	EndedAt   time.Time
}

// Progress reports incremental status while cleaning a batch.
type Progress struct {
	Current     string
	Completed   int
	Total       int
	BytesFreed  int64
	LastResult  *Result
	Done        bool
}

// Clean processes the given paths and emits Progress updates. The progress
// channel is closed when cleaning finishes (or context is cancelled).
//
// sizes maps path -> bytes-on-disk and is used to report freed totals; entries
// missing from the map fall back to 0.
func Clean(ctx context.Context, paths []string, sizes map[string]int64, mode Mode, progress chan<- Progress) []Result {
	if progress != nil {
		defer close(progress)
	}
	results := make([]Result, 0, len(paths))
	var freed int64

	for i, p := range paths {
		if ctx.Err() != nil {
			break
		}
		if progress != nil {
			select {
			case progress <- Progress{Current: p, Completed: i, Total: len(paths), BytesFreed: freed}:
			default:
			}
		}
		r := Result{Path: p, StartedAt: time.Now()}
		switch mode {
		case ModeDryRun:
			// no-op
		case ModeTrash:
			r.Err = moveToTrash(p)
		case ModeRemove:
			r.Err = os.RemoveAll(p)
		}
		r.EndedAt = time.Now()
		if r.Err == nil {
			r.Freed = sizes[p]
			freed += r.Freed
		}
		results = append(results, r)
		if progress != nil {
			res := r
			select {
			case progress <- Progress{
				Current:    p,
				Completed:  i + 1,
				Total:      len(paths),
				BytesFreed: freed,
				LastResult: &res,
			}:
			default:
			}
		}
	}

	if progress != nil {
		select {
		case progress <- Progress{Completed: len(results), Total: len(paths), BytesFreed: freed, Done: true}:
		default:
		}
	}
	return results
}

// moveToTrash relocates path into ~/.Trash. Falls back to AppleScript on
// failures so the file appears in Finder's Trash with "Put Back" working.
func moveToTrash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}

	// Try the fast path first: rename into ~/.Trash with a unique name.
	home, err := os.UserHomeDir()
	if err == nil {
		trashDir := filepath.Join(home, ".Trash")
		if err := os.MkdirAll(trashDir, 0o755); err == nil {
			target := uniqueTrashTarget(trashDir, filepath.Base(abs))
			if err := os.Rename(abs, target); err == nil {
				return nil
			}
			// fall through to AppleScript on cross-device or permission errors
		}
	}

	// AppleScript fallback. Finder handles cross-volume moves and writes
	// the metadata that powers Put Back.
	script := fmt.Sprintf(`tell application "Finder" to delete POSIX file %q`, abs)
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript trash: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func uniqueTrashTarget(trashDir, base string) string {
	target := filepath.Join(trashDir, base)
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return target
	}
	stamp := time.Now().Format("2006-01-02 150405")
	return filepath.Join(trashDir, fmt.Sprintf("%s (%s)", base, stamp))
}
