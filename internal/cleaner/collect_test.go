package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wanggang316/handbyte/internal/scanner"
	"github.com/wanggang316/handbyte/internal/settings"
)

// collectHome makes a private home and a fake Trash that moves with rename.
func collectHome(t *testing.T) (home string, opts CollectOptions) {
	t.Helper()
	home, err := scanner.Physical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	trash := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trash, 0o700); err != nil {
		t.Fatal(err)
	}
	opts.Trash = func(path string) (string, error) {
		destination := filepath.Join(trash, filepath.Base(path))
		return destination, os.Rename(path, destination)
	}
	return home, opts
}

func makeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectMovesFilesAndFoldersToTheTrashAndRecordsThem(t *testing.T) {
	home, opts := collectHome(t)
	file := filepath.Join(home, "Downloads/old.dmg")
	folder := filepath.Join(home, "Documents/archive")
	makeFile(t, file, 5000)
	makeFile(t, filepath.Join(folder, "a.txt"), 100)
	var items []Collected
	for _, p := range []string{file, folder} {
		c, err := NewCollected(p, 1)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, c)
	}
	results := Collect(context.Background(), items, opts, nil)
	for _, r := range results {
		if r.Status != "trashed" || r.Destination == "" || r.TrashIdentity.Inode == 0 || r.Moved != 1 {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
	for _, p := range []string{file, folder} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", p)
		}
	}
	history, err := History()
	if err != nil || len(history) != 2 || history[0].Status != "trashed" {
		t.Fatalf("journal: %+v %v", history, err)
	}
}

func TestCollectRefusesStructuralAndPrivatePaths(t *testing.T) {
	home, _ := collectHome(t)
	for _, rel := range []string{"", "Documents", "Library", "Library/Keychains/login.keychain-db", "Library/Mobile Documents/x", ".Trash/old", "Desktop"} {
		path := filepath.Join(home, rel)
		makeFile(t, filepath.Join(path, "x"), 1)
		if _, err := NewCollected(path, 1); err == nil {
			t.Errorf("%q: accepted", rel)
		}
	}
	outside, err := scanner.Physical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	makeFile(t, filepath.Join(outside, "f"), 1)
	if _, err := NewCollected(filepath.Join(outside, "f"), 1); err == nil {
		t.Error("a path outside the home folder was accepted")
	}
	link := filepath.Join(home, "Downloads/link")
	makeFile(t, filepath.Join(home, "Downloads/keep"), 1)
	if err := os.Symlink(filepath.Join(home, "Documents"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCollected(link, 1); err == nil {
		t.Error("a symbolic link was accepted")
	}
	if _, err := NewCollected(filepath.Join(link, "x"), 1); err == nil {
		t.Error("a path through a symbolic link was accepted")
	}
}

func TestCollectRefusesAnExcludedPath(t *testing.T) {
	home, _ := collectHome(t)
	path := filepath.Join(home, "Downloads/keep.zip")
	makeFile(t, path, 1)
	if err := settings.Save(settings.Config{Version: 1, Exclusions: []string{path}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCollected(path, 1); err == nil || !strings.Contains(err.Error(), "exclusion") {
		t.Fatalf("an excluded path must be refused: %v", err)
	}
}

func TestCollectSkipsAChangedItemAndAChildOfASelectedFolder(t *testing.T) {
	home, opts := collectHome(t)
	folder := filepath.Join(home, "Documents/project")
	child := filepath.Join(folder, "big.bin")
	other := filepath.Join(home, "Downloads/replaced.bin")
	makeFile(t, child, 100)
	makeFile(t, other, 100)
	var items []Collected
	for _, p := range []string{folder, child, other} {
		c, err := NewCollected(p, 1)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, c)
	}
	// Replace the file after review: the identity differs.
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	makeFile(t, other, 100)
	results := Collect(context.Background(), items, opts, nil)
	got := map[string]string{}
	for _, r := range results {
		got[r.Path] = r.Status
	}
	if got[folder] != "trashed" || got[child] != "skipped" || got[other] != "skipped" {
		t.Fatalf("statuses: %v", got)
	}
	if _, err := os.Lstat(other); err != nil {
		t.Fatal("a changed item must stay")
	}
}

func TestCollectDryRunChangesNothing(t *testing.T) {
	home, opts := collectHome(t)
	path := filepath.Join(home, "Downloads/a.bin")
	makeFile(t, path, 100)
	c, err := NewCollected(path, 7)
	if err != nil {
		t.Fatal(err)
	}
	opts.DryRun = true
	results := Collect(context.Background(), []Collected{c}, opts, nil)
	if results[0].Status != "previewed" || results[0].Previewed != 7 {
		t.Fatalf("preview: %+v", results[0])
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatal("a preview must not move the file")
	}
}
