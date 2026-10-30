package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExploreReportsChildrenWithoutCleanupAuthority(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := Explore(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("wanted both file and directory, got %d", len(items))
	}
	for _, it := range items {
		if it.Selectable() {
			t.Fatal("explorer granted cleanup authority")
		}
	}
}
