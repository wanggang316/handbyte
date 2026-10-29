package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestValidateBlocksStructuralAndPersonalDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"", "Downloads", "Library/Containers/example", ".docker", ".Trash"} {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		it := Item{Path: path, Source: "downloads", Eligibility: Review, ScanStatus: "complete"}
		if err := Validate(context.Background(), it, nil, nil); err == nil {
			t.Errorf("allowed protected directory %s", name)
		}
	}
}

func idleProbe(context.Context, Item) (Activity, error) { return Idle, nil }

func reviewedCache(t *testing.T) Item {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".npm/_cacache")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "data"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := Physical(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := CaptureIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	size, count, mt, err := du(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return Item{Path: path, Source: "npm-cache", ScanStatus: "complete", Identity: id, Size: size, FileCount: count, ModTime: mt, Eligibility: Review}
}

func TestValidateRechecksReviewedCache(t *testing.T) {
	it := reviewedCache(t)
	if err := Validate(context.Background(), it, nil, idleProbe); err != nil {
		t.Fatal(err)
	}
	if err := Validate(context.Background(), it, []string{filepath.Join(it.Path, "data")}, idleProbe); err == nil {
		t.Error("protected child ignored")
	}
	if err := Validate(context.Background(), it, nil, func(context.Context, Item) (Activity, error) { return Active, nil }); err == nil {
		t.Error("active owner allowed")
	}
	if err := Validate(context.Background(), it, nil, func(context.Context, Item) (Activity, error) { return Unknown, nil }); err == nil {
		t.Error("unknown owner allowed")
	}
	saved := it.Path + "-original"
	if err := os.Rename(it.Path, saved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(it.Path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Validate(context.Background(), it, nil, idleProbe); err == nil {
		t.Error("replacement allowed")
	}
	if err := os.Remove(it.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(saved, it.Path); err != nil {
		t.Fatal(err)
	}
	if err := Validate(context.Background(), it, nil, idleProbe); err == nil {
		t.Error("symlink swap allowed")
	}
}

func TestValidateRejectsNestedRepository(t *testing.T) {
	it := reviewedCache(t)
	if err := os.Mkdir(filepath.Join(it.Path, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Validate(context.Background(), it, nil, idleProbe); err == nil {
		t.Fatal("nested repository allowed")
	}
}

func TestValidateRejectsTrackedCache(t *testing.T) {
	it := reviewedCache(t)
	home, _ := os.UserHomeDir()
	for _, args := range [][]string{{"init", "-q", home}, {"-C", home, "add", ".npm/_cacache/data"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s: %v", out, err)
		}
	}
	if err := Validate(context.Background(), it, nil, idleProbe); err == nil {
		t.Fatal("tracked cache allowed")
	}
}

func TestValidateRejectsNewDirectoryAfterReview(t *testing.T) {
	it := reviewedCache(t)
	if err := os.Mkdir(filepath.Join(it.Path, "new-work"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Validate(context.Background(), it, nil, idleProbe); err == nil {
		t.Fatal("new work was not detected")
	}
}
