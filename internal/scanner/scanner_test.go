package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanReportsSpecificCacheOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "Library/Caches/go-build")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "data"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := Scan(context.Background(), Config{Workers: 4, SkipProjectScan: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var matches []Item
	for _, it := range items {
		if it.Path == path {
			matches = append(matches, it)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("wanted one cache, got %d", len(matches))
	}
	if matches[0].Source != "go-build" {
		t.Fatalf("lost specific explanation: %s", matches[0].Source)
	}
}

func TestScanKeepsUnreadableCategoryVisible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "Library/Caches")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0700)
	items, err := Scan(context.Background(), Config{MinSize: 1024, Workers: 2, SkipProjectScan: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Path == path && it.ScanStatus == "unavailable" && it.ScanError != "" {
			return
		}
	}
	t.Fatalf("unreadable category disappeared: %+v", items)
}
