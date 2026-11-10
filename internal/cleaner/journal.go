package cleaner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
)

func journalPath() (string, error) {
	dir, err := settings.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "actions.jsonl"), nil
}

func appendJournal(result Result) error {
	path, err := journalPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("journal must be a regular file")
	}
	if _, err := History(); err != nil {
		return fmt.Errorf("journal validation failed: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], info.Size()-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			return errors.New("journal has an unterminated record")
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(data, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// History returns the last recorded outcome per action, including interrupted intents.
func History() ([]Result, error) {
	path, err := journalPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []Result{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var results []Result
	indices := map[string]int{}
	lines := bufio.NewScanner(f)
	lines.Buffer(make([]byte, 4096), 1024*1024)
	for lines.Scan() {
		var r Result
		if err := json.Unmarshal(lines.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("incomplete journal record: %w", err)
		}
		if i, ok := indices[r.ID]; ok {
			results[i] = r
		} else {
			indices[r.ID] = len(results)
			results = append(results, r)
		}
	}
	return results, lines.Err()
}

// Restore puts back the action with this ID, or every action of the group
// with this ID. It refuses replacement of an existing path and checks the
// Trash identity.
func Restore(ctx context.Context, id string) error {
	history, err := History()
	if err != nil {
		return err
	}
	var group []string
	for _, r := range history {
		if r.Group == id && r.Group != "" && r.Status == "trashed" {
			group = append(group, r.ID)
		}
	}
	if len(group) == 0 {
		return restoreOne(ctx, id)
	}
	var failed []string
	for _, member := range group {
		if err := restoreOne(ctx, member); err != nil {
			failed = append(failed, err.Error())
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d items were not restored: %s", len(failed), len(group), strings.Join(failed, "; "))
	}
	return nil
}

func restoreOne(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	history, err := History()
	if err != nil {
		return err
	}
	for _, r := range history {
		if r.ID != id {
			continue
		}
		if r.Status != "trashed" || r.Destination == "" || r.TrashIdentity.Inode == 0 {
			return errors.New("action has no verified Trash destination")
		}
		// A collected item can be a file; EntryIdentity accepts files and
		// folders and refuses a symbolic link.
		actual, err := scanner.EntryIdentity(r.Destination)
		if err != nil {
			return err
		}
		if actual != r.TrashIdentity {
			return errors.New("Trash item changed; inspect it in Finder")
		}
		if _, err := os.Lstat(r.Path); !os.IsNotExist(err) {
			return errors.New("original path exists or cannot be checked")
		}
		parent, err := scanner.Physical(filepath.Dir(r.Path))
		if err != nil {
			return err
		}
		if parent != filepath.Dir(r.Path) {
			return errors.New("original parent changed; restore in Finder")
		}
		r.Status = "restoring"
		if err := appendJournal(r); err != nil {
			return err
		}
		if _, err := nativeFileOperation(restoreScript, r.Destination, r.Path); err != nil {
			r.Status = "trashed"
			r.Message = err.Error()
			r.EndedAt = time.Now()
			if logErr := appendJournal(r); logErr != nil {
				return fmt.Errorf("restore failed: %v; journal: %w", err, logErr)
			}
			return err
		}
		r.Status = "restored"
		r.Message = ""
		r.EndedAt = time.Now()
		return appendJournal(r)
	}
	return errors.New("action ID not found")
}
